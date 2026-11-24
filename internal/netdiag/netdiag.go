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
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// Registry is a place net ping checks. OS, when not empty, lists the
// systems where it matters: Ubuntu's mirrors say nothing about a Mac.
type Registry struct {
	Name string
	URL  string
	OS   []string
}

// DefaultRegistries to check in net ping. The URLs answer 200 to a plain
// request, so a code of 400 or more says something about the service.
var DefaultRegistries = []Registry{
	{"Homebrew", "https://formulae.brew.sh", []string{"darwin", "linux"}},
	{"npm", "https://registry.npmjs.org", nil},
	{"PyPI", "https://pypi.org", nil},
	{"crates.io", "https://crates.io", nil},
	{"Docker Hub", "https://hub.docker.com", nil},
	{"GitHub", "https://api.github.com", nil},
	{"Go Packages", "https://proxy.golang.org", nil},
	{"Ubuntu Mirrors", "https://archive.ubuntu.com", []string{"linux"}},
	{"winget", "https://cdn.winget.microsoft.com/cache/source.msix", []string{"windows"}},
}

// Registries are the ones that matter on this system.
func Registries() []Registry {
	var out []Registry
	for _, r := range DefaultRegistries {
		if len(r.OS) == 0 || slices.Contains(r.OS, goos) {
			out = append(out, r)
		}
	}
	return out
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

// PingRegistries tests reachability and latency against the registries
// of this system, all at once. Any HTTP answer means the service can be
// reached; Status says which.
func PingRegistries(ctx context.Context) []RegistryPing {
	regs := Registries()
	results := make([]RegistryPing, len(regs))
	client := &http.Client{Timeout: 4 * time.Second}
	var wg sync.WaitGroup
	for i, reg := range regs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = ping(ctx, client, reg)
		}()
	}
	wg.Wait()
	return results
}

func ping(ctx context.Context, client *http.Client, reg Registry) RegistryPing {
	try := func(method string) (int, time.Duration, error) {
		req, err := http.NewRequestWithContext(ctx, method, reg.URL, nil)
		if err != nil {
			return 0, 0, err
		}
		req.Header.Set("User-Agent", "omega/1.0 (network-check)")
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return 0, time.Since(start), err
		}
		resp.Body.Close()
		return resp.StatusCode, time.Since(start), nil
	}
	code, latency, err := try(http.MethodHead)
	// Some servers refuse HEAD (405, or 404 as crates.io does) or drop
	// it: GET says more.
	if err != nil || code >= 400 {
		if c, l, gerr := try(http.MethodGet); gerr == nil {
			code, latency, err = c, l, nil
		}
	}
	if err != nil {
		return RegistryPing{Name: reg.Name, URL: reg.URL, Latency: latency, Error: err.Error()}
	}
	return RegistryPing{Name: reg.Name, URL: reg.URL, Reachable: true, Latency: latency, Status: code}
}

// FindPorts finds the processes listening on a port, or on every port when
// targetPort is 0, sorted by port. proto is "tcp" (listening sockets) or
// "udp" (bound sockets).
func FindPorts(ctx context.Context, targetPort int, proto string) ([]PortInfo, error) {
	var ports []PortInfo
	var err error
	if goos == "windows" {
		ports, err = findPortsWindows(ctx, targetPort, proto)
	} else {
		ports, err = findPortsUnix(ctx, targetPort, proto)
	}
	sort.SliceStable(ports, func(i, j int) bool {
		if ports[i].Port != ports[j].Port {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].PID < ports[j].PID
	})
	return ports, err
}

func findPortsUnix(ctx context.Context, targetPort int, proto string) ([]PortInfo, error) {
	if !has("lsof") {
		return nil, errors.New("lsof is not installed, so the listening ports cannot be read")
	}
	// TCP sockets that are listening, and nothing else: "-i :PORT" alone
	// also lists UDP sockets and outgoing connections on that port, and
	// `net kill` would end those processes. +c 0 prints the whole name of
	// the program ("ControlCenter", not "ControlCe").
	udp := strings.EqualFold(proto, "udp")
	cmdStr := "lsof -iTCP -sTCP:LISTEN -P -n +c 0"
	switch {
	case udp && targetPort > 0:
		cmdStr = fmt.Sprintf("lsof -iUDP:%d -P -n +c 0", targetPort)
	case udp:
		cmdStr = "lsof -iUDP -P -n +c 0"
	case targetPort > 0:
		cmdStr = fmt.Sprintf("lsof -iTCP:%d -sTCP:LISTEN -P -n +c 0", targetPort)
	}

	// lsof ends with status 1 when it finds nothing, so only what it
	// printed counts.
	res := run(ctx, cmdStr, 5*time.Second)
	var ports []PortInfo
	seen := map[string]bool{}
	want := "TCP"
	if udp {
		want = "UDP"
	}
	for i, line := range strings.Split(res.Output, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header
		}
		// COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME (STATE)
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[7] != want {
			continue
		}
		if !udp && len(fields) > 9 && fields[9] != "(LISTEN)" {
			continue
		}
		if strings.Contains(fields[8], "->") {
			continue // a connected socket, not one waiting on the port
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
		// lsof writes a space in a name as \x20.
		name := strings.ReplaceAll(fields[0], `\x20`, " ")
		ports = append(ports, PortInfo{Port: port, PID: pid, Process: name, User: fields[2], Proto: want})
	}
	return ports, nil
}

func findPortsWindows(ctx context.Context, targetPort int, proto string) ([]PortInfo, error) {
	filter := ""
	if targetPort > 0 {
		filter = fmt.Sprintf("Where-Object { $_.LocalPort -eq %d } | ", targetPort)
	}
	source, want := "Get-NetTCPConnection -State Listen | ", "TCP"
	if strings.EqualFold(proto, "udp") {
		source, want = "Get-NetUDPEndpoint | ", "UDP"
	}
	script := source + filter +
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
					Proto:   want,
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

// Seams of StopProcess, replaced by the tests: signalProcess asks a
// process to exit (or, with force, ends it), and processAlive says whether
// it still runs.
var (
	signalProcess = osSignal
	processAlive  = osAlive
)

// StopGrace is how long a process gets to exit before it is forced to.
const StopGrace = 3 * time.Second

// StopProcess asks a process to exit (SIGTERM; taskkill without /F on
// Windows), waits up to grace for it to go, and only then forces it
// (SIGKILL; taskkill /F). forced says whether it had to be.
func StopProcess(ctx context.Context, pid int, grace time.Duration) (forced bool, err error) {
	if pid <= 1 || pid == os.Getpid() {
		return false, fmt.Errorf("PID %d is not one omega ends", pid)
	}
	// When asking fails (another user's process, say), forcing is still
	// tried, and says why when it fails too.
	_ = signalProcess(ctx, pid, false)
	if waitGone(pid, grace) {
		return false, nil
	}
	if err := signalProcess(ctx, pid, true); err != nil && processAlive(pid) {
		return true, fmt.Errorf("PID %d could not be ended: %w", pid, err)
	}
	if waitGone(pid, time.Second) {
		return true, nil
	}
	return true, fmt.Errorf("PID %d is still running", pid)
}

// waitGone waits up to d for pid to exit and says whether it did.
func waitGone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if !processAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// hideUserinfo replaces the user and password of a URL with "***".
func hideUserinfo(v string) string {
	scheme, rest, ok := strings.Cut(v, "://")
	if !ok {
		rest, scheme = v, ""
	}
	at := strings.LastIndex(rest, "@")
	slash := strings.Index(rest, "/")
	if at < 0 || (slash >= 0 && slash < at) {
		return v
	}
	rest = "***@" + rest[at+1:]
	if scheme != "" {
		return scheme + "://" + rest
	}
	return rest
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
			// A proxy address may carry a user and a password
			// (http://user:pass@host:3128): they are not shown.
			audit.ProxyEnv[key] = hideUserinfo(val)
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
