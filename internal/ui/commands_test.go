package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func TestSuggestions(t *testing.T) {
	m := &model{input: textinput.New(), header: Header{Model: "fast", Models: []string{"fast", "smart"}}}
	labels := func() []string {
		var out []string
		for _, s := range m.suggestions() {
			out = append(out, s.value)
		}
		return out
	}
	for in, want := range map[string][]string{
		"hello":     nil,
		"/":         {"/help", "/mode", "/model", "/expand", "/exit", "/quit"},
		"/mo":       {"/mode", "/model"},
		"/model ":   {"/model fast", "/model smart"},
		"/model sm": {"/model smart"},
		"/model a":  {"/model fast", "/model smart"},
		"/mode r":   {"/mode readonly"},
		"/zz":       nil,
	} {
		m.input.SetValue(in)
		got := labels()
		if len(got) != len(want) {
			t.Errorf("%q: got %v, want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q: got %v, want %v", in, got, want)
				break
			}
		}
	}
	m.input.SetValue("/mo")
	m.complete(m.suggestions()[1])
	if m.input.Value() != "/model " {
		t.Fatalf("completing a command with arguments leaves room for one: %q", m.input.Value())
	}
}
