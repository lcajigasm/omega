package cli

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"
)

// The budgets of a local probe. Connecting is quick when something listens
// and refused at once when nothing does, so the dial budget stays short. The
// answer can take much longer: a local proxy in front of a remote model
// spends most of a second on the round trip to its upstream (#54).
var (
	localDialBudget  = time.Second
	localProbeBudget = 3 * time.Second
)

// localModels probes a local OpenAI-compatible /models endpoint. up is false
// only when nothing accepts the connection, the request fails after it, or
// the server answers 5xx. A server that took the request but did not answer
// within the budget is up: it is there and slow, which is what a local proxy
// in front of a remote provider looks like. A body that is not a model list,
// or that did not arrive in time, leaves ids empty.
func localModels(base string) (ids []string, up bool) {
	ctx, cancel := context.WithTimeout(context.Background(), localProbeBudget)
	defer cancel()
	var sent atomic.Bool
	trace := &httptrace.ClientTrace{WroteRequest: func(i httptrace.WroteRequestInfo) {
		if i.Err == nil {
			sent.Store(true)
		}
	}}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, strings.TrimRight(base, "/")+"/models", nil)
	if err != nil {
		return nil, false
	}
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: localDialBudget}).DialContext,
		TLSHandshakeTimeout: localDialBudget,
	}
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return nil, sent.Load() && ctx.Err() != nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return nil, false
	}
	if resp.StatusCode >= 400 {
		return nil, true
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, true
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &out) != nil {
		return nil, true
	}
	for _, m := range out.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, true
}
