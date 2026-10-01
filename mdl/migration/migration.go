// SPDX-License-Identifier: Apache-2.0

// Package migration is the generated half of the migration reference: what
// changes between MDL language versions, read from the two registries that
// define it, and nowhere else.
//
//   - Every change of meaning or new rejection is a langver.Change, declared
//     next to the code that implements both meanings and listed by
//     visitor.LanguageChanges (decided while parsing) or
//     executor.LanguageChanges (decided at exec time, against the project).
//   - Every deprecated spelling is an entry in mdl/deprecation.
//
// Both surfaces that explain them render from here: the tables on the docs
// page "Language versions and migration" (GeneratedTables, spliced into the
// page by cmd/gen-migration-reference, kept current by
// TestMigrationReferenceIsCurrent), and `mxcli help <code>` (Help). A change
// registered in code reaches both without anyone editing prose (ako/mxcli#714,
// decisions 2 and 3).
package migration

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Decided says where a language change takes effect.
type Decided string

const (
	// Parse is a change the visitor makes while reading the script: `check`
	// reports it without a project, and `fmt --upgrade` sees it.
	Parse Decided = "parse"
	// Exec is a change made while executing against the project: `check -p`
	// reports it, `fmt --upgrade` without -p cannot.
	Exec Decided = "exec"
)

// Change is one langver.Change with what the upgrade does about it.
type Change struct {
	langver.Change
	Decided Decided
	// Rewritable is true when `fmt --upgrade --header` rewrites the construct
	// to the spelling that keeps its old meaning under the header.
	Rewritable bool
	// VersionNeutral is true when that rewrite is applied without --header too.
	VersionNeutral bool
	// NoRewrite is why there is no rewrite, for a parse-time change without one.
	NoRewrite string
}

// Changes returns every language change, sorted by code.
func Changes() []Change {
	var out []Change
	for _, c := range visitor.LanguageChanges() {
		rw, neutral, reason, _ := upgrade.GatedRewrite(c.Code)
		out = append(out, Change{Change: c, Decided: Parse, Rewritable: rw, VersionNeutral: neutral, NoRewrite: reason})
	}
	for _, c := range executor.LanguageChanges() {
		out = append(out, Change{Change: c, Decided: Exec})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Deprecations returns every deprecated spelling, sorted by code.
func Deprecations() []deprecation.Entry {
	out := deprecation.All()
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// RewriteSummary says, in one line, what `fmt --upgrade` does with c.
func (c Change) RewriteSummary() string {
	switch {
	case c.Decided == Exec:
		return "no: decided by exec against the project; `check -p` reports it"
	case c.Rewritable && c.VersionNeutral:
		return "yes: `fmt --upgrade`, with or without `--header`"
	case c.Rewritable:
		return "yes: `fmt --upgrade --header`"
	default:
		return "no: blocks `--header`; " + c.NoRewrite
	}
}

// DeprecationRewrite says, in one line, what `fmt --upgrade` does with e.
func DeprecationRewrite(e deprecation.Entry) string {
	switch {
	case e.Rewrite.Structural != "":
		return "yes: " + e.Rewrite.Structural
	case e.Rewrite.Token != "":
		return fmt.Sprintf("yes: `%s` → `%s`", e.Rewrite.Token, e.Rewrite.Replacement)
	default:
		return "no: `fmt --upgrade` reports each use; rewrite it by hand"
	}
}

// IsCode reports whether s has the shape of a code Help answers for, so a
// caller can tell a misspelt code from a command name.
func IsCode(s string) bool {
	u := strings.ToUpper(s)
	return strings.HasPrefix(u, "MDL-DEPR") || strings.HasPrefix(u, "MDL-V1-")
}

// Help renders the registry entry for code (case-insensitive): a deprecated
// spelling or a language change. ok is false for a code neither registry has.
func Help(code string) (text string, ok bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var b strings.Builder
	if e, found := deprecation.Lookup(code); found {
		fmt.Fprintf(&b, "%s: deprecated spelling\n\n", e.Code)
		field(&b, "Old form", e.Old)
		field(&b, "New form", e.Canonical)
		field(&b, "Rewrite", DeprecationRewrite(e))
		field(&b, "Refused from", fmt.Sprintf("mdl %d; warns under every earlier version", e.RemovedIn))
		if t := e.Topics(); len(t) > 0 {
			field(&b, "Syntax topics", strings.Join(t, ", "))
		}
		if e.Note != "" {
			field(&b, "Note", e.Note)
		}
		fmt.Fprintf(&b, "\nExample, old form:\n  %s\nExample, new form:\n  %s\n",
			indent(e.Example), indent(e.CanonicalExample))
		b.WriteString(footer)
		return b.String(), true
	}
	for _, c := range Changes() {
		if c.Code != code {
			continue
		}
		fmt.Fprintf(&b, "%s: change of meaning from %s\n\n", c.Code, c.Since)
		field(&b, fmt.Sprintf("Before %s", c.Since), c.Old)
		field(&b, fmt.Sprintf("From %s", c.Since), c.New)
		field(&b, "Rewrite", c.RewriteSummary())
		field(&b, "Applies from", fmt.Sprintf("the `%s;` header; a script without it keeps the old meaning and warns", c.Since))
		b.WriteString(footer)
		return b.String(), true
	}
	return "", false
}

const footer = "\nMigrating a script: mxcli fmt --upgrade --header -w -p app.mpr script.mdl\n" +
	"Every code: docs site, \"Language versions and migration\".\n"

func field(b *strings.Builder, name, value string) {
	fmt.Fprintf(b, "  %-14s %s\n", name+":", value)
}

func indent(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n  ") }
