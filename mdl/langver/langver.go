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
//   - A version is a PREVIEW until it is frozen. A preview parses but warns
//     "preview: may still change", and describe/fmt do not emit its header.
//     Freezing is one edit to Frozen below.
//
// The language version is independent of the Mendix target version (ADR-0011
// decision 3); nothing here reads or writes a Mendix version.
package langver

import (
	"fmt"
	"regexp"
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

// Frozen is the newest FROZEN version. It is the single preview/frozen switch.
//
// Every version above Frozen and up to Latest is a preview: it parses, warns
// that it may still change, and describe and fmt do not emit it. At beta this
// becomes V1 and nothing else has to change: the preview warning stops, and
// HeaderLine starts returning "mdl 1;" for describe and fmt to write.
//
// After that, a change of meaning needs a new version (V2), which starts life
// as a preview by being above Frozen.
const Frozen = V0

// Default is the version of a script that has no header.
const Default = V0

// Known reports whether this mxcli can run a script written in v.
func (v Version) Known() bool { return v >= V0 && v <= Latest }

// IsPreview reports whether v is a version that may still change.
func (v Version) IsPreview() bool { return isPreview(v, Frozen) }

func isPreview(v, frozen Version) bool { return v > frozen && v <= Latest }

// String renders the header spelling of v without the terminator: "mdl 1".
func (v Version) String() string { return fmt.Sprintf("mdl %d", int(v)) }

// HeaderLine is the header describe and fmt write at the top of a script, or
// "" when they write none.
//
// It is the newest frozen version, never a preview: output carrying a preview
// header would pin the reader's script to rules that may still change under
// it. While Frozen is V0 this is "", because mdl 0 is only ever implicit.
func HeaderLine() string { return headerLine(Frozen) }

func headerLine(frozen Version) string {
	if frozen <= V0 {
		return ""
	}
	return frozen.String() + ";"
}

// PreviewWarning is the message for a header naming a preview version.
func PreviewWarning(v Version) string {
	return fmt.Sprintf("%s is a preview: may still change. It is not frozen until beta, "+
		"so a script written against it can behave differently under a later mxcli release.", v)
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
// script is written in a version before Since.
func (c Change) Warning(v Version) string {
	return fmt.Sprintf("%s under %s; under %s it means %s. "+
		"Start the script with `%s;` to opt in, or keep this meaning explicitly.",
		c.Old, v, c.Since, c.New, c.Since)
}
