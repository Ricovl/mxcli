// SPDX-License-Identifier: Apache-2.0

// Package langver is the MDL language version: the `mdl <n>;` header a script
// may start with, and the gate every change of meaning goes through.
//
// ADR-0011 decision 2 is the contract this package implements:
//
//   - A script with no header is mdl 0, the alpha meaning. Nobody writes
//     `mdl 0;`; it is only ever implicit.
//   - A change of meaning, or a new rejection, applies only under the version
//     that introduces it. Under an older version the old meaning is kept and
//     the construct warns, so a committed script never changes behaviour just
//     because a newer mxcli runs it.
//   - A version is a contract once it is frozen (Frozen below). describe and
//     fmt write the frozen version's header; mdl 1 was frozen at beta, so a
//     later change of meaning needs mdl 2.
//
// The language version is independent of the Mendix target version (ADR-0011
// decision 3); nothing here reads or writes a Mendix version.
package langver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version is an MDL language version, the number in `mdl <n>;`.
type Version int

const (
	// V0 is the alpha language: every script written before the header
	// existed, and every script that still omits it.
	V0 Version = 0
	// V1 is the beta language (ADR-0010's canonical syntax and the §5 changes
	// of meaning in PROPOSAL_mdl_beta_syntax_freeze.md).
	V1 Version = 1
)

// Latest is the newest version this mxcli understands. A header naming a
// higher one is refused: running a script under rules it was not written for
// is exactly the silent change of meaning the header exists to prevent.
const Latest = V1

// Frozen is the newest FROZEN version: the language describe writes by
// default, and the one whose header (HeaderLine) heads its output. mdl 1 was
// frozen at beta (ako/mxcli#714), so it is a contract: a later change of
// meaning needs a new version, which this package must then learn about by
// raising Latest. A version above Frozen would be unfinished, and nothing
// may emit its header until it is frozen too.
const Frozen = V1

// Default is the version of a script that has no header. Headerless SCRIPTS
// keep the alpha meaning forever (ADR-0011); only the interactive surfaces
// (the REPL, `-c` one-liners) start in the frozen version, see Interactive.
const Default = V0

// Interactive is the version a REPL session or a `-c` one-liner starts in
// (freeze decision 6, PROPOSAL_mdl_beta_syntax_freeze.md §7): the frozen
// language, switched with `--mdl 0` or, in the REPL, an `mdl 0;` statement.
// Nothing typed interactively is committed, so there is no older script whose
// meaning the newer default could change.
const Interactive = Frozen

// Known reports whether this mxcli can run a script written in v.
func (v Version) Known() bool { return v >= V0 && v <= Latest }

// String renders the header spelling of v without the terminator: "mdl 1".
func (v Version) String() string { return fmt.Sprintf("mdl %d", int(v)) }

// HeaderLine is the header describe and fmt write at the top of a script:
// the newest frozen version's, "mdl 1;".
func HeaderLine() string { return HeaderFor(Frozen) }

// HeaderFor is the header that makes a script mean what output written in v
// means: "mdl 1;" for mdl 1, and "" for mdl 0, which is only ever implicit in
// output (a headerless script is mdl 0 to every mxcli release).
func HeaderFor(v Version) string {
	if v <= V0 {
		return ""
	}
	return v.String() + ";"
}

// ParseFlag reads a `--mdl <n>` value: a version this mxcli knows, written as
// its number ("1") or as its header spelling without the terminator ("mdl 1").
func ParseFlag(s string) (Version, error) {
	t := strings.TrimSpace(s)
	if len(t) > 3 && strings.EqualFold(t[:3], "mdl") {
		t = strings.TrimSpace(t[3:])
	}
	n, err := strconv.Atoi(t)
	if err != nil {
		return V0, fmt.Errorf("--mdl %q: the language version is a whole number, e.g. --mdl 1", s)
	}
	v := Version(n)
	if !v.Known() {
		return V0, fmt.Errorf("--mdl %s: this mxcli understands mdl 0 through mdl %d", t, int(Latest))
	}
	return v, nil
}

// UnknownVersionError is the message for a header naming a version this mxcli
// does not know. It takes the version as written, since a whole number too
// large for a Version is unknown rather than malformed.
func UnknownVersionError(written string) string {
	return fmt.Sprintf("unknown MDL language version %s: this mxcli understands mdl 0 through mdl %d. "+
		"Running a script under rules older than the ones it was written for would change "+
		"its meaning; upgrade mxcli.", written, int(Latest))
}

// headerLineRe matches a line holding only a language header.
var headerLineRe = regexp.MustCompile(`(?i)^\s*mdl\s+\d+\s*;\s*(--.*)?$`)

// IsHeaderLine reports whether a source line holds a language header and
// nothing else (a trailing `--` comment aside).
func IsHeaderLine(line string) bool { return headerLineRe.MatchString(line) }

// Change is one construct whose meaning differs between language versions: a
// change of meaning or a new rejection from PROPOSAL_mdl_beta_syntax_freeze.md
// §5, gated on the header per ADR-0011.
//
// Declare one per change, next to the code that implements both meanings, and
// branch on Applies. Under an older version the caller keeps the old meaning
// and reports Warning, so a headerless script names every construct whose
// meaning differs under the newer language.
type Change struct {
	// Code is the rule ID reported with the old-version warning.
	Code string
	// Since is the first version with the new meaning.
	Since Version
	// Old describes what the construct means before Since.
	Old string
	// New describes what it means from Since on.
	New string
}

// Applies reports whether the new meaning is in force for a script written in v.
func (c Change) Applies(v Version) bool { return v >= c.Since }

// Warning is the message for a construct kept at its old meaning because the
// script is written in a version before Since. It ends with HelpHint, so the
// reader is one command away from the change's full entry.
func (c Change) Warning(v Version) string {
	return fmt.Sprintf("%s under %s; under %s it means %s. "+
		"Start the script with `%s;` to opt in, or keep this meaning explicitly. %s",
		c.Old, v, c.Since, c.New, c.Since, HelpHint(c.Code))
}

// HelpHint is the pointer every warning carrying a language-change or
// deprecation code ends with: "(mxcli help MDL-V1-LIMIT1)". `mxcli help <code>`
// prints the code's entry — old form, new form, whether `fmt --upgrade`
// rewrites it, and the version that refuses it.
func HelpHint(code string) string { return "(mxcli help " + code + ")" }

// ScanHeader reads the language version from the `mdl <n>;` header src starts
// with, before it is parsed. It is for the one decision that has to be made
// before lexing: under mdl 1 a backslash in a string literal is an ordinary
// character, which changes where the literal ends (ADR-0010 R11).
//
// It reads the header exactly as the grammar does — the first statement,
// after any whitespace, `--` and `/* */` comments, with the same between its
// tokens — and returns V0 when there is none. A header naming a version this
// mxcli does not know also returns V0: the parser refuses the script anyway.
func ScanHeader(src string) Version {
	v, _ := ScanWrittenHeader(src)
	return v
}

// ScanWrittenHeader is ScanHeader that also reports whether src states a
// header at all, so that an explicit `mdl 0;` can be told from no header —
// the REPL switches its session on the one and not on the other.
func ScanWrittenHeader(src string) (Version, bool) {
	i := skipTrivia(src, 0)
	j := i
	for j < len(src) && isIdentByte(src[j]) {
		j++
	}
	if !strings.EqualFold(src[i:j], "mdl") {
		return V0, false
	}
	i = skipTrivia(src, j)
	j = i
	for j < len(src) && src[j] >= '0' && src[j] <= '9' {
		j++
	}
	if j == i || (j < len(src) && (isIdentByte(src[j]) || src[j] == '.')) {
		return V0, false
	}
	n, err := strconv.Atoi(src[i:j])
	if err != nil {
		return V0, false
	}
	if k := skipTrivia(src, j); k >= len(src) || src[k] != ';' {
		return V0, false
	}
	if v := Version(n); v.Known() {
		return v, true
	}
	return V0, false
}

// skipTrivia returns the index of the first byte at or after i that is not
// whitespace or inside a `--` or `/* */` comment. A `/** */` doc comment is a
// token, not trivia, so it stops the scan.
func skipTrivia(src string, i int) int {
	for i < len(src) {
		switch {
		case src[i] == ' ' || src[i] == '\t' || src[i] == '\r' || src[i] == '\n' || src[i] == '\v' || src[i] == '\f':
			i++
		case strings.HasPrefix(src[i:], "--"):
			if nl := strings.IndexByte(src[i:], '\n'); nl >= 0 {
				i += nl + 1
			} else {
				return len(src)
			}
		case strings.HasPrefix(src[i:], "/*") && !strings.HasPrefix(src[i:], "/**"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return len(src)
			}
			i += 2 + end + 2
		default:
			return i
		}
	}
	return i
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}
