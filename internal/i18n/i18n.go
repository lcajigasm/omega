// Package i18n translates omega's user-facing text. Supported languages are
// English, Spanish, French, German and Portuguese; English is the fallback.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Supported lists the language codes omega ships translations for, in the
// column order used by the messages table.
var Supported = []string{"en", "es", "fr", "de", "pt"}

// Names maps each supported code to the language's English name, which is
// what the agent is told to reply in.
var Names = map[string]string{
	"en": "English",
	"es": "Spanish",
	"fr": "French",
	"de": "German",
	"pt": "Portuguese",
}

var current = "en"

// Init selects the language: OMEGA_LANG, then the configured language, then
// the system locale (LC_ALL, LC_MESSAGES, LANG), then English.
func Init(configured string) {
	for _, cand := range []string{os.Getenv("OMEGA_LANG"), configured, os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
		if code := Normalize(cand); code != "" {
			current = code
			return
		}
	}
	current = "en"
}

// Normalize turns values such as "es_ES.UTF-8" or "pt-BR" into a supported
// code, or "" when the language is not supported.
func Normalize(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if len(v) < 2 || v == "c" || v == "posix" {
		return ""
	}
	code := v[:2]
	if _, ok := Names[code]; ok {
		return code
	}
	return ""
}

// Lang returns the active language code.
func Lang() string { return current }

// T returns the translation of key in the active language, formatted with args.
func T(key string, args ...any) string {
	row, ok := messages[key]
	if !ok {
		return key
	}
	s := ""
	for i, code := range Supported {
		if code == current && i < len(row) {
			s = row[i]
		}
	}
	if s == "" {
		s = row[0]
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// TOr translates key, or returns fallback when no translation exists.
func TOr(key, fallback string) string {
	if _, ok := messages[key]; ok {
		return T(key)
	}
	return fallback
}

// IsYes reports whether an answer means "yes" in any supported language.
func IsYes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes", "s", "si", "sí", "sim", "o", "oui", "j", "ja":
		return true
	}
	return false
}
