package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestShortcutsCast(t *testing.T) {
	path := "../../docs/assets/shortcuts.cast"
	if os.Getenv("OMEGA_UPDATE_CAST") == "1" {
		m := testModel(t)
		data, err := shortcutsCast(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var header struct {
		Version int    `json:"version"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
		Title   string `json:"title"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header.Version != 2 || header.Width < 80 || header.Height < 24 {
		t.Fatalf("header %+v", header)
	}
	var inputs []string
	var sawHelp, sawConfirm, sawDenied bool
	helpBeforeConfirm := false
	for _, line := range lines[1:] {
		var ev []json.RawMessage
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		var kind, data string
		if err := json.Unmarshal(ev[1], &kind); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(ev[2], &data); err != nil {
			t.Fatal(err)
		}
		if kind == "i" {
			inputs = append(inputs, data)
			continue
		}
		plain := stripANSI(data)
		switch {
		case strings.Contains(plain, "Keyboard shortcuts") && strings.Contains(plain, "ctrl+y"):
			sawHelp = true
		case strings.Contains(plain, "Run this?"):
			if sawHelp {
				helpBeforeConfirm = true
			}
			sawConfirm = true
		case strings.Contains(plain, "denied") && !strings.Contains(plain, "Run this?"):
			sawDenied = true
		}
	}
	if len(inputs) != 3 || inputs[0] != "?" || inputs[1] != "\x1b" || inputs[2] != "n" {
		t.Fatalf("inputs %q", inputs)
	}
	if !sawHelp || !helpBeforeConfirm || !sawConfirm || !sawDenied {
		t.Fatalf("help=%v before=%v confirm=%v denied=%v", sawHelp, helpBeforeConfirm, sawConfirm, sawDenied)
	}
}
