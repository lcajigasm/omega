package models

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

// catalogue is a cut of OpenRouter's list in its own format.
const catalogue = `{"data":[
 {"id":"acme/big-2","name":"Big 2","created":1790000000,"context_length":1000000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.000008","completion":"0.00004"},"supported_parameters":["tools"],
  "benchmarks":{"design_arena":[],"artificial_analysis":{"intelligence_index":64,"coding_index":null,"agentic_index":null}}},
 {"id":"acme/mid-2.5","context_length":1000000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.000002","completion":"0.00001"},"supported_parameters":["tools"],
  "benchmarks":{"artificial_analysis":{"intelligence_index":54}}},
 {"id":"acme/mid-2.5:batch","context_length":1000000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.000001","completion":"0.000005"},"supported_parameters":["tools"],
  "benchmarks":{"artificial_analysis":{"intelligence_index":54}}},
 {"id":"acme/small-2.5","context_length":200000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.0000001","completion":"0.0000005"},"supported_parameters":["tools"],
  "benchmarks":{"artificial_analysis":{"intelligence_index":36}}},
 {"id":"acme/tiny","context_length":200000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.00000001","completion":"0.00000002"},"supported_parameters":["tools"],
  "benchmarks":{"artificial_analysis":{"intelligence_index":9}}},
 {"id":"acme/no-tools","context_length":200000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.00000001","completion":"0.00000002"},"supported_parameters":[],
  "benchmarks":{"artificial_analysis":{"intelligence_index":60}}},
 {"id":"acme/painter","context_length":200000,"architecture":{"output_modalities":["text","image"]},
  "pricing":{"prompt":"0.00000001","completion":"0.00000002"},"supported_parameters":["tools"],
  "benchmarks":{"artificial_analysis":{"intelligence_index":60}}},
 {"id":"acme/leaving","context_length":200000,"expiration_date":"2026-11-15","architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.00000001","completion":"0.00000002"},"supported_parameters":["tools"],
  "benchmarks":{"artificial_analysis":{"intelligence_index":60}}},
 {"id":"acme/gift:free","context_length":200000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0","completion":"0"},"supported_parameters":["tools"],
  "benchmarks":{"artificial_analysis":{"intelligence_index":60}}},
 {"id":"acme/unscored","context_length":200000,"architecture":{"output_modalities":["text"]},
  "pricing":{"prompt":"0.000001","completion":"0.000002"},"supported_parameters":["tools"]}
]}`

func store(t *testing.T) *Store {
	t.Helper()
	ms, err := Parse([]byte(catalogue))
	if err != nil {
		t.Fatal(err)
	}
	return &Store{Models: ms, Fetched: now, Picks: map[string]Picks{}}
}

func TestParseReadsPricesPerMillionAndScores(t *testing.T) {
	s := store(t)
	m := s.Models[0]
	if m.ID != "acme/big-2" || m.In != 8 || m.Out != 40 || m.Score != 64 || !m.Tools || !m.Text || m.Created.IsZero() {
		t.Fatalf("%+v", m)
	}
	if _, err := Parse([]byte(`{"data":[]}`)); err == nil {
		t.Fatal("an empty catalogue is an error, not a reason to forget the stored one")
	}
}

func TestChoosePicksValueForFastAndStrengthForSmart(t *testing.T) {
	s := store(t)
	p := Choose(s.For("openrouter", nil, now), Picks{}, now)
	// tiny is the cheapest by far but too weak; big is the best but costs
	// four times mid for a sixth more score.
	if p.Fast != "acme/small-2.5" || p.Smart != "acme/mid-2.5" {
		t.Fatalf("%+v", p)
	}
	if !p.Since.Equal(now) {
		t.Fatalf("since %v", p.Since)
	}
}

func TestUnusableModelsAreListedButNeverPicked(t *testing.T) {
	s := store(t)
	usable := map[string]bool{}
	for _, c := range s.For("openrouter", nil, now) {
		usable[c.Native] = c.Usable
	}
	for _, id := range []string{"acme/no-tools", "acme/painter", "acme/leaving", "acme/unscored"} {
		if u, listed := usable[id]; !listed || u {
			t.Errorf("%s: listed %v usable %v", id, listed, u)
		}
	}
	for _, id := range []string{"acme/gift:free", "acme/mid-2.5:batch"} {
		if _, listed := usable[id]; listed {
			t.Errorf("%s is a route, not a model to offer", id)
		}
	}
}

func TestPreviousPickIsKeptWhenTheNewBestIsBarelyBetter(t *testing.T) {
	a := Candidate{Model: Model{ID: "v/a", Score: 50, In: 2, Out: 10, Tools: true, Text: true}, Native: "a", Usable: true}
	b := a
	b.Native, b.Score = "b", 50.5
	c := a
	c.Native, c.Score = "c", 50.2
	cs := []Candidate{a, b, c}
	if p := Choose(cs, Picks{}, now); p.Smart != "b" {
		t.Fatalf("fresh choice %+v", p)
	}
	since := now.Add(-48 * time.Hour)
	p := Choose(cs, Picks{Fast: "a", Smart: "a", Since: since}, now)
	if p.Smart != "a" || !p.Since.Equal(since) {
		t.Fatalf("a decimal should not change the model: %+v", p)
	}
	b.Score = 60
	if p := Choose([]Candidate{a, b, c}, Picks{Fast: "a", Smart: "a", Since: since}, now); p.Smart != "b" || !p.Since.Equal(now) {
		t.Fatalf("a clearly better model should win: %+v", p)
	}
}

func TestProviderNamesAreMatchedToTheCatalogue(t *testing.T) {
	s := &Store{Models: []Model{
		{ID: "anthropic/claude-haiku-5.5", Score: 36, In: 0.1, Out: 0.5, Tools: true, Text: true},
		{ID: "anthropic/claude-old-4.5", Score: 20, In: 1, Out: 5, Tools: true, Text: true},
		{ID: "openai/gpt-6.1-sol", Score: 54, In: 2, Out: 10, Tools: true, Text: true},
		{ID: "google/gemini-4", Score: 50, In: 2, Out: 10, Tools: true, Text: true},
	}}
	names := func(provider string, available []string) []string {
		var out []string
		for _, c := range s.For(provider, available, now) {
			out = append(out, c.Native)
		}
		return out
	}
	eq := func(got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %v want %v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("got %v want %v", got, want)
			}
		}
	}
	// Without the provider's list the names are derived: dots become dashes.
	eq(names("anthropic", nil), "claude-haiku-5-5", "claude-old-4-5")
	eq(names("openai", nil), "gpt-6.1-sol")
	// With it, only what the key can use, under the provider's own name.
	eq(names("anthropic", []string{"claude-haiku-5-5", "claude-old-4-5-20251001"}), "claude-haiku-5-5", "claude-old-4-5-20251001")
	eq(names("anthropic", []string{"claude-haiku-5-5"}), "claude-haiku-5-5")
	// OpenCode resells several vendors, but not GPT through an API omega speaks.
	eq(names("opencode", []string{"gemini-4", "claude-haiku-5-5", "gpt-6.1-sol"}), "gemini-4", "claude-haiku-5-5")
	eq(names("opencode", nil))
	eq(names("ollama", nil))
}

func TestPriceByProviderName(t *testing.T) {
	s := &Store{Models: []Model{
		{ID: "anthropic/claude-haiku-5.5", In: 0.1, Out: 0.5},
		{ID: "anthropic/claude-haiku-5.5:batch", In: 0.05, Out: 0.25},
	}}
	if in, out, ok := s.Price("anthropic", "claude-haiku-5-5"); !ok || in != 0.1 || out != 0.5 {
		t.Fatalf("%v %v %v", in, out, ok)
	}
	if in, _, ok := s.Price("openrouter", "anthropic/claude-haiku-5.5:batch"); !ok || in != 0.05 {
		t.Fatalf("batch route: %v %v", in, ok)
	}
	// OpenCode sets its own prices: the catalogue's would be a guess.
	if _, _, ok := s.Price("opencode", "claude-haiku-5-5"); ok {
		t.Fatal("no price should be reported for a reseller")
	}
}

func TestEstimateFillsMissingScoresFromOtherBenchmarks(t *testing.T) {
	ms := make([]Model, 10)
	other := make([]float64, 10)
	for i := 0; i < 9; i++ {
		ms[i].Score = float64(10 + 5*i)
		other[i] = float64(100 + 10*i) // score = other/2 - 40
	}
	other[9] = 200
	estimate(ms, other)
	if !ms[9].Estimated || ms[9].Score != 60 {
		t.Fatalf("%+v", ms[9])
	}
	// Too few models with both: no estimate.
	ms = make([]Model, 3)
	ms[0].Score, ms[1].Score = 10, 20
	estimate(ms, []float64{1, 2, 3})
	if ms[2].Score != 0 {
		t.Fatalf("%+v", ms[2])
	}
}

func TestUpdateFetchesOnceADayAndSurvivesAFailedFetch(t *testing.T) {
	fetched := 0
	var fail error
	old := fetch
	defer func() { fetch = old }()
	fetch = func(context.Context) ([]Model, error) {
		fetched++
		if fail != nil {
			return nil, fail
		}
		return Parse([]byte(catalogue))
	}
	listed := 0
	list := func(context.Context) ([]string, error) { listed++; return nil, errors.New("no list") }

	path := filepath.Join(t.TempDir(), "state", "models.json")
	s := Load(path)
	if err := s.Update(context.Background(), "openrouter", list, false, now); err != nil {
		t.Fatal(err)
	}
	if fetched != 1 || s.Picks["openrouter"].Fast != "acme/small-2.5" {
		t.Fatalf("fetched %d picks %+v", fetched, s.Picks)
	}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	// The same day: nothing is fetched and the provider is not asked again.
	s = Load(path)
	if err := s.Update(context.Background(), "openrouter", list, false, now.Add(2*time.Hour)); err != nil || fetched != 1 || listed != 1 {
		t.Fatalf("err %v fetched %d listed %d", err, fetched, listed)
	}
	// A provider not seen yet is chosen from the stored catalogue.
	if err := s.Update(context.Background(), "anthropic", nil, false, now.Add(2*time.Hour)); err != nil || fetched != 1 {
		t.Fatalf("err %v fetched %d", err, fetched)
	}

	// The next day the network is down: the error is reported and the
	// stored catalogue and picks stay.
	fail = errors.New("offline")
	if err := s.Update(context.Background(), "openrouter", list, false, now.Add(30*time.Hour)); err == nil {
		t.Fatal("the failed fetch should be reported")
	}
	if fetched != 2 || len(s.Models) == 0 || s.Picks["openrouter"].Fast != "acme/small-2.5" {
		t.Fatalf("fetched %d models %d picks %+v", fetched, len(s.Models), s.Picks)
	}

	// Never fetched and offline: no picks, the caller uses its defaults.
	empty := Load(filepath.Join(t.TempDir(), "none.json"))
	if err := empty.Update(context.Background(), "openrouter", list, false, now); err == nil || empty.Picks["openrouter"].Fast != "" {
		t.Fatalf("err %v picks %+v", err, empty.Picks)
	}
}
