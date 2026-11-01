package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestShortcutsCast checks the asciinema published with the README. It must be
// a recording of the omega binary (scripts/capture.sh), not a screen rendered
// in-process. The in-process fixture used timestamp 1762000000.
func TestShortcutsCast(t *testing.T) {
	data, err := os.ReadFile("../../docs/assets/shortcuts.cast")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var header struct {
		Version   int    `json:"version"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		Timestamp int64  `json:"timestamp"`
		Title     string `json:"title"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header.Version != 2 || header.Width < 80 || header.Height < 24 {
		t.Fatalf("header %+v", header)
	}
	if header.Timestamp == 1762000000 {
		t.Fatal("shortcuts.cast is the in-process render; record it with scripts/capture.sh")
	}
	var plain strings.Builder
	for _, line := range lines[1:] {
		var ev []json.RawMessage
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		var kind, data string
		if err := json.Unmarshal(ev[1], &kind); err != nil {
			t.Fatal(err)
		}
		if kind != "o" {
			continue
		}
		if err := json.Unmarshal(ev[2], &data); err != nil {
			t.Fatal(err)
		}
		plain.WriteString(stripANSI(data))
	}
	text := plain.String()
	help := strings.Index(text, "Keyboard shortcuts")
	denied := strings.Index(strings.ToLower(text), "denied")
	if help < 0 || !strings.Contains(text, "ctrl+y") || denied < 0 || help > denied {
		t.Fatalf("help=%d denied=%d ctrl+y=%v", help, denied, strings.Contains(text, "ctrl+y"))
	}
	if !strings.Contains(text, "Run this?") {
		t.Fatal("recording has no approval card")
	}
}
