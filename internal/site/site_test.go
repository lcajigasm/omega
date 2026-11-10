package site

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/lcajigasm/omega/internal/i18n"
)

const src = "../../site"

// The website speaks the languages the app speaks: one page per language in
// i18n.Supported, each with every string. A language added to the app fails
// here until the site has it too.
func TestSiteHasEveryAppLanguage(t *testing.T) {
	files, err := Build(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range i18n.Supported {
		for _, page := range pages {
			path := folder(lang) + page
			text, ok := files[path]
			if !ok {
				t.Errorf("no %s", path)
				continue
			}
			if !strings.Contains(text, `<html lang="`+lang+`"`) {
				t.Errorf("%s does not declare lang=%q", path, lang)
			}
			// Every page links every language, itself included.
			for _, other := range i18n.Supported {
				if !strings.Contains(text, `hreflang="`+other+`"`) {
					t.Errorf("%s has no link to %q", path, other)
				}
			}
		}
		if names[lang] == "" {
			t.Errorf("the language menu has no name for %q", lang)
		}
	}
}

var (
	literal = regexp.MustCompile(`(?s)<(?:code|kbd)>(.*?)</(?:code|kbd)>`)
	markup  = regexp.MustCompile(`<[^>]+>`)
)

// A translation changes the words and nothing else: commands, keys and the
// markup around them stay as the English page has them. A translated command
// is one the reader copies and omega does not know.
func TestTranslationsKeepCommandsAndMarkup(t *testing.T) {
	tables, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	en := tables[Default]
	sorted := func(list []string) []string {
		out := append([]string(nil), list...)
		sort.Strings(out)
		return out
	}
	for _, lang := range i18n.Supported {
		if lang == Default {
			continue
		}
		same := 0
		for key, want := range en {
			got := tables[lang][key]
			if got == want {
				same++
			}
			if a, b := sorted(literal.FindAllString(want, -1)), sorted(literal.FindAllString(got, -1)); !reflect.DeepEqual(a, b) {
				t.Errorf("%s %s: commands and keys differ from English\n en: %q\n %s: %q", lang, key, a, lang, b)
			}
			if a, b := sorted(markup.FindAllString(want, -1)), sorted(markup.FindAllString(got, -1)); !reflect.DeepEqual(a, b) {
				t.Errorf("%s %s: markup differs from English\n en: %q\n %s: %q", lang, key, a, lang, b)
			}
			// A shell snippet: only the comments are words.
			if strings.Contains(want, "\n") && strings.Contains(want, "# ") {
				wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
				if len(wl) != len(gl) {
					t.Errorf("%s %s: %d lines, English has %d", lang, key, len(gl), len(wl))
					continue
				}
				for i := range wl {
					w, _, _ := strings.Cut(wl[i], "#")
					g, _, _ := strings.Cut(gl[i], "#")
					if w != g {
						t.Errorf("%s %s line %d: the command changed: %q, English %q", lang, key, i+1, g, w)
					}
				}
			}
		}
		// Names and commands are the same in every language; prose is not.
		if same > len(en)/4 {
			t.Errorf("%s: %d of %d strings are still English", lang, same, len(en))
		}
	}
}

// docs/ is what GitHub Pages serves, so it has to be what site/ builds.
func TestDocsAreBuiltFromSite(t *testing.T) {
	files, err := Build(src)
	if err != nil {
		t.Fatal(err)
	}
	clean := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	for path, want := range files {
		got, err := os.ReadFile(filepath.Join("../../docs", filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("docs/%s is missing: run `make site`", path)
			continue
		}
		if clean(string(got)) != clean(want) {
			t.Errorf("docs/%s is not what site/ builds: edit site/ and run `make site`", path)
		}
	}
}
