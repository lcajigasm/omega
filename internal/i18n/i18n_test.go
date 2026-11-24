package i18n

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
	"testing"
)

var verb = regexp.MustCompile(`%(%|(\[\d+\])?[-+# 0]*(\d+|\*)?(\.\d*)?(\[\d+\])?[a-zA-Z])`)

// verbs reads the format verbs of a text as the argument each one takes:
// "%[2]s and %d" is {2: "s", 3: "d"}. Explicit indexes let a translation
// put the arguments in another order, so the order of the verbs is not
// compared, only which argument gets which verb.
func verbs(s string) map[int]string {
	out := map[int]string{}
	next := 1
	for _, v := range verb.FindAllString(s, -1) {
		if v == "%%" {
			continue
		}
		if m := regexp.MustCompile(`\[(\d+)\]`).FindStringSubmatch(v); m != nil {
			fmt.Sscan(m[1], &next)
		}
		out[next] += v[len(v)-1:]
		next++
	}
	return out
}

// Every key has the five translations, and every translation takes the
// same arguments with the same verbs: one with a %d too few prints
// %!d(MISSING) to the user.
func TestEveryKeyIsTranslated(t *testing.T) {
	for key, row := range messages {
		if len(row) != len(Supported) {
			t.Errorf("%s: %d translations, want %d", key, len(row), len(Supported))
			continue
		}
		want := verbs(row[0])
		for i, s := range row {
			if s == "" {
				t.Errorf("%s: empty %s translation", key, Supported[i])
			}
			if got := verbs(s); !maps.Equal(got, want) {
				t.Errorf("%s/%s: format verbs %v, want %v (as in English)", key, Supported[i], got, want)
			}
		}
		// The singular of a counted text stands in for the base key: it
		// takes the same arguments.
		if base, ok := strings.CutSuffix(key, ".one"); ok {
			if b, ok := messages[base]; !ok {
				t.Errorf("%s: no plural %s beside it", key, base)
			} else if !maps.Equal(verbs(b[0]), want) {
				t.Errorf("%s: verbs %v, but %s has %v", key, want, base, verbs(b[0]))
			}
		}
	}
}

func TestVerbs(t *testing.T) {
	for in, want := range map[string]map[int]string{
		"%d of %s, 100%%":   {1: "d", 2: "s"},
		"%[2]s then %[1]q":  {1: "q", 2: "s"},
		"%-12s %5.1f %v":    {1: "s", 2: "f", 3: "v"},
		"nothing to format": {},
	} {
		if got := verbs(in); !maps.Equal(got, want) {
			t.Errorf("verbs(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	messages["test.n"] = []string{"%d steps", "%d pasos", "%d étapes", "%d Schritte", "%d passos"}
	messages["test.n.one"] = []string{"%d step", "%d paso", "%d étape", "%d Schritt", "%d passo"}
	messages["test.plain"] = []string{"%d items in %s", "", "", "", ""}
	t.Cleanup(func() {
		delete(messages, "test.n")
		delete(messages, "test.n.one")
		delete(messages, "test.plain")
		Init("en")
	})
	Init("es")
	if got := N("test.n", 1); got != "1 paso" {
		t.Errorf("one: %q", got)
	}
	if got := N("test.n", 0); got != "0 pasos" {
		t.Errorf("zero in Spanish: %q", got)
	}
	Init("fr")
	if got := N("test.n", 0); got != "0 étape" {
		t.Errorf("zero in French: %q", got)
	}
	Init("en")
	if got := N("test.n", 3); got != "3 steps" {
		t.Errorf("many: %q", got)
	}
	if got := N("test.plain", 1, 1, "x"); got != "1 items in x" {
		t.Errorf("a key without a singular: %q", got)
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

// Off macOS the texts that name the Mac have another wording, in every
// language, and it must not name the Mac.
func TestOtherSystemsWording(t *testing.T) {
	old := OS
	t.Cleanup(func() { OS = old; Init("en") })
	for key := range messages {
		base, isOther := strings.CutSuffix(key, "@other")
		if !isOther {
			continue
		}
		if _, ok := messages[base]; !ok {
			t.Errorf("%s replaces a key that does not exist", key)
		}
		for i, code := range Supported {
			Init(code)
			OS = "darwin"
			mac := T(base)
			OS = "linux"
			other := T(base)
			if mac != messages[base][i] || other != messages[key][i] {
				t.Errorf("%s/%s: the wrong wording was picked", base, code)
			}
			if strings.Contains(other, "Mac") {
				t.Errorf("%s/%s still names the Mac: %s", key, code, other)
			}
		}
	}
	for i, code := range Supported {
		if messages["cli.long@other"][i] == messages["cli.long"][i] {
			t.Errorf("cli.long/%s: the Mac phrase was not found", code)
		}
	}
}
