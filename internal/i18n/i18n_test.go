package i18n

import (
	"regexp"
	"testing"
)

var verb = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z]`)

func TestEveryKeyIsTranslated(t *testing.T) {
	for key, row := range messages {
		if len(row) != len(Supported) {
			t.Errorf("%s: %d translations, want %d", key, len(row), len(Supported))
			continue
		}
		want := verb.FindAllString(row[0], -1)
		for i, s := range row {
			if s == "" {
				t.Errorf("%s: empty %s translation", key, Supported[i])
			}
			got := verb.FindAllString(s, -1)
			if len(got) != len(want) {
				t.Errorf("%s/%s: format verbs %v, want %v", key, Supported[i], got, want)
				continue
			}
			for j := range got {
				if got[j] != want[j] {
					t.Errorf("%s/%s: format verbs %v, want %v", key, Supported[i], got, want)
					break
				}
			}
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"es_ES.UTF-8": "es", "pt-BR": "pt", "de": "de", "FR_fr": "fr", "en_US": "en",
		"it_IT": "", "C": "", "POSIX": "", "": "", "auto": "",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInitPrecedence(t *testing.T) {
	t.Setenv("OMEGA_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "de_DE.UTF-8")
	Init("auto")
	if Lang() != "de" {
		t.Fatalf("locale fallback: got %s", Lang())
	}
	Init("fr")
	if Lang() != "fr" {
		t.Fatalf("config: got %s", Lang())
	}
	t.Setenv("OMEGA_LANG", "pt")
	Init("fr")
	if Lang() != "pt" {
		t.Fatalf("OMEGA_LANG: got %s", Lang())
	}
	if T("ui.denied") != "recusado" {
		t.Fatalf("T in pt: %s", T("ui.denied"))
	}
	Init("en")
}
