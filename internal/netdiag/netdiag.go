// Package netdiag provides network, DNS and connectivity maintenance tools
// including DNS cache flushing, registry ping/latency checks, port conflict
// detection, and hosts file / proxy environment auditing.
package netdiag

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

var goos = runtime.GOOS

// Seams the tests replace, so that no test flushes a cache or kills a
// process.
var (
	run = func(ctx context.Context, command string, timeout time.Duration) runner.Result {
		return runner.Run(ctx, command, timeout, nil)
	}
	has = runner.Has
	// hostsFile is the hosts file of this system.
	hostsFile = func() string {
		if goos == "windows" {
			return filepathJoin(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
		}
		return "/etc/hosts"
	}
)

// RegistryPing reports latency and accessibility for a package registry or service.
type RegistryPing struct {
	Name      string        `json:"name"`
	URL       string        `json:"url"`
	Reachable bool          `json:"reachable"`
	Latency   time.Duration `json:"latency"`
	Status    int           `json:"status"`
	Error     string        `json:"error,omitempty"`
}

// PortInfo describes a process listening on a network port.
type PortInfo struct {
	Port    int    `json:"port"`
	PID     int    `json:"pid"`
	Process string `json:"process"`
	User    string `json:"user,omitempty"`
	Proto   string `json:"proto"`
}

// HostsEntry describes a non-default entry found in the hosts file.
type HostsEntry struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Comment  string `json:"comment,omitempty"`
}

// NetworkAudit holds the findings of hosts and proxy configurations.
type NetworkAudit struct {
	HostsFile   string            `json:"hosts_file"`
	CustomHosts []HostsEntry      `json:"custom_hosts"`
	ProxyEnv    map[string]string `json:"proxy_env"`
	Warnings    []string          `json:"warnings"`
}

// DefaultRegistries to check in net ping.
var DefaultRegistries = []struct {
	Name string
	URL  string
}{
	{"Homebrew", "https://github.com/Homebrew/brew"},
	{"npm", "https://registry.npmjs.org"},
	{"PyPI", "https://pypi.org"},
	{"crates.io", "https://crates.io"},
	{"Docker Hub", "https://registry-1.docker.io"},
	{"GitHub", "https://api.github.com"},
	{"Ubuntu Mirrors", "https://archive.ubuntu.com"},
	{"Go Packages", "https://proxy.golang.org"},
}

// FlushDNS flushes the system's DNS resolver cache. The message says what
// was flushed; when the cache that matters was not, that is an error which
// says what is missing.
func FlushDNS(ctx context.Context) (string, error) {
	switch goos {
	case "darwin":
		// Two caches: the directory service's, and mDNSResponder's, which
		// is the one name lookups go through and needs an administrator.
		ds := run(ctx, "dscacheutil -flushcache", 10*time.Second)
		mdns := run(ctx, "sudo killall -HUP mDNSResponder", 10*time.Second)
		switch {
		case ds.ExitCode == 0 && mdns.ExitCode == 0:
			return "macOS DNS cache flushed (dscacheutil + mDNSResponder)", nil
		case mdns.ExitCode == 0:
			return "macOS DNS cache flushed (mDNSResponder); dscacheutil failed: " + strings.TrimSpace(ds.Output), nil
		case ds.ExitCode == 0:
			return "", errors.New("only the directory-service cache was flushed (dscacheutil). mDNSResponder, which holds the DNS cache, was not: " +
				"it needs administrator access. Run: sudo killall -HUP mDNSResponder")
		}
		return "", fmt.Errorf("the DNS cache was not flushed: %s", strings.TrimSpace(ds.Output+"\n"+mdns.Output))
	case "windows":
		res := run(ctx, "ipconfig /flushdns", 10*time.Second)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("failed to flush DNS on Windows: %s", res.Output)
		}
		return "Windows DNS cache flushed successfully (ipconfig /flushdns)", nil
	default: // Linux
		// The first of these that is installed and works. Each is tried as
		// the user and then as administrator.
		var found, failed []string
		for _, tool := range []struct{ bin, command string }{
			{"resolvectl", "resolvectl flush-caches"},
			{"systemd-resolve", "systemd-resolve --flush-caches"},
			{"nscd", "nscd -I hosts"},
		} {
			if !has(tool.bin) {
				continue
			}
			found = append(found, tool.bin)
			if run(ctx, tool.command, 10*time.Second).ExitCode == 0 {
				return "Linux DNS cache flushed (" + tool.bin + ")", nil
			}
			res := run(ctx, "sudo "+tool.command, 10*time.Second)
			if res.ExitCode == 0 {
				return "Linux DNS cache flushed (" + tool.bin + ", as administrator)", nil
			}
			failed = append(failed, tool.bin+": "+strings.TrimSpace(res.Output))
		}
		if len(found) == 0 {
			return "", errors.New("nothing was flushed: no local DNS cache was found (looked for resolvectl, systemd-resolve and nscd)")
		}
		return "", fmt.Errorf("the DNS cache was not flushed; it may need administrator access (%s)", strings.Join(failed, "; "))
	}
}

// PingRegistries tests reachability and latency against package registries.
func PingRegistries(ctx context.Context) []RegistryPing {
	var results []RegistryPing
	client := &http.Client{
		Timeout: 4 * time.Second,
	}

	for _, reg := range DefaultRegistries {
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, reg.URL, nil)
		if err != nil {
			results = append(results, RegistryPing{
				Name: reg.Name, URL: reg.URL, Reachable: false, Error: err.Error(),
			})
			continue
		}
		req.Header.Set("User-Agent", "omega/1.0 (network-check)")
		resp, err := client.Do(req)
		latency := time.Since(start)

		if err != nil {
			// Try GET if HEAD was refused (some servers return 405 on HEAD)
			reqGet, _ := http.NewRequestWithContext(ctx, http.MethodGet, reg.URL, nil)
			if reqGet != nil {
				startGet := time.Now()
				respGet, errGet := client.Do(reqGet)
				if errGet == nil {
					respGet.Body.Close()
					results = append(results, RegistryPing{
						Name: reg.Name, URL: reg.URL, Reachable: true, Latency: time.Since(startGet), Status: respGet.StatusCode,
					})
					continue
				}
			}
			results = append(results, RegistryPing{
				Name: reg.Name, URL: reg.URL, Reachable: false, Latency: latency, Error: err.Error(),
			})
			continue
		}
		resp.Body.Close()
		results = append(results, RegistryPing{
			Name: reg.Name, URL: reg.URL, Reachable: true, Latency: latency, Status: resp.StatusCode,
		})
	}
	return results
}

// FindPorts finds processes listening on the given port, or common development ports if port == 0.
func FindPorts(ctx context.Context, targetPort int) ([]PortInfo, error) {
	if goos == "windows" {
		return findPortsWindows(ctx, targetPort)
	}
	return findPortsUnix(ctx, targetPort)
}

func findPortsUnix(ctx context.Context, targetPort int) ([]PortInfo, error) {
	if !has("lsof") {
		return nil, errors.New("lsof is not installed, so the listening ports cannot be read")
	}
	// TCP sockets that are listening, and nothing else: "-i :PORT" alone
	// also lists UDP sockets and outgoing connections on that port, and
	// `net kill` would end those processes.
	cmdStr := "lsof -iTCP -sTCP:LISTEN -P -n"
	if targetPort > 0 {
		cmdStr = fmt.Sprintf("lsof -iTCP:%d -sTCP:LISTEN -P -n", targetPort)
	}

	// lsof ends with status 1 when it finds nothing, so only what it
	// printed counts.
	res := run(ctx, cmdStr, 5*time.Second)
	var ports []PortInfo
	seen := map[string]bool{}
	for i, line := range strings.Split(res.Output, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header
		}
		// COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME (STATE)
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[7] != "TCP" {
			continue
		}
		if len(fields) > 9 && fields[9] != "(LISTEN)" {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		port := parsePort(fields[8]) // e.g. "*:3000" or "127.0.0.1:8080"
		if port <= 0 || (targetPort > 0 && port != targetPort) {
			continue
		}
		// One process listening on IPv4 and IPv6 is one line each.
		key := fmt.Sprintf("%d/%d", pid, port)
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, PortInfo{Port: port, PID: pid, Process: fields[0], User: fields[2], Proto: "TCP"})
	}
	return ports, nil
}

func findPortsWindows(ctx context.Context, targetPort int) ([]PortInfo, error) {
	filter := ""
	if targetPort > 0 {
		filter = fmt.Sprintf("Where-Object { $_.LocalPort -eq %d } | ", targetPort)
	}
	script := "Get-NetTCPConnection -State Listen | " + filter +
		"Select-Object LocalPort, OwningProcess | " +
		"ForEach-Object { $p = (Get-Process -Id $_.OwningProcess -ErrorAction SilentlyContinue); " +
		"'{0} {1} {2}' -f $_.LocalPort, $_.OwningProcess, ($p.ProcessName) }"

	res := run(ctx, script, 5*time.Second)
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("the listening ports could not be read: %s", strings.TrimSpace(res.Output))
	}
	var ports []PortInfo
	for _, line := range strings.Split(res.Output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			port, _ := strconv.Atoi(fields[0])
			pid, _ := strconv.Atoi(fields[1])
			proc := ""
			if len(fields) >= 3 {
				proc = fields[2]
			}
			if port > 0 {
				ports = append(ports, PortInfo{
					Port:    port,
					PID:     pid,
					Process: proc,
					Proto:   "TCP",
				})
			}
		}
	}
	return ports, nil
}

func parsePort(addr string) int {
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		return 0
	}
	portStr := parts[len(parts)-1]
	p, _ := strconv.Atoi(portStr)
	return p
}

// KillPortProcess terminates the process occupying the specified port.
func KillPortProcess(ctx context.Context, port int) error {
	ports, err := FindPorts(ctx, port)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return fmt.Errorf("no process found listening on port %d", port)
	}
	killed := map[int]bool{}
	for _, p := range ports {
		if p.PID <= 1 || killed[p.PID] {
			continue // safeguard against init/launchd; one kill per process
		}
		killed[p.PID] = true
		var killCmd string
		if goos == "windows" {
			killCmd = fmt.Sprintf("Stop-Process -Id %d -Force", p.PID)
		} else {
			killCmd = fmt.Sprintf("kill -9 %d", p.PID)
		}
		res := run(ctx, killCmd, 5*time.Second)
		if res.ExitCode != 0 {
			return fmt.Errorf("failed to terminate PID %d (%s) on port %d: %s", p.PID, p.Process, port, res.Output)
		}
	}
	return nil
}

// proxyAddress is the host and port a proxy variable points to. The value
// is a URL, with or without a scheme, and the port may be left out.
func proxyAddress(value string) (string, error) {
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" {
		return "", errors.New("no host in it")
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "socks", "socks4", "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port), nil
}

// stockHost reports whether a hosts entry is one the system ships with.
func stockHost(ip, host string) bool {
	switch host {
	case "localhost":
		return ip == "127.0.0.1" || ip == "::1" || strings.HasPrefix(ip, "fe80::1%")
	case "broadcasthost":
		return ip == "255.255.255.255"
	case "ip6-localhost", "ip6-loopback":
		return ip == "::1"
	case "ip6-localnet":
		return ip == "fe00::0"
	case "ip6-mcastprefix":
		return ip == "ff00::0"
	case "ip6-allnodes":
		return ip == "ff02::1"
	case "ip6-allrouters":
		return ip == "ff02::2"
	}
	// Debian and Ubuntu give the machine's own name an address.
	if ip == "127.0.1.1" {
		if name, err := os.Hostname(); err == nil {
			short, _, _ := strings.Cut(name, ".")
			return host == name || host == short
		}
	}
	return false
}

// AuditHostsAndProxies checks /etc/hosts and environment proxy variables.
func AuditHostsAndProxies() (*NetworkAudit, error) {
	hostsPath := hostsFile()

	audit := &NetworkAudit{
		HostsFile: hostsPath,
		ProxyEnv:  make(map[string]string),
	}

	// Check proxy variables
	for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		if val := os.Getenv(key); val != "" {
			audit.ProxyEnv[key] = val
			if strings.Contains(key, "HTTP") || strings.Contains(key, "http") {
				// Test if proxy host is reachable
				addr, err := proxyAddress(val)
				if err != nil {
					audit.Warnings = append(audit.Warnings, fmt.Sprintf("Proxy environment variable %s is not a proxy address (%v)", key, err))
					continue
				}
				if conn, err := net.DialTimeout("tcp", addr, 1*time.Second); err != nil {
					audit.Warnings = append(audit.Warnings, fmt.Sprintf("Proxy environment variable %s points to %s, which cannot be reached (%v)", key, addr, err))
				} else {
					conn.Close()
				}
			}
		}
	}

	// Read hosts file
	f, err := os.Open(hostsPath)
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ip := fields[0]
				for _, host := range fields[1:] {
					if strings.HasPrefix(host, "#") {
						break
					}
					if stockHost(ip, host) {
						continue
					}
					audit.CustomHosts = append(audit.CustomHosts, HostsEntry{
						IP:       ip,
						Hostname: host,
					})
				}
			}
		}
	}

	return audit, nil
}

func filepathJoin(elem ...string) string {
	return strings.Join(elem, string(os.PathSeparator))
}
