// SPDX-License-Identifier: Apache-2.0

package model

import "sync"

// DefaultTextLanguage is the language a Texts$Text is built under when nothing
// has told the process otherwise. It is the pre-#970 behaviour, so an unset
// process behaves exactly as mxcli always did.
const DefaultTextLanguage = "en_US"

var (
	textLangMu sync.RWMutex
	textLang   = DefaultTextLanguage
)

// AuthoringLanguage is the language code a writer stores a bare string under
// when it has to build a Texts$Text and the caller handed it no language.
//
// Why this is process-level rather than a parameter. Mendix has no
// language-neutral text: every Texts$Translation carries a LanguageCode, so the
// leaf that turns a Go string into a Texts$Text must name one. Those leaves sit
// at the bottom of the widget/navigation serializers, and threading a language
// down to them would change the signature of **183 functions** across five
// packages (113 in sdk/mpr alone) — nearly every widget writer in the codebase,
// to carry a value that is constant for the entire run. Measured, not estimated;
// see the #970 investigation.
//
// The language genuinely is ambient for a write: it is a property of the project
// being written, mxcli writes one project per process, and it never changes
// mid-write. That is the same shape as MXCLI_ENGINE and MXCLI_ALWAYS_WRITE, which
// this codebase already reads from the process.
//
// The contract: SetAuthoringLanguage is called once, when a project's settings
// are first resolved, BEFORE any write. Reads are cheap and safe from any
// goroutine (page widget extraction runs in parallel). An empty or unset value
// means DefaultTextLanguage, so nothing that forgets to set it regresses.
//
// It also names the translation ALTER PAGE edits in an existing Texts$Text
// (pagemutator.setTextsTextTranslation): that entry is updated or added, and
// the other languages' translations are kept — the language DESCRIBE shows is
// the one a SET writes.
func AuthoringLanguage() string {
	textLangMu.RLock()
	defer textLangMu.RUnlock()
	return textLang
}

// SetAuthoringLanguage sets the language returned by AuthoringLanguage. An empty
// code resets to DefaultTextLanguage rather than storing an empty LanguageCode,
// which no Mendix document has and Studio Pro would not resolve.
func SetAuthoringLanguage(code string) {
	textLangMu.Lock()
	defer textLangMu.Unlock()
	if code == "" {
		code = DefaultTextLanguage
	}
	textLang = code
}

// TranslationEntry is one Texts$Translation as stored: its language and its
// text. A Texts$Text is read as a slice of them, in document order.
type TranslationEntry struct {
	LanguageCode string
	Text         string
}

// FallbackTextLanguage is the language a text is read in when it has no
// translation in the preferred one.
const FallbackTextLanguage = "en_US"

// SelectTranslation is the one rule for reading a translatable text: the one
// DESCRIBE prints for a page's or widget's text (issue #702), a navigation or
// menu item's caption, and the caption an ALTER NAVIGATION / ALTER MENU
// address is compared with — so an address copied from describe output is the
// string the resolver matches, by construction.
//
// It returns the preferred language's translation, else en_US's, else the
// first non-empty translation in document order.
//
// A preferred-language translation that is present but EMPTY is returned as
// the empty string. DESCRIBE output is re-executed, and whatever it prints is
// written back into the preferred language — so falling back to another
// language here turned a caption stored as en_US "" + nl_NL "Knop" into en_US
// "Knop" (ako/mxcli#705). The fallbacks are for a text with no entry in that
// language. When a language occurs twice, its first entry counts.
func SelectTranslation(entries []TranslationEntry, preferredLang string) string {
	byLang := make(map[string]string)
	firstNonEmpty := ""
	for _, e := range entries {
		if e.LanguageCode != "" {
			if _, exists := byLang[e.LanguageCode]; !exists {
				byLang[e.LanguageCode] = e.Text
			}
		}
		if firstNonEmpty == "" && e.Text != "" {
			firstNonEmpty = e.Text
		}
	}
	if v, ok := byLang[preferredLang]; ok && preferredLang != "" {
		return v
	}
	if byLang[FallbackTextLanguage] != "" {
		return byLang[FallbackTextLanguage]
	}
	return firstNonEmpty
}
