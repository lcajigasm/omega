package gui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// The runtime call the window makes to open a link in the browser. Its
// numbers are Wails's own (pkg/application: browserRequest = 9,
// BrowserOpenURL = 0).
const (
	browserObject = 9
	openURLMethod = 0
)

// maxRuntimeBody bounds what the guard reads of one runtime call.
const maxRuntimeBody = 8 << 20

// URLGuard refuses a runtime call that asks the system to open anything but
// a web page. The window's own code only opens http(s) links, but the
// runtime call is there for any script in the page, and the system would
// open file://, smb:// or an app's own scheme just the same. Wails sends a
// large call in chunks, so those are put together before the decision,
// which is taken on the last one: the call never reaches Wails.
type URLGuard struct {
	mu     sync.Mutex
	chunks map[string]map[int][]byte
}

// Allow reads the call (and leaves its body readable for Wails) and
// reports whether it may go on.
func (g *URLGuard) Allow(r *http.Request) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRuntimeBody+1))
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) > maxRuntimeBody {
		return false
	}
	if id := r.Header.Get("x-wails-chunk-id"); id != "" {
		idx, err1 := strconv.Atoi(r.Header.Get("x-wails-chunk-index"))
		total, err2 := strconv.Atoi(r.Header.Get("x-wails-chunk-total"))
		if err1 != nil || err2 != nil || total < 1 || total > 1024 || idx < 0 || idx >= total {
			return false
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.chunks == nil {
			g.chunks = map[string]map[int][]byte{}
		}
		if g.chunks[id] == nil {
			if len(g.chunks) > 64 {
				return false
			}
			g.chunks[id] = map[int][]byte{}
		}
		g.chunks[id][idx] = body
		if len(g.chunks[id]) < total {
			return true // not complete: Wails waits for the rest too
		}
		var all []byte
		for i := 0; i < total; i++ {
			all = append(all, g.chunks[id][i]...)
		}
		delete(g.chunks, id)
		return OpenURLAllowed(all, nil)
	}
	return OpenURLAllowed(body, r.URL.Query())
}

// OpenURLAllowed reports whether a runtime call is fine to pass: anything
// that is not Browser.OpenURL, or Browser.OpenURL of an http or https
// address. query is the fallback some webviews post the call in.
func OpenURLAllowed(body []byte, query url.Values) bool {
	var req struct {
		Object *int            `json:"object"`
		Method *int            `json:"method"`
		Args   json.RawMessage `json:"args"`
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if json.Unmarshal(body, &req) != nil {
			return true // Wails refuses what it cannot parse
		}
	} else if query != nil {
		if o, err := strconv.Atoi(query.Get("object")); err == nil {
			req.Object = &o
		}
		if m, err := strconv.Atoi(query.Get("method")); err == nil {
			req.Method = &m
		}
		req.Args = json.RawMessage(query.Get("args"))
	}
	if req.Object == nil || *req.Object != browserObject {
		return true
	}
	if req.Method == nil || *req.Method != openURLMethod {
		return false // the browser object has no other method
	}
	var args struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(req.Args, &args) != nil {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
