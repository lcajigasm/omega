// Package site builds the website under docs/ from the templates and the
// string tables under site/.
//
// There is one page per language the app speaks (i18n.Supported): English at
// the root, the others in a folder named after their code. The pages are
// plain files, so GitHub Pages serves them as they are and every language
// has an address of its own.
package site

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/lcajigasm/omega/internal/i18n"
)

// Base is where GitHub Pages publishes docs/.
const Base = "https://lcajigasm.github.io/omega/"

// Default is the language served at the root.
const Default = "en"

// Each language under its own name, for the language menu.
var names = map[string]string{
	"en": "English",
	"es": "Español",
	"fr": "Français",
	"de": "Deutsch",
	"pt": "Português",
}

// The pages every language has. 404.html exists once: GitHub Pages serves
// that single file for any missing address.
var pages = []string{"index.html", "privacy.html"}

// Lang is one entry of the language menu, seen from the page being built.
type Lang struct {
	Code, Name string
	Href       string // relative, from the page being built
	URL        string // absolute, for hreflang
	Current    bool
}

// QA is one question of the FAQ.
type QA struct{ Q, A string }

// Load reads the string table of every language the app supports. A language
// the app has and the site lacks is an error, and so is a table whose keys
// differ from the English one.
func Load(src string) (map[string]map[string]string, error) {
	tables := map[string]map[string]string{}
	for _, lang := range i18n.Supported {
		path := filepath.Join(src, "i18n", lang+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("the app speaks %q and the website has no %s: %w", lang, path, err)
		}
		table := map[string]string{}
		if err := json.Unmarshal(data, &table); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		tables[lang] = table
	}
	en := tables[Default]
	for _, lang := range i18n.Supported {
		var problems []string
		for key := range en {
			if strings.TrimSpace(tables[lang][key]) == "" {
				problems = append(problems, "missing "+key)
			}
		}
		for key := range tables[lang] {
			if _, ok := en[key]; !ok {
				problems = append(problems, "unknown "+key)
			}
		}
		if len(problems) > 0 {
			sort.Strings(problems)
			return nil, fmt.Errorf("site/i18n/%s.json: %s", lang, strings.Join(problems, ", "))
		}
	}
	return tables, nil
}

// Build renders every page of the site. The keys of the result are paths
// under docs/, with forward slashes.
func Build(src string) (map[string]string, error) {
	tables, err := Load(src)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, lang := range i18n.Supported {
		for _, page := range pages {
			text, err := render(src, page, lang, tables)
			if err != nil {
				return nil, err
			}
			out[folder(lang)+page] = text
		}
	}
	text, err := render(src, "404.html", Default, tables)
	if err != nil {
		return nil, err
	}
	out["404.html"] = text
	return out, nil
}

// folder is where a language lives under docs/: the root for English.
func folder(lang string) string {
	if lang == Default {
		return ""
	}
	return lang + "/"
}

// address is a page as a link shows it: the home page is its folder.
func address(page string) string {
	if page == "index.html" {
		return ""
	}
	return page
}

var tag = regexp.MustCompile(`<[^>]*>`)

// plain is a string without its markup, for attributes and structured data.
func plain(s string) string {
	return html.UnescapeString(tag.ReplaceAllString(s, ""))
}

func render(src, page, lang string, tables map[string]map[string]string) (string, error) {
	table := tables[lang]
	root := ""
	if lang != Default {
		root = "../"
	}
	lookup := func(key string) (string, error) {
		s, ok := table[key]
		if !ok {
			return "", fmt.Errorf("%s: no string %q", page, key)
		}
		return s, nil
	}
	langs := func() []Lang {
		var list []Lang
		for _, code := range i18n.Supported {
			name := names[code]
			if name == "" {
				name = code
			}
			href := root + folder(code) + address(page)
			if href == "" {
				href = "./"
			}
			list = append(list, Lang{
				Code: code, Name: name, Href: href,
				URL:     Base + folder(code) + address(page),
				Current: code == lang,
			})
		}
		return list
	}
	faq := func() []QA {
		var list []QA
		for i := 1; ; i++ {
			q, ok := table[fmt.Sprintf("faq.q%d", i)]
			if !ok {
				return list
			}
			list = append(list, QA{q, table[fmt.Sprintf("faq.a%d", i)]})
		}
	}
	funcs := template.FuncMap{
		"t": lookup,
		// attr is a string inside an attribute: no markup, quotes escaped.
		"attr": func(key string) (string, error) {
			s, err := lookup(key)
			return html.EscapeString(plain(s)), err
		},
		// js is a string as a JSON value, for a script or structured data.
		"js": func(s string) (string, error) {
			data, err := json.Marshal(plain(s))
			return string(data), err
		},
		"langs": langs,
		"faq":   faq,
		// faqld is the FAQ as schema.org data, from the questions the page shows.
		"faqld": func() (string, error) {
			type answer struct {
				Type string `json:"@type"`
				Text string `json:"text"`
			}
			type question struct {
				Type   string `json:"@type"`
				Name   string `json:"name"`
				Answer answer `json:"acceptedAnswer"`
			}
			doc := struct {
				Context string     `json:"@context"`
				Type    string     `json:"@type"`
				Lang    string     `json:"inLanguage"`
				Main    []question `json:"mainEntity"`
			}{Context: "https://schema.org", Type: "FAQPage", Lang: lang}
			for _, qa := range faq() {
				doc.Main = append(doc.Main, question{"Question", plain(qa.Q), answer{"Answer", plain(qa.A)}})
			}
			data, err := json.MarshalIndent(doc, "  ", "  ")
			return "  " + string(data), err
		},
		// others lists the languages that are not at the root, as JSON.
		"others": func() (string, error) {
			var codes []string
			for _, code := range i18n.Supported {
				if code != Default {
					codes = append(codes, code)
				}
			}
			data, err := json.Marshal(codes)
			return string(data), err
		},
		// all gives the named strings in every other language, as JSON, for
		// the one page that picks its language in the browser.
		"all": func(keys ...string) (string, error) {
			set := map[string]map[string]string{}
			for _, code := range i18n.Supported {
				if code == Default {
					continue
				}
				set[code] = map[string]string{}
				for _, key := range keys {
					s, ok := tables[code][key]
					if !ok {
						return "", fmt.Errorf("%s: no string %q in %s", page, key, code)
					}
					set[code][key] = s
				}
			}
			data, err := json.Marshal(set)
			return string(data), err
		},
	}
	tmpl, err := template.New(page).Funcs(funcs).ParseFiles(filepath.Join(src, page))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	data := struct {
		Lang, Root, URL, DefaultURL string
		Default                     bool
	}{
		Lang: lang, Root: root,
		URL:        Base + folder(lang) + address(page),
		DefaultURL: Base + address(page),
		Default:    lang == Default,
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}
