package netdiag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lcajigasm/omega/internal/runner"
)

// fake replaces the system: commands are recorded and answered by answer,
// and the programs in installed are the ones found.
func fake(t *testing.T, system string, installed []string, answer func(command string) runner.Result) *[]string {
	t.Helper()
	oldOS, oldRun, oldHas := goos, run, has
	t.Cleanup(func() { goos, run, has = oldOS, oldRun, oldHas })
	goos = system
	var ran []string
	run = func(_ context.Context, command string, _ time.Duration) runner.Result {
		ran = append(ran, command)
		return answer(command)
	}
	has = func(bin string) bool {
		for _, b := range installed {
			if b == bin {
				return true
			}
		}
		return false
	}
	return &ran
}

// Flushing says what happened: half a flush on a Mac and no flush on Linux
// are errors, not "flushed successfully".
func TestFlushDNSIsHonest(t *testing.T) {
	ctx := context.Background()
	fake(t, "darwin", nil, func(c string) runner.Result {
		if strings.HasPrefix(c, "sudo ") {
			return runner.Result{ExitCode: 1, Output: "sudo: a password is required"}
		}
		return runner.Result{}
	})
	if msg, err := FlushDNS(ctx); err == nil || !strings.Contains(err.Error(), "administrator") || !strings.Contains(err.Error(), "mDNSResponder") {
		t.Errorf("macOS without sudo: %q, %v", msg, err)
	}
	fake(t, "darwin", nil, func(string) runner.Result { return runner.Result{} })
	if msg, err := FlushDNS(ctx); err != nil || !strings.Contains(msg, "mDNSResponder") {
		t.Errorf("macOS: %q, %v", msg, err)
	}

	fake(t, "linux", []string{"resolvectl", "nscd"}, func(string) runner.Result { return runner.Result{ExitCode: 1, Output: "access denied"} })
	if msg, err := FlushDNS(ctx); err == nil || !strings.Contains(err.Error(), "administrator") {
		t.Errorf("Linux with every tool failing: %q, %v", msg, err)
	}
	fake(t, "linux", nil, func(string) runner.Result { return runner.Result{} })
	if msg, err := FlushDNS(ctx); err == nil {
		t.Errorf("Linux with no cache service: %q", msg)
	}
	ran := fake(t, "linux", []string{"resolvectl"}, func(string) runner.Result { return runner.Result{} })
	if msg, err := FlushDNS(ctx); err != nil || !strings.Contains(msg, "resolvectl") || len(*ran) != 1 {
		t.Errorf("Linux: %q, %v, %q", msg, err, *ran)
	}
}

const lsofOutput = `COMMAND   PID USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
node    4242 luis   23u  IPv4 0x1234567890abcdef      0t0  TCP *:3000 (LISTEN)
node    4242 luis   24u  IPv6 0x1234567890abcdee      0t0  TCP *:3000 (LISTEN)
mDNSResp 300 _mdns   8u  IPv4 0x1234567890abcded      0t0  UDP *:3000
curl    5151 luis    5u  IPv4 0x1234567890abcdec      0t0  TCP 127.0.0.1:51000->127.0.0.1:3000 (ESTABLISHED)
`

// Only TCP listeners are ports in use: a UDP socket or a client connected
// to the port is not what `net kill` is for.
func TestFindPortsOnlyTCPListeners(t *testing.T) {
	ctx := context.Background()
	ran := fake(t, "darwin", []string{"lsof"}, func(c string) runner.Result {
		if strings.HasPrefix(c, "lsof") {
			return runner.Result{Output: lsofOutput}
		}
		return runner.Result{}
	})
	ports, err := FindPorts(ctx, 3000)
	if err != nil || len(ports) != 1 || ports[0].PID != 4242 || ports[0].Proto != "TCP" || ports[0].Process != "node" {
		t.Fatalf("ports: %+v, %v", ports, err)
	}
	if (*ran)[0] != "lsof -iTCP:3000 -sTCP:LISTEN -P -n" {
		t.Errorf("command: %s", (*ran)[0])
	}
	if err := KillPortProcess(ctx, 3000); err != nil {
		t.Fatal(err)
	}
	var kills []string
	for _, c := range *ran {
		if strings.HasPrefix(c, "kill") {
			kills = append(kills, c)
		}
	}
	if len(kills) != 1 || kills[0] != "kill -9 4242" {
		t.Errorf("kills: %q", kills)
	}

	// Without lsof nothing can be said, and saying "no ports" would be wrong.
	fake(t, "linux", nil, func(string) runner.Result { return runner.Result{ExitCode: 127} })
	if ports, err := FindPorts(ctx, 0); err == nil {
		t.Errorf("without lsof: %+v and no error", ports)
	}
}

func TestProxyAddress(t *testing.T) {
	for in, want := range map[string]string{
		"http://localhost":              "localhost:80",
		"https://proxy.corp":            "proxy.corp:443",
		"http://proxy.corp:3128/":       "proxy.corp:3128",
		"proxy.corp:3128":               "proxy.corp:3128",
		"http://user:secret@proxy:8080": "proxy:8080",
		"socks5://127.0.0.1":            "127.0.0.1:1080",
		"http://[::1]:8080":             "[::1]:8080",
	} {
		if got, err := proxyAddress(in); err != nil || got != want {
			t.Errorf("proxyAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

// The hosts file as macOS and Ubuntu ship it has nothing custom in it.
func TestAuditIgnoresStockHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	stock := "##\n# Host Database\n##\n127.0.0.1\tlocalhost\n255.255.255.255\tbroadcasthost\n::1             localhost\nfe80::1%lo0\tlocalhost\n" +
		"::1     ip6-localhost ip6-loopback\nfe00::0 ip6-localnet\nff00::0 ip6-mcastprefix\nff02::1 ip6-allnodes\nff02::2 ip6-allrouters\n" +
		"10.0.0.5 build.internal # ours\n"
	if err := os.WriteFile(path, []byte(stock), 0o644); err != nil {
		t.Fatal(err)
	}
	old := hostsFile
	defer func() { hostsFile = old }()
	hostsFile = func() string { return path }
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
	audit, err := AuditHostsAndProxies()
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.CustomHosts) != 1 || audit.CustomHosts[0].Hostname != "build.internal" {
		t.Errorf("custom hosts: %+v", audit.CustomHosts)
	}
}

func TestParsePort(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"*:3000", 3000},
		{"127.0.0.1:8080", 8080},
		{"[::1]:5432", 5432},
		{"invalid", 0},
	}
	for _, tc := range cases {
		got := parsePort(tc.in)
		if got != tc.want {
			t.Errorf("parsePort(%q) = %d; want %d", tc.in, got, tc.want)
		}
	}
}

func TestAuditHostsAndProxies(t *testing.T) {
	audit, err := AuditHostsAndProxies()
	if err != nil {
		t.Fatalf("AuditHostsAndProxies failed: %v", err)
	}
	if audit.HostsFile == "" {
		t.Errorf("expected HostsFile path to not be empty")
	}
}

func TestPingRegistries(t *testing.T) {
	// Simple smoke test with cancelled or quick context so it doesn't hang in CI/offline
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	results := PingRegistries(ctx)
	if len(results) == 0 {
		t.Errorf("expected results from PingRegistries")
	}
}
