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
	ports, err := FindPorts(ctx, 3000, "tcp")
	if err != nil || len(ports) != 1 || ports[0].PID != 4242 || ports[0].Proto != "TCP" || ports[0].Process != "node" {
		t.Fatalf("ports: %+v, %v", ports, err)
	}
	if (*ran)[0] != "lsof -iTCP:3000 -sTCP:LISTEN -P -n +c 0" {
		t.Errorf("command: %s", (*ran)[0])
	}
	// --udp lists the bound UDP sockets, not the TCP ones.
	ports, err = FindPorts(ctx, 3000, "udp")
	if err != nil || len(ports) != 1 || ports[0].PID != 300 || ports[0].Proto != "UDP" {
		t.Fatalf("udp ports: %+v, %v", ports, err)
	}

	// Without lsof nothing can be said, and saying "no ports" would be wrong.
	fake(t, "linux", nil, func(string) runner.Result { return runner.Result{ExitCode: 127} })
	if ports, err := FindPorts(ctx, 0, "tcp"); err == nil {
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

// Third review: net audit printed a proxy's user and password.
func TestHideUserinfo(t *testing.T) {
	for in, want := range map[string]string{
		"http://user:secret@proxy:3128": "http://***@proxy:3128",
		"user:secret@proxy:3128":        "***@proxy:3128",
		"http://proxy:3128":             "http://proxy:3128",
		"localhost,.corp":               "localhost,.corp",
		"http://proxy/a@b":              "http://proxy/a@b",
	} {
		if got := hideUserinfo(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
}

// Every port, sorted by number, with the whole name of the program.
func TestFindPortsSortedWithWholeNames(t *testing.T) {
	fake(t, "darwin", []string{"lsof"}, func(string) runner.Result {
		return runner.Result{Output: `COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
ControlCenter 673 luis 9u IPv4 0x1 0t0 TCP *:7000 (LISTEN)
Google\x20Chrome 80 luis 9u IPv4 0x2 0t0 TCP 127.0.0.1:5000 (LISTEN)
`}
	})
	ports, err := FindPorts(context.Background(), 0, "tcp")
	if err != nil || len(ports) != 2 {
		t.Fatalf("ports: %+v, %v", ports, err)
	}
	if ports[0].Port != 5000 || ports[0].Process != "Google Chrome" || ports[1].Process != "ControlCenter" {
		t.Errorf("ports: %+v", ports)
	}
}

// A process is asked to exit first, and forced only when it does not.
func TestStopProcessAsksThenForces(t *testing.T) {
	oldSignal, oldAlive := signalProcess, processAlive
	t.Cleanup(func() { signalProcess, processAlive = oldSignal, oldAlive })
	var sent []bool
	alive := true
	stubborn := false
	signalProcess = func(_ context.Context, _ int, force bool) error {
		sent = append(sent, force)
		if force || !stubborn {
			alive = false
		}
		return nil
	}
	processAlive = func(int) bool { return alive }

	forced, err := StopProcess(context.Background(), 4242, 100*time.Millisecond)
	if err != nil || forced || len(sent) != 1 || sent[0] {
		t.Fatalf("polite stop: forced=%v err=%v signals=%v", forced, err, sent)
	}

	sent, alive, stubborn = nil, true, true
	forced, err = StopProcess(context.Background(), 4242, 100*time.Millisecond)
	if err != nil || !forced || len(sent) != 2 || sent[0] || !sent[1] {
		t.Fatalf("forced stop: forced=%v err=%v signals=%v", forced, err, sent)
	}

	if _, err := StopProcess(context.Background(), 1, time.Millisecond); err == nil {
		t.Error("PID 1 was not refused")
	}
}

// Only the registries of this system are checked.
func TestRegistriesBySystem(t *testing.T) {
	old := goos
	t.Cleanup(func() { goos = old })
	goos = "darwin"
	for _, r := range Registries() {
		if r.Name == "Ubuntu Mirrors" || r.Name == "winget" {
			t.Errorf("%s is checked on a Mac", r.Name)
		}
	}
	goos = "linux"
	found := false
	for _, r := range Registries() {
		found = found || r.Name == "Ubuntu Mirrors"
	}
	if !found {
		t.Error("Ubuntu's mirrors are not checked on Linux")
	}
}
