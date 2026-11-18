// Package i18n holds the user-facing strings of todo (English and French).
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Supported languages; English is the default.
const (
	EN = "en"
	FR = "fr"
)

var cur = EN

// Languages lists the supported language codes.
func Languages() []string { return []string{EN, FR} }

// Normalize maps a locale ("fr_FR.UTF-8", "FR", "en-US"…) to a supported code.
func Normalize(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "_-.@:"); i >= 0 {
		s = s[:i]
	}
	for _, l := range Languages() {
		if s == l {
			return l, true
		}
	}
	return "", false
}

// Detect reads the system language (LC_ALL, LC_MESSAGES, LANGUAGE, LANG); English when unknown.
func Detect() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		for _, part := range strings.Split(os.Getenv(v), ":") {
			if part == "" || part == "C" || part == "POSIX" {
				continue
			}
			if l, ok := Normalize(part); ok {
				return l
			}
			// a language we do not translate: stop looking at lower-priority variables
			if v == "LC_ALL" || v == "LC_MESSAGES" {
				return EN
			}
		}
	}
	return EN
}

// Set selects the language; it returns false (and changes nothing) for an unsupported one.
func Set(lang string) bool {
	l, ok := Normalize(lang)
	if ok {
		cur = l
	}
	return ok
}

// Lang returns the current language code.
func Lang() string { return cur }

// T returns the translation of key, formatted with args when given.
func T(key string, args ...any) string {
	s, ok := catalogs[cur][key]
	if !ok {
		if s, ok = catalogs[EN][key]; !ok {
			return key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// N picks key+".one" or key+".other" according to n and formats it with n.
func N(key string, n int) string {
	one := n == 1
	if cur == FR {
		one = n >= 0 && n < 2
	}
	if one {
		return T(key+".one", n)
	}
	return T(key+".other", n)
}
