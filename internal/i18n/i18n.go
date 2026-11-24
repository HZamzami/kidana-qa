// Package i18n translates the UI. Arabic is the source language and the
// default; English is looked up in a catalog keyed by the Arabic text
// (gettext style). A string with no entry passes through unchanged, so user
// data (site names, notes) can safely go through T as well.
package i18n

import (
	"fmt"
	"strings"
)

// Lang is a UI language.
type Lang string

const (
	AR Lang = "ar"
	EN Lang = "en"
)

// Parse returns the language for a code, defaulting to Arabic.
func Parse(s string) Lang {
	if strings.EqualFold(strings.TrimSpace(s), string(EN)) {
		return EN
	}
	return AR
}

// Valid reports whether s names a supported language.
func Valid(s string) bool { return s == string(AR) || s == string(EN) }

// Dir is the text direction of the language.
func (l Lang) Dir() string {
	if l == EN {
		return "ltr"
	}
	return "rtl"
}

// Other is the language the toggle switches to.
func (l Lang) Other() Lang {
	if l == EN {
		return AR
	}
	return EN
}

// Name is the language's own name, for the toggle.
func (l Lang) Name() string {
	if l == EN {
		return "English"
	}
	return "عربي"
}

// T translates an Arabic source string.
func T(l Lang, s string) string {
	if l != EN || s == "" {
		return s
	}
	if v, ok := en[s]; ok {
		return v
	}
	return s
}

// Tf translates a format string, then formats it.
func Tf(l Lang, format string, args ...any) string {
	return fmt.Sprintf(T(l, format), args...)
}

// Has reports whether the catalog has an English entry for s (for tests).
func Has(s string) bool {
	_, ok := en[s]
	return ok
}

// Pick returns the English value when the language is English and it is set,
// else the Arabic one. Used for admin-entered names with an optional English
// translation.
func Pick(l Lang, ar, en string) string {
	if l == EN && strings.TrimSpace(en) != "" {
		return en
	}
	return ar
}
