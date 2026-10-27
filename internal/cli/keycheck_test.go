package cli

import "testing"

func TestCleanKey(t *testing.T) {
	for in, want := range map[string]string{
		"  sk-ant-abc123  \n":             "sk-ant-abc123",
		"\x1b[200~sk-ant-abc123\x1b[201~": "sk-ant-abc123",
		`"sk-or-v1-abc"`:                  "sk-or-v1-abc",
	} {
		if got := cleanKey(in); got != want {
			t.Errorf("cleanKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyLooksWrong(t *testing.T) {
	for key, want := range map[string]bool{
		"https://lcajigasm.github.io/omega/":        true,
		"sk-ant-api03 with spaces in the middle":    true,
		"short":                                     true,
		"sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz0":  false,
		"sk-or-v1-0123456789abcdef0123456789abcdef": false,
	} {
		if got := keyLooksWrong(key); got != want {
			t.Errorf("keyLooksWrong(%q) = %v, want %v", key, got, want)
		}
	}
}
