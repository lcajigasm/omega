package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/config"
	"github.com/lcajigasm/omega/internal/i18n"
	"github.com/lcajigasm/omega/internal/secrets"
	"github.com/lcajigasm/omega/internal/steps"
)

func writeDoctorConfig(t *testing.T, body string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	if err := os.MkdirAll(filepath.Join(root, "omega"), 0o755); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(root, "omega", "config.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stubKey(t *testing.T, key string, err error) {
	t.Helper()
	prev := getKey
	t.Cleanup(func() { getKey = prev })
	getKey = func(string) (string, secrets.Source, error) {
		if err != nil {
			return "", "", err
		}
		return key, secrets.FromEnv, nil
	}
}

func runDoctorOut(t *testing.T) (string, error) {
	t.Helper()
	i18n.Init("en")
	var buf bytes.Buffer
	err := runDoctor(&buf)
	return buf.String(), err
}

func TestDoctorReportsConfigKeyAndSteps(t *testing.T) {
	writeDoctorConfig(t, "provider = \"anthropic\"\napproval = \"sometimes\"\n")
	stubKey(t, "", secrets.ErrNotFound)
	out, err := runDoctorOut(t)
	if err == nil {
		t.Fatal("invalid config and a missing key succeeded")
	}
	if !strings.Contains(out, "config:") || !strings.Contains(out, "key: missing") {
		t.Fatalf("output:\n%s", out)
	}
	if !strings.Contains(out, "✓ disk") || !strings.Contains(out, "steps:") {
		t.Fatalf("steps missing:\n%s", out)
	}
	installed := 0
	for _, s := range steps.All {
		if s.Available() {
			installed++
			if !strings.Contains(out, "✓ "+s.ID) {
				t.Fatalf("installed step %s not listed:\n%s", s.ID, out)
			}
		}
	}
	if installed == 0 {
		t.Fatal("no step is available; the doctor listing cannot be checked")
	}
}

func TestDoctorRejectsAMalformedKeyWithoutCallingTheProvider(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		t.Errorf("provider was called for a malformed key: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	writeDoctorConfig(t, "provider = \"anthropic\"\n\n[providers.anthropic]\nbase_url = \""+srv.URL+"\"\n")
	stubKey(t, "https://console.anthropic.com/settings/keys", nil)
	out, err := runDoctorOut(t)
	if err == nil || hits != 0 {
		t.Fatalf("err %v hits %d\n%s", err, hits, out)
	}
	if !strings.Contains(out, "not shaped like an API key") {
		t.Fatalf("output:\n%s", out)
	}
	if strings.Contains(out, "POST") || strings.Contains(strings.ToLower(out), "token") {
		t.Fatalf("output mentions a chat:\n%s", out)
	}
}

func TestDoctorListsModelsAndDoesNotSendAMessage(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-haiku","type":"model"}],"has_more":false,"first_id":"claude-haiku","last_id":"claude-haiku"}`))
	}))
	defer srv.Close()
	writeDoctorConfig(t, "provider = \"anthropic\"\nskip_steps = [\"disk\"]\n\n[providers.anthropic]\nbase_url = \""+srv.URL+"\"\n")
	stubKey(t, "sk-ant-"+strings.Repeat("a", 20), nil)
	out, err := runDoctorOut(t)
	if err != nil {
		t.Fatalf("err %v\n%s", err, out)
	}
	if strings.Join(calls, ",") != "GET /v1/models" {
		t.Fatalf("calls %v", calls)
	}
	if !strings.Contains(out, "config: ok") || !strings.Contains(out, "key: ok") || !strings.Contains(out, "provider: reachable") {
		t.Fatalf("output:\n%s", out)
	}
	if !strings.Contains(out, "✓ disk (skipped)") {
		t.Fatalf("skipped step:\n%s", out)
	}
}

func TestDoctorWarnsOnKeyPrefixAndRejectsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"bad"}}`))
	}))
	defer srv.Close()
	writeDoctorConfig(t, "provider = \"anthropic\"\n\n[providers.anthropic]\nbase_url = \""+srv.URL+"\"\n")
	stubKey(t, strings.Repeat("x", 24), nil)
	out, err := runDoctorOut(t)
	if err == nil {
		t.Fatal("rejected key succeeded")
	}
	if !strings.Contains(out, "usually start with sk-ant-") || !strings.Contains(out, "rejected") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestDoctorLocalProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Method != http.MethodGet {
			t.Errorf("%s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	writeDoctorConfig(t, "provider = \"ollama\"\n\n[providers.ollama]\nbase_url = \""+srv.URL+"/v1\"\n")
	out, err := runDoctorOut(t)
	if err != nil {
		t.Fatalf("err %v\n%s", err, out)
	}
	if !strings.Contains(out, "not required") || !strings.Contains(out, "provider: reachable") {
		t.Fatalf("output:\n%s", out)
	}

	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	down.Close()
	writeDoctorConfig(t, "provider = \"ollama\"\n\n[providers.ollama]\nbase_url = \""+down.URL+"/v1\"\n")
	out, err = runDoctorOut(t)
	if err == nil || !strings.Contains(out, "not reachable") {
		t.Fatalf("err %v\n%s", err, out)
	}

	writeDoctorConfig(t, "provider = \"custom\"\n")
	out, err = runDoctorOut(t)
	if err == nil || !strings.Contains(out, "base_url") {
		t.Fatalf("err %v\n%s", err, out)
	}
}

func TestDoctorUnknownProvider(t *testing.T) {
	writeDoctorConfig(t, "provider = \"nope\"\n")
	out, err := runDoctorOut(t)
	if err == nil || !strings.Contains(out, "unknown provider") {
		t.Fatalf("err %v\n%s", err, out)
	}
	if _, err := config.Load(); err != nil {
		t.Fatal(err)
	}
}
