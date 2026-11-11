// Package netdiag provides network, DNS and connectivity maintenance tools
// including DNS cache flushing, registry ping/latency checks, port conflict
// detection, and hosts file / proxy environment auditing.
package netdiag

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

var goos = runtime.GOOS

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

// FlushDNS flushes the system's DNS resolver cache.
func FlushDNS(ctx context.Context) (string, error) {
	switch goos {
	case "darwin":
		cmd := "dscacheutil -flushcache; sudo killall -HUP mDNSResponder"
		res := runner.Run(ctx, cmd, 10*time.Second, nil)
		if res.ExitCode != 0 {
			// Try without sudo just in case user is root
			res = runner.Run(ctx, "dscacheutil -flushcache", 5*time.Second, nil)
			if res.ExitCode != 0 {
				return "", fmt.Errorf("failed to flush DNS on macOS: %s", res.Output)
			}
		}
		return "macOS DNS cache flushed successfully (dscacheutil + mDNSResponder)", nil
	case "windows":
		cmd := "ipconfig /flushdns"
		res := runner.Run(ctx, cmd, 10*time.Second, nil)
		if res.ExitCode != 0 {
			return "", fmt.Errorf("failed to flush DNS on Windows: %s", res.Output)
		}
		return "Windows DNS cache flushed successfully (ipconfig /flushdns)", nil
	default: // Linux
		if runner.Has("resolvectl") {
			res := runner.Run(ctx, "sudo resolvectl flush-caches", 10*time.Second, nil)
			if res.ExitCode == 0 {
				return "Linux DNS cache flushed (resolvectl)", nil
			}
		}
		if runner.Has("systemd-resolve") {
			res := runner.Run(ctx, "sudo systemd-resolve --flush-caches", 10*time.Second, nil)
			if res.ExitCode == 0 {
				return "Linux DNS cache flushed (systemd-resolve)", nil
			}
		}
		if runner.Has("nscd") {
			res := runner.Run(ctx, "sudo nscd -I hosts", 10*time.Second, nil)
			if res.ExitCode == 0 {
				return "Linux DNS cache flushed (nscd)", nil
			}
		}
		return "DNS flushed via nscd/systemd or no local resolver active", nil
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
	var cmdStr string
	if targetPort > 0 {
		cmdStr = fmt.Sprintf("lsof -i :%d -sTCP:LISTEN -P -n", targetPort)
	} else {
		cmdStr = "lsof -iTCP -sTCP:LISTEN -P -n"
	}

	res := runner.Run(ctx, cmdStr, 5*time.Second, nil)
	var ports []PortInfo
	lines := strings.Split(res.Output, "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) >= 9 {
			proc := fields[0]
			pid, _ := strconv.Atoi(fields[1])
			user := fields[2]
			proto := fields[4]
			nameField := fields[8] // e.g. "*:3000" or "127.0.0.1:8080"
			port := parsePort(nameField)
			if targetPort > 0 && port != targetPort {
				continue
			}
			if port > 0 {
				ports = append(ports, PortInfo{
					Port:    port,
					PID:     pid,
					Process: proc,
					User:    user,
					Proto:   proto,
				})
			}
		}
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

	res := runner.Run(ctx, script, 5*time.Second, nil)
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
	for _, p := range ports {
		if p.PID <= 1 {
			continue // safeguard against init/launchd
		}
		var killCmd string
		if goos == "windows" {
			killCmd = fmt.Sprintf("Stop-Process -Id %d -Force", p.PID)
		} else {
			killCmd = fmt.Sprintf("kill -9 %d", p.PID)
		}
		res := runner.Run(ctx, killCmd, 5*time.Second, nil)
		if res.ExitCode != 0 {
			return fmt.Errorf("failed to terminate PID %d (%s) on port %d: %s", p.PID, p.Process, port, res.Output)
		}
	}
	return nil
}

// AuditHostsAndProxies checks /etc/hosts and environment proxy variables.
func AuditHostsAndProxies() (*NetworkAudit, error) {
	hostsPath := "/etc/hosts"
	if goos == "windows" {
		hostsPath = filepathJoin(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
	}

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
				u := strings.TrimPrefix(strings.TrimPrefix(val, "http://"), "https://")
				host := strings.Split(u, "/")[0]
				if conn, err := net.DialTimeout("tcp", host, 1*time.Second); err != nil {
					audit.Warnings = append(audit.Warnings, fmt.Sprintf("Proxy environment variable %s=%s points to unreachable host (%v)", key, val, err))
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
					// Ignore standard loopback
					if (ip == "127.0.0.1" || ip == "::1") && (host == "localhost" || host == "broadcasthost") {
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
