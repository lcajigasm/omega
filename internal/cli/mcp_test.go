package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func mcpSession(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := serveMCP(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var replies []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %q", l)
		}
		replies = append(replies, m)
	}
	return replies
}

func TestMCPHandshakeAndTools(t *testing.T) {
	r := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
	)
	if len(r) != 3 {
		t.Fatalf("%d replies, want 3 (a notification gets none)", len(r))
	}
	if v := r[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-03-26" {
		t.Errorf("protocol %v: an older client's version must be kept", v)
	}
	tools := r[1]["result"].(map[string]any)["tools"].([]any)
	for _, tool := range tools {
		m := tool.(map[string]any)
		ann := m["annotations"].(map[string]any)
		name := m["name"].(string)
		writes := slices.Contains([]string{"omega_set_limit", "omega_apply_check", "omega_free_space"}, name)
		if ann["readOnlyHint"] == writes {
			t.Errorf("%s: readOnlyHint=%v", name, ann["readOnlyHint"])
		}
		if name == "omega_free_space" && ann["destructiveHint"] != true {
			t.Error("omega_free_space is not marked destructive")
		}
	}
	if r[2]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Errorf("unknown method: %v", r[2])
	}
}

func TestMCPRunsOnlyPlainOmegaCommands(t *testing.T) {
	var ran [][]string
	old := runOmega
	defer func() { runOmega = old }()
	runOmega = func(_ context.Context, args ...string) (string, error) {
		ran = append(ran, args)
		return "ok", nil
	}
	r := mcpSession(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"omega_set_limit","arguments":{"check":"--yes","value":"1"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"omega_help","arguments":{"command":"disk limits; rm -rf ~"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"omega_free_space","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"omega_set_limit","arguments":{"check":"docker.logs","value":"10m"}}}`,
	)
	for i := 0; i < 2; i++ {
		if r[i]["result"].(map[string]any)["isError"] != true {
			t.Errorf("call %d was not refused: %v", i+1, r[i])
		}
	}
	want := [][]string{{"disk", "limits", "enforce", "--dry-run"}, {"disk", "limits", "set", "docker.logs", "10m"}}
	if len(ran) != len(want) {
		t.Fatalf("ran %v, want %v", ran, want)
	}
	for i := range want {
		if !slices.Equal(ran[i], want[i]) {
			t.Errorf("ran %v, want %v", ran[i], want[i])
		}
	}
}

func TestManualSearch(t *testing.T) {
	hits := searchManual("limitar el disco de docker", 3)
	if len(hits) == 0 {
		t.Fatal("no section found")
	}
	found := false
	for _, h := range hits {
		if strings.Contains(strings.ToLower(h.body), "omega disk limits") {
			found = true
		}
	}
	if !found {
		t.Errorf("a Spanish question about Docker's disk does not reach the disk limits: %v", hits[0].heading)
	}
}
