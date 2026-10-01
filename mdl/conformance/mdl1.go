// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

// The mdl 1 classes (decision 4 on ako/mxcli#714): what the skills and the
// `mxcli init` templates teach is mdl 1, so a complete script starts with
// `mdl 1;` and a fragment, which carries no header, is valid mdl 1.
const (
	// ClassMissingHeader is a complete script without the `mdl 1;` header.
	ClassMissingHeader = "mdl1-header"
	// ClassNotMDL1 is MDL that does not parse as mdl 1: a deprecated
	// spelling mdl 1 refuses, a `/` terminator, a missing `;`, a session
	// command, or a header naming another version.
	ClassNotMDL1 = "mdl1"
)

// mdl1Header is the header a complete script starts with.
const mdl1Header = "mdl 1;"

// queryStatementPrefixes name the statements that only read: a block made of
// these alone is a command for the REPL or `-c`, not a script, and carries no
// header. Anything else (create, alter, drop, grant, move, …) makes the block
// a script someone writes to a file and executes.
var queryStatementPrefixes = []string{"Show", "Describe", "Select", "Search", "Refresh", "Explain", "Help", "Analyze"}

func isQueryStatement(s any) bool {
	t := reflect.TypeOf(s)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	for _, p := range queryStatementPrefixes {
		if strings.HasPrefix(t.Name(), p) {
			return true
		}
	}
	return false
}

// CheckMDL1 returns the mdl 1 findings for one documentation unit:
//
//   - a block with a header must parse as the script it is, under mdl 1;
//   - a headerless block of top-level statements that writes anything is a
//     complete script and must start with `mdl 1;` (ClassMissingHeader);
//   - any other headerless block — a read-only command, or a fragment written
//     for a microflow body, a page, a workflow or a retrieve — must parse under
//     mdl 1 in the context it is written for (ClassNotMDL1).
//
// A block that parses under neither version in any context is a template
// (`<Name>`, `...`) and is not reported here: the canonical-form gate counts
// those. A whole-script unit (Unit.Script, a shipped .mdl file) must carry the
// header and parse under it.
func CheckMDL1(u Unit) []Finding {
	text := stripNonMDLKeepSlash(u.Text)
	if strings.TrimSpace(stripComments(text)) == "" {
		return nil
	}
	if langver.ScanHeader(text) != langver.V0 || hasHeaderLine(text) {
		prog, errs := build(text)
		if len(errs) > 0 {
			return []Finding{{Source: u.Source, Line: u.Line, Class: ClassNotMDL1, Detail: firstLine(errs[0].Error())}}
		}
		if prog.LanguageVersion != langver.V1 {
			return []Finding{{Source: u.Source, Line: u.Line, Class: ClassNotMDL1,
				Detail: "the header names " + prog.LanguageVersion.String() + "; write `mdl 1;`"}}
		}
		return nil
	}
	if u.Script {
		return []Finding{{Source: u.Source, Line: u.Line, Class: ClassMissingHeader,
			Detail: "a script starts with `mdl 1;`"}}
	}

	// A block of top-level statements.
	if prog, errs := build(text); len(errs) == 0 && len(prog.Statements) > 0 {
		for _, s := range prog.Statements {
			if !isQueryStatement(s) {
				return []Finding{{Source: u.Source, Line: u.Line, Class: ClassMissingHeader,
					Detail: "a complete script starts with `mdl 1;` (run `mxcli fmt --upgrade --header` on it)"}}
			}
		}
	}

	if f, decided := judgeMDL1(text); decided {
		return located(u, 0, f)
	}
	// Neither version parses the block whole: check its blank-line-separated
	// snippets one by one, so a template next to a real statement does not
	// hide the statement.
	var out []Finding
	for _, c := range splitChunks(text) {
		if f, decided := judgeMDL1(c.text); decided {
			out = append(out, located(u, c.offset, f)...)
		}
	}
	return out
}

// located places a judgement at the unit's line plus offset.
func located(u Unit, offset int, f *Finding) []Finding {
	if f == nil {
		return nil
	}
	f.Source, f.Line = u.Source, u.Line+offset
	return []Finding{*f}
}

// judgeMDL1 tries text in each context, in order, as mdl 1 and as mdl 0. The
// first context either version parses it in is the one it is written for, and
// decides: it must parse as mdl 1 there, and mean there what it meant as mdl 0
// (a LanguageNote is a construct whose meaning the header changes, such as
// `limit 1`). decided is false when no context parses it at all — a template.
// A list of alternatives, one per line, is judged line by line.
func judgeMDL1(text string) (f *Finding, decided bool) {
	for _, c := range Contexts {
		_, errs1 := build(mdl1Header + "\n" + c.Wrap(text))
		prog0, errs0 := build(c.Wrap(text))
		switch {
		case len(errs1) == 0 && len(errs0) == 0 && len(prog0.LanguageNotes) > 0:
			n := prog0.LanguageNotes[0]
			return &Finding{Class: ClassNotMDL1, Detail: n.Code + ": " + firstLine(n.Message)}, true
		case len(errs1) == 0:
			return nil, true
		case len(errs0) == 0:
			return &Finding{Class: ClassNotMDL1, Detail: firstLine(errs1[0].Error())}, true
		}
	}
	lines := nonEmptyLines(text)
	if len(lines) < 2 {
		return nil, false
	}
	for _, l := range lines {
		lf, ok := judgeMDL1(l.text)
		if !ok {
			return nil, false
		}
		if lf != nil {
			return lf, true
		}
	}
	return nil, true
}

func hasHeaderLine(text string) bool {
	for _, l := range strings.Split(text, "\n") {
		if langver.IsHeaderLine(l) {
			return true
		}
	}
	return false
}

// stripNonMDLKeepSlash is stripNonMDL without blanking the `/` separator:
// mdl 1 refuses it, so it must be seen.
func stripNonMDLKeepSlash(s string) string {
	lines := strings.Split(s, "\n")
	orig := append([]string(nil), lines...)
	stripped := strings.Split(stripNonMDL(s), "\n")
	for i := range stripped {
		if strings.TrimSpace(orig[i]) == "/" {
			stripped[i] = orig[i]
		}
	}
	return strings.Join(stripped, "\n")
}
