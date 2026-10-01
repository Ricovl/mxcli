// SPDX-License-Identifier: Apache-2.0

// Package executor - diff results and their rendering, shared by `mxcli diff`
// (mdl/scriptdiff, which runs the script against a scratch copy) and
// `mxcli diff-local`.
package executor

import (
	"fmt"
	"io"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// DiffFormat represents the output format for diff results
type DiffFormat string

const (
	DiffFormatUnified    DiffFormat = "unified"
	DiffFormatSideBySide DiffFormat = "side"
	DiffFormatStructural DiffFormat = "struct"
)

// DiffOptions configures diff output
type DiffOptions struct {
	Format   DiffFormat
	UseColor bool
	Width    int
}

// ChangeType represents the type of structural change
type ChangeType string

const (
	ChangeAdded    ChangeType = "+"
	ChangeRemoved  ChangeType = "-"
	ChangeModified ChangeType = "~"
)

// StructuralChange represents a single structural change within an object
type StructuralChange struct {
	ChangeType  ChangeType
	ElementType string // "Attribute", "Parameter", "Value", etc.
	ElementName string
	Details     string
}

// DiffResult represents the diff for a single object
type DiffResult struct {
	ObjectType string
	ObjectName ast.QualifiedName
	Current    string // MDL from MPR (empty if new)
	Proposed   string // MDL after the change (empty if deleted)
	IsNew      bool
	IsDeleted  bool
	Changes    []StructuralChange
	// Writes is what is written that the two renderings do not show (a move
	// to another folder, a property describe does not print), or why they
	// could not be made; "" otherwise.
	Writes string
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
)

// WriteDiffResults prints results in the chosen format. A result whose change
// the renderings do not show is printed as one line saying what is written.
func WriteDiffResults(w io.Writer, results []DiffResult, opts DiffOptions) {
	if opts.Format == "" {
		opts.Format = DiffFormatUnified
	}
	if opts.Width == 0 {
		opts.Width = 120
	}
	ctx := &ExecContext{Output: w}
	for _, result := range results {
		verdict := "Modified"
		switch {
		case result.IsNew:
			verdict = "New"
		case result.IsDeleted:
			verdict = "Removed"
		}
		shows := result.Current != result.Proposed
		if !shows {
			fmt.Fprintf(w, "%s: %s %s: %s\n", verdict, result.ObjectType, result.ObjectName, result.Writes)
			continue
		}
		switch opts.Format {
		case DiffFormatSideBySide:
			outputSideBySideDiff(ctx, result, opts.Width, opts.UseColor)
		case DiffFormatStructural:
			outputStructuralDiff(ctx, result, opts.UseColor)
		default:
			outputUnifiedDiff(ctx, result, opts.UseColor)
		}
		if result.Writes != "" {
			fmt.Fprintf(w, "%s: %s %s: also %s\n\n", verdict, result.ObjectType, result.ObjectName, result.Writes)
		}
	}
}

// LineChanges summarises the difference between two renderings as lines added
// and removed, for the structural format.
func LineChanges(current, proposed string) []StructuralChange {
	if current == proposed {
		return nil
	}
	lines := func(s string) []string {
		if s == "" {
			return nil
		}
		return difflib.SplitLines(s)
	}
	m := difflib.NewMatcher(lines(current), lines(proposed))
	added, removed := 0, 0
	for _, op := range m.GetOpCodes() {
		switch op.Tag {
		case 'r':
			removed += op.I2 - op.I1
			added += op.J2 - op.J1
		case 'd':
			removed += op.I2 - op.I1
		case 'i':
			added += op.J2 - op.J1
		}
	}
	var out []StructuralChange
	if added > 0 {
		out = append(out, StructuralChange{ChangeType: ChangeAdded, ElementType: "Lines", Details: fmt.Sprintf("%d line(s) added", added)})
	}
	if removed > 0 {
		out = append(out, StructuralChange{ChangeType: ChangeRemoved, ElementType: "Lines", Details: fmt.Sprintf("%d line(s) removed", removed)})
	}
	return out
}
