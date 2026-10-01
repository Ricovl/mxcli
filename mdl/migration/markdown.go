// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"fmt"
	"strings"
)

// The generated tables sit between these markers on the docs page. Everything
// outside them is hand-written and left alone by Splice.
const (
	BeginMarker = "<!-- BEGIN GENERATED: mxcli migration reference (make gen-migration-reference) -->"
	EndMarker   = "<!-- END GENERATED: mxcli migration reference -->"
)

// PagePath is the docs page the tables are generated into, relative to the
// repository root.
const PagePath = "docs-site/src/language/versions.md"

// GeneratedTables renders the two tables: every language change and every
// deprecated spelling.
func GeneratedTables() string {
	var b strings.Builder
	b.WriteString("<!-- Do not edit by hand: generated from visitor.LanguageChanges, executor.LanguageChanges\n" +
		"     and mdl/deprecation by cmd/gen-migration-reference. -->\n\n")

	changes := Changes()
	fmt.Fprintf(&b, "### Changes of meaning (`MDL-V1-*`)\n\n"+
		"%d constructs mean something different under the `mdl 1;` header. Without the header each keeps "+
		"the meaning in the second column and warns with its code.\n\n", len(changes))
	b.WriteString("| Code | Without the header (mdl 0) | Under `mdl 1;` | Decided at | Rewritten by `fmt --upgrade` |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, c := range changes {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n",
			c.Code, cell(c.Old), cell(c.New), c.Decided, cell(c.RewriteSummary()))
	}

	deps := Deprecations()
	fmt.Fprintf(&b, "\n### Deprecated spellings (`MDL-DEPR*`)\n\n"+
		"%d old spellings mean exactly what their new form means. They warn with their code under every "+
		"version before the one in the last column, which refuses them.\n\n", len(deps))
	b.WriteString("| Code | Old form | New form | Rewritten by `fmt --upgrade` | Refused from |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, e := range deps {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | mdl %d |\n",
			e.Code, code(e.Old), code(e.Canonical), cell(DeprecationRewrite(e)), e.RemovedIn)
	}
	return b.String()
}

// Splice returns page with the text between the markers replaced by
// generated. The markers must each appear exactly once, in order.
func Splice(page, generated string) (string, error) {
	if strings.Count(page, BeginMarker) != 1 || strings.Count(page, EndMarker) != 1 {
		return "", fmt.Errorf("the page must hold %q and %q exactly once each", BeginMarker, EndMarker)
	}
	start := strings.Index(page, BeginMarker) + len(BeginMarker)
	end := strings.Index(page, EndMarker)
	if end < start {
		return "", fmt.Errorf("%q comes before %q", EndMarker, BeginMarker)
	}
	return page[:start] + "\n" + generated + "\n" + page[end:], nil
}

// cell makes prose safe inside a table cell: a `|` would end the cell, a line
// break the row.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", `\|`)
}

// code renders a spelling as inline code inside a table cell. A spelling that
// holds a backtick takes a double-backtick span.
func code(s string) string {
	s = cell(s)
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}
