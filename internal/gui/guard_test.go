package gui

import (
	"bytes"
	"io"
	"net/http/httptest"
	"testing"
)

// Third review: Browser.OpenURL opened any scheme a script in the page asked for.
func TestOpenURLGuard(t *testing.T) {
	for body, want := range map[string]bool{
		`{"object":9,"method":0,"args":{"url":"https://omega.example/docs"}}`: true,
		`{"object":9,"method":0,"args":{"url":"file:///etc/passwd"}}`:         false,
		`{"object":9,"method":0,"args":{"url":"smb://host/share"}}`:           false,
		`{"object":9,"method":0,"args":{"url":"x-apple.systempreferences:"}}`: false,
		`{"object":9,"method":0,"args":{"url":"http:/nohost"}}`:               false,
		`{"object":0,"method":1,"args":{"name":"main.App.Runs"}}`:             true,
	} {
		if got := OpenURLAllowed([]byte(body), nil); got != want {
			t.Errorf("%s: %v", body, got)
		}
	}
	// A call sent in chunks is judged when it is complete, and its body is
	// still there for Wails.
	g := &URLGuard{}
	full := []byte(`{"object":9,"method":0,"args":{"url":"file:///etc/passwd"}}`)
	parts := [][]byte{full[:20], full[20:]}
	for i, p := range parts {
		r := httptest.NewRequest("POST", "/wails/runtime", bytes.NewReader(p))
		r.Header.Set("x-wails-chunk-id", "c1")
		r.Header.Set("x-wails-chunk-index", string(rune('0'+i)))
		r.Header.Set("x-wails-chunk-total", "2")
		ok := g.Allow(r)
		if i == 0 && !ok || i == 1 && ok {
			t.Fatalf("chunk %d: %v", i, ok)
		}
		if b, _ := io.ReadAll(r.Body); !bytes.Equal(b, p) {
			t.Fatal("the body must stay readable")
		}
	}
}
