// SPDX-License-Identifier: Apache-2.0

// Package executor - diff results and their rendering, shared by `mxcli diff`
// (mdl/scriptdiff, which runs the script against a scratch copy) and
// `mxcli diff-local`.
package executor

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"go.mongodb.org/mongo-driver/v2/bson"

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
	// FromLabel / ToLabel name the two sides in the headers. "" keeps the
	// script diff's "current" / "script"; diff-local sets the git revisions,
	// since neither side of `--ref a..b` is a script (#1393).
	FromLabel string
	ToLabel   string
}

// sideLabels returns the header labels for the two sides of a diff.
func (r DiffResult) sideLabels() (from, to string) {
	from, to = r.FromLabel, r.ToLabel
	if from == "" {
		from = "current"
	}
	if to == "" {
		to = "script"
	}
	return from, to
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

// ChangedPaths lists up to max property paths whose values differ between two
// unit documents, element $IDs aside. A list element is named by its Name
// where it has one. It names properties, never values, so a secret that
// changed is reported by where it is stored and not by what it holds.
//
// Both diffs use it for a unit that is written although its MDL rendering
// does not change: `mxcli diff` for what exec would write, diff-local for what
// changed between two revisions (ako/mxcli#1512).
func ChangedPaths(a, b []byte, max int) []string {
	var out []string
	var walk func(path string, x, y bson.RawValue)
	walk = func(path string, x, y bson.RawValue) {
		if len(out) >= max {
			return
		}
		if x.Type != y.Type {
			out = append(out, path)
			return
		}
		switch x.Type {
		case bson.TypeEmbeddedDocument:
			dx, dy := x.Document(), y.Document()
			ex, _ := dx.Elements()
			keys := map[string]bool{}
			for _, e := range ex {
				k := e.Key()
				keys[k] = true
				if k == "$ID" {
					continue
				}
				vy, err := dy.LookupErr(k)
				if err != nil {
					out = append(out, diffPathJoin(path, k))
				} else {
					walk(diffPathJoin(path, k), e.Value(), vy)
				}
				if len(out) >= max {
					return
				}
			}
			ey, _ := dy.Elements()
			for _, e := range ey {
				if !keys[e.Key()] && len(out) < max {
					out = append(out, diffPathJoin(path, e.Key()))
				}
			}
		case bson.TypeArray:
			vx, _ := x.Array().Values()
			vy, _ := y.Array().Values()
			vx, vy = withoutListMarker(vx), withoutListMarker(vy)
			if len(vx) != len(vy) {
				out = append(out, fmt.Sprintf("%s (%d -> %d items)", path, len(vx), len(vy)))
				return
			}
			for i := range vx {
				walk(fmt.Sprintf("%s[%s]", path, diffPathItemName(vx[i], i)), vx[i], vy[i])
				if len(out) >= max {
					return
				}
			}
		case bson.TypeBinary:
			// A pointer holds an element $ID, so a pointer to an element exec
			// re-minted differs without anything having changed; canon has
			// already said whether the unit did. A GUID is reported.
			if !bytes.Equal(x.Value, y.Value) && !strings.HasSuffix(path, "Pointer") {
				out = append(out, path)
			}
		default:
			if !bytes.Equal(x.Value, y.Value) {
				out = append(out, path)
			}
		}
	}
	walk("", bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: a}, bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: b})
	return out
}

func diffPathJoin(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func diffPathItemName(v bson.RawValue, i int) string {
	if d, ok := v.DocumentOK(); ok {
		if n, ok := d.Lookup("Name").StringValueOK(); ok && n != "" {
			return n
		}
	}
	return fmt.Sprint(i)
}

// withoutListMarker drops the int32 Mendix stores ahead of a list's items (the
// typed-array marker), so a count or an index is the list's own: an empty
// list holds one value, and counting it reported a first grant as "1 -> 2
// items".
func withoutListMarker(vals []bson.RawValue) []bson.RawValue {
	if len(vals) > 0 && vals[0].Type == bson.TypeInt32 {
		return vals[1:]
	}
	return vals
}
