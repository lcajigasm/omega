package aiagents

import "testing"

func TestParseStat(t *testing.T) {
	// The name may hold spaces and parentheses.
	line := "4242 (my (odd) name) S 17 4242 4242 0 -1 4194304 100 0 0 0 1 2 0 0 20 0 1 0 98765 1000 10 18446744073709551615"
	p := parseStat(4242, line)
	if p == nil || p.ppid != 17 || p.comm != "my (odd) name" || p.startTicks != 98765 {
		t.Fatalf("parseStat = %+v", p)
	}
	if parseStat(1, "garbage") != nil {
		t.Error("garbage parsed")
	}
}

func TestMarkers(t *testing.T) {
	env := []byte("PATH=/bin\x00CLAUDECODE=1\x00CLAUDE_CODE_SESSION_ID=abc\x00CLAUDE_PID=77\x00X=CLAUDECODE=1\x00")
	m := markers(env)
	if m["CLAUDECODE"] != "1" || m["CLAUDE_CODE_SESSION_ID"] != "abc" || m["CLAUDE_PID"] != "77" || len(m) != 3 {
		t.Fatalf("markers = %v", m)
	}
	if markers([]byte("PATH=/bin\x00")) != nil {
		t.Error("markers found where there are none")
	}
}
