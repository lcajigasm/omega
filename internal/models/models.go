// Package models keeps a live catalogue of the language models on offer and
// picks, for a provider, the two omega uses in automatic mode: a fast one for
// routine work and a smart one for diagnosing failures.
//
// The catalogue is OpenRouter's public model list, which needs no key and
// carries, for every model, its list price and its benchmark scores
// (Artificial Analysis and Design Arena). Model names and prices change every
// few weeks; a table compiled into omega would be out of date at release.
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Source is where the catalogue comes from.
const Source = "https://openrouter.ai/api/v1/models"

// MaxAge is how long a catalogue is used before it is fetched again.
const MaxAge = 24 * time.Hour

// Model is one entry of the catalogue.
type Model struct {
	ID        string    `json:"id"` // OpenRouter id: vendor/name
	Name      string    `json:"name,omitempty"`
	Created   time.Time `json:"created,omitempty"`
	In        float64   `json:"in"`  // US dollars per million input tokens
	Out       float64   `json:"out"` // US dollars per million output tokens
	Score     float64   `json:"score,omitempty"`
	Estimated bool      `json:"estimated,omitempty"` // Score derived from other benchmarks
	Tools     bool      `json:"tools,omitempty"`
	Text      bool      `json:"text,omitempty"` // answers with text only
	Context   int       `json:"context,omitempty"`
	Expires   time.Time `json:"expires,omitempty"`
}

// Blended is the price of the mix omega sends: a maintenance run reads far
// more (command output, the history resent every turn) than it writes.
func (m Model) Blended() float64 { return (3*m.In + m.Out) / 4 }

// Picks are the automatic choices for one provider.
type Picks struct {
	Fast  string    `json:"fast,omitempty"`
	Smart string    `json:"smart,omitempty"`
	At    time.Time `json:"at,omitempty"`    // when they were last computed
	Since time.Time `json:"since,omitempty"` // when they last changed
}

// Store is the catalogue as kept on disk.
type Store struct {
	Fetched time.Time        `json:"fetched"`
	Models  []Model          `json:"models"`
	Picks   map[string]Picks `json:"picks,omitempty"`
}

// Load reads the stored catalogue; a missing or broken file is an empty one.
func Load(path string) *Store {
	s := &Store{}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, s) != nil {
			s = &Store{}
		}
	}
	if s.Picks == nil {
		s.Picks = map[string]Picks{}
	}
	return s
}

// Save writes the catalogue.
func (s *Store) Save(path string) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Fresh reports whether the catalogue is recent enough to use as is.
func (s *Store) Fresh(now time.Time) bool {
	return len(s.Models) > 0 && now.Sub(s.Fetched) < MaxAge
}

// Fetch downloads the catalogue. The request carries nothing about the user.
func Fetch(ctx context.Context, url string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "omega")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model catalogue: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

type rawModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Created       int64  `json:"created"`
	ContextLength int    `json:"context_length"`
	Expiration    string `json:"expiration_date"`
	Architecture  struct {
		Output []string `json:"output_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	Supported  []string `json:"supported_parameters"`
	Benchmarks struct {
		Arena []struct {
			Arena string  `json:"arena"`
			Elo   float64 `json:"elo"`
		} `json:"design_arena"`
		AA struct {
			Intelligence *float64 `json:"intelligence_index"`
			Coding       *float64 `json:"coding_index"`
			Agentic      *float64 `json:"agentic_index"`
		} `json:"artificial_analysis"`
	} `json:"benchmarks"`
}

// Parse reads OpenRouter's model list.
func Parse(data []byte) ([]Model, error) {
	var raw struct {
		Data []rawModel `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("model catalogue: %w", err)
	}
	if len(raw.Data) == 0 {
		return nil, fmt.Errorf("model catalogue: no models")
	}
	out := make([]Model, 0, len(raw.Data))
	// The other benchmarks, for the models the main index has not reached.
	var coding, agentic, elo []float64
	for _, r := range raw.Data {
		m := Model{ID: r.ID, Name: r.Name, Context: r.ContextLength}
		if r.Created > 0 {
			m.Created = time.Unix(r.Created, 0).UTC()
		}
		if t, err := time.Parse("2006-01-02", r.Expiration); err == nil {
			m.Expires = t
		}
		in, _ := strconv.ParseFloat(r.Pricing.Prompt, 64)
		outPrice, _ := strconv.ParseFloat(r.Pricing.Completion, 64)
		m.In, m.Out = round(in*1e6), round(outPrice*1e6)
		for _, p := range r.Supported {
			if p == "tools" {
				m.Tools = true
			}
		}
		m.Text = len(r.Architecture.Output) == 1 && r.Architecture.Output[0] == "text"
		if v := r.Benchmarks.AA.Intelligence; v != nil {
			m.Score = *v
		}
		out = append(out, m)
		coding = append(coding, deref(r.Benchmarks.AA.Coding))
		agentic = append(agentic, deref(r.Benchmarks.AA.Agentic))
		sum, n := 0.0, 0
		for _, a := range r.Benchmarks.Arena {
			if a.Arena == "models" {
				sum, n = sum+a.Elo, n+1
			}
		}
		if n > 0 {
			elo = append(elo, sum/float64(n))
		} else {
			elo = append(elo, 0)
		}
	}
	estimate(out, coding, agentic, elo)
	return out, nil
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func round(v float64) float64 {
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 4, 64), 64)
	return r
}

// estimate fills the score of models the main index has not measured from
// the benchmarks that have, each fitted against the main index on the models
// that carry both. The result is marked as an estimate.
func estimate(ms []Model, others ...[]float64) {
	type line struct {
		a, b float64
		ok   bool
	}
	fits := make([]line, len(others))
	for k, xs := range others {
		var n, sx, sy, sxx, sxy float64
		for i, m := range ms {
			if m.Score > 0 && xs[i] > 0 {
				n, sx, sy, sxx, sxy = n+1, sx+xs[i], sy+m.Score, sxx+xs[i]*xs[i], sxy+xs[i]*m.Score
			}
		}
		if d := n*sxx - sx*sx; n >= 8 && d != 0 {
			b := (n*sxy - sx*sy) / d
			// A benchmark that does not rise with the index says nothing.
			fits[k] = line{a: (sy - b*sx) / n, b: b, ok: b > 0}
		}
	}
	for i := range ms {
		if ms[i].Score > 0 {
			continue
		}
		sum, n := 0.0, 0
		for k, xs := range others {
			if fits[k].ok && xs[i] > 0 {
				sum, n = sum+fits[k].a+fits[k].b*xs[i], n+1
			}
		}
		if n > 0 && sum > 0 {
			ms[i].Score, ms[i].Estimated = round(sum/float64(n)), true
		}
	}
}

// key folds a model name for comparison across providers: lower case, no
// vendor, and dots as dashes (OpenRouter writes claude-haiku-5.5).
func key(id string) string {
	id = strings.ToLower(id)
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	return strings.ReplaceAll(id, ".", "-")
}

func vendor(id string) string {
	if i := strings.Index(id, "/"); i >= 0 {
		return id[:i]
	}
	return ""
}

// Candidate is a catalogue model as a provider names it.
type Candidate struct {
	Model
	Native string // the name to send to the provider
	Usable bool   // can be picked in automatic mode
}

// usable reports whether a model can run a maintenance session unattended:
// it calls tools, answers in text, has a measured price and is not a
// promotional, batch or about-to-be-removed route.
func usable(m Model, now time.Time) bool {
	if !m.Tools || !m.Text || m.Score <= 0 || (m.In <= 0 && m.Out <= 0) {
		return false
	}
	if strings.ContainsAny(m.ID, ":~") || (m.Context > 0 && m.Context < 64000) {
		return false
	}
	return m.Expires.IsZero() || m.Expires.After(now.Add(14*24*time.Hour))
}

// For lists the catalogue models a provider serves, best score first.
// available is what the provider's own model list returned; nil when it
// could not be read, in which case the names are derived from the catalogue.
func (s *Store) For(provider string, available []string, now time.Time) []Candidate {
	have := map[string]string{}
	for _, id := range available {
		have[key(id)] = id
	}
	native := func(m Model) string {
		k := key(m.ID)
		if id, ok := have[k]; ok {
			return id
		}
		if available != nil {
			// A provider may list only dated snapshots: claude-haiku-4-5-20251001.
			best := ""
			for hk, id := range have {
				if strings.HasPrefix(hk, k+"-20") && id > best {
					best = id
				}
			}
			return best
		}
		switch provider {
		case "anthropic":
			return k
		case "openai":
			return m.ID[strings.Index(m.ID, "/")+1:]
		}
		return ""
	}
	var out []Candidate
	seen := map[string]bool{}
	for _, m := range s.Models {
		c := Candidate{Model: m}
		switch provider {
		case "openrouter":
			c.Native = m.ID
			if available != nil && have[key(m.ID)] == "" {
				c.Native = ""
			}
		case "anthropic", "openai":
			if vendor(m.ID) == provider {
				c.Native = native(m)
			}
		case "opencode":
			// OpenCode serves GPT only through an API omega does not speak.
			if v := vendor(m.ID); v != "openai" && available != nil {
				c.Native = native(m)
			}
		}
		if c.Native == "" || seen[c.Native] || strings.ContainsAny(m.ID, ":~") {
			continue
		}
		seen[c.Native] = true
		c.Usable = usable(m, now)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Native < out[j].Native
	})
	return out
}

// The rule. Both picks maximise score^k per dollar among the models whose
// score is at least a share of the best one on offer: the fast model needs
// 40% of the best score and is chosen mostly on price; the smart model
// must be within a fifth of the best and is chosen mostly on score.
const (
	fastFloor, fastPower   = 0.4, 2.0
	smartFloor, smartPower = 0.8, 3.0
	// keep is how close the previous pick must stay to the new best to be
	// kept, so two models that trade places by a decimal do not alternate.
	keep = 0.9
)

func value(c Candidate, power float64) float64 {
	v := 1.0
	for i := 0; i < int(power); i++ {
		v *= c.Score
	}
	return v / c.Blended()
}

func pick(cs []Candidate, floor, power float64, previous string) string {
	top := 0.0
	for _, c := range cs {
		if c.Usable && c.Score > top {
			top = c.Score
		}
	}
	best, bestV, prevV := "", 0.0, 0.0
	for _, c := range cs {
		if !c.Usable || c.Score < floor*top {
			continue
		}
		v := value(c, power)
		if v > bestV {
			best, bestV = c.Native, v
		}
		if c.Native == previous {
			prevV = v
		}
	}
	if prevV >= keep*bestV && prevV > 0 {
		return previous
	}
	return best
}

// Choose picks the fast and smart models among the candidates. Estimated
// scores are left out when enough models have a measured one.
func Choose(cs []Candidate, previous Picks, now time.Time) Picks {
	var measured []Candidate
	for _, c := range cs {
		if c.Usable && !c.Estimated {
			measured = append(measured, c)
		}
	}
	if len(measured) >= 3 {
		cs = measured
	}
	p := Picks{At: now, Since: previous.Since,
		Fast:  pick(cs, fastFloor, fastPower, previous.Fast),
		Smart: pick(cs, smartFloor, smartPower, previous.Smart)}
	if p.Fast != previous.Fast || p.Smart != previous.Smart || p.Since.IsZero() {
		p.Since = now
	}
	return p
}

// fetch downloads the catalogue; tests replace it.
var fetch = func(ctx context.Context) ([]Model, error) { return Fetch(ctx, Source) }

// Update brings the catalogue and a provider's picks up to date: it fetches
// the catalogue when it is old (or force is set), asks the provider which
// models the key can use, and chooses again. A failed fetch keeps the stored
// catalogue; with none, the picks stay empty and the caller uses its
// built-in defaults.
func (s *Store) Update(ctx context.Context, provider string, list func(context.Context) ([]string, error), force bool, now time.Time) error {
	var fetchErr error
	if force || !s.Fresh(now) {
		ms, err := fetch(ctx)
		if err != nil {
			fetchErr = err
		} else {
			s.Models, s.Fetched = ms, now
		}
	}
	if len(s.Models) == 0 {
		return fetchErr
	}
	prev := s.Picks[provider]
	if !force && fetchErr == nil && prev.Fast != "" && !prev.At.Before(s.Fetched) {
		return nil
	}
	var available []string
	if list != nil {
		if ids, err := list(ctx); err == nil && len(ids) > 0 {
			available = ids
		}
	}
	if p := Choose(s.For(provider, available, now), prev, now); p.Fast != "" {
		s.Picks[provider] = p
	}
	return fetchErr
}

// Price is the list price of a model as a provider names it.
func (s *Store) Price(provider, model string) (in, out float64, ok bool) {
	k := key(model)
	for _, m := range s.Models {
		if strings.ContainsAny(m.ID, ":~") && m.ID != model {
			continue
		}
		switch provider {
		case "openrouter":
			if m.ID != model {
				continue
			}
		case "anthropic", "openai":
			if vendor(m.ID) != provider || key(m.ID) != k {
				continue
			}
		default:
			continue
		}
		return m.In, m.Out, m.In > 0 || m.Out > 0
	}
	return 0, 0, false
}
