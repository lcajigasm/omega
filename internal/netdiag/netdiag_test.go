package netdiag

import (
	"context"
	"testing"
)

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
