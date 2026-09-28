// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// OutputFormat controls how command results are rendered.
type OutputFormat string

const (
	// FormatTable renders results as a pipe-delimited markdown table (default).
	FormatTable OutputFormat = "table"
	// FormatJSON renders results as a JSON array of objects.
	FormatJSON OutputFormat = "json"
)

// TableResult holds structured tabular data that can be rendered in multiple formats.
type TableResult struct {
	Columns []string // column headers
	Rows    [][]any  // row data (one slice per row, matching Columns order)
	Summary string   // optional summary line, e.g. "(42 entities)"
}

// writeResult renders a TableResult to ctx.Output in the current format.
func writeResult(ctx *ExecContext, r *TableResult) error {
	if ctx.Format == FormatJSON {
		return writeResultJSON(ctx, r)
	}
	writeResultTable(ctx, r)
	return nil
}

// writeEmptyResult reports a query that matched nothing. In text mode that is
// the sentence a person expects ("(no references found)"); in JSON mode it is
// an empty array, with the sentence moved to the progress stream. An empty
// answer is exactly the case a caller must be able to parse — a sentence there
// reads as a corrupt payload rather than as "no".
func writeEmptyResult(ctx *ExecContext, columns []string, message string) error {
	if ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, message)
		return nil
	}
	fmt.Fprintln(ctx.progress(), message)
	return writeResultJSON(ctx, &TableResult{Columns: columns})
}

// writeResultTable renders a TableResult as a pipe-delimited markdown table.
func writeResultTable(ctx *ExecContext, r *TableResult) {
	if len(r.Columns) == 0 {
		return
	}

	// Calculate column widths from headers and data.
	widths := make([]int, len(r.Columns))
	for i, col := range r.Columns {
		widths[i] = len(col)
	}
	for _, row := range r.Rows {
		for i, val := range row {
			if i >= len(widths) {
				break
			}
			s := formatCellValue(val)
			if len(s) > widths[i] {
				widths[i] = len(s)
			}
		}
	}

	// Print header.
	fmt.Fprint(ctx.Output, "|")
	for i, col := range r.Columns {
		fmt.Fprintf(ctx.Output, " %-*s |", widths[i], col)
	}
	fmt.Fprintln(ctx.Output)

	// Print separator.
	fmt.Fprint(ctx.Output, "|")
	for _, w := range widths {
		fmt.Fprintf(ctx.Output, "-%s-|", strings.Repeat("-", w))
	}
	fmt.Fprintln(ctx.Output)

	// Print rows.
	for _, row := range r.Rows {
		fmt.Fprint(ctx.Output, "|")
		for i := range r.Columns {
			var s string
			if i < len(row) {
				s = formatCellValue(row[i])
			}
			fmt.Fprintf(ctx.Output, " %-*s |", widths[i], s)
		}
		fmt.Fprintln(ctx.Output)
	}

	// Print summary.
	if r.Summary != "" {
		fmt.Fprintf(ctx.Output, "\n%s\n", r.Summary)
	}
}

// writeResultJSON renders a TableResult as a JSON array of objects.
func writeResultJSON(ctx *ExecContext, r *TableResult) error {
	objects := make([]map[string]any, 0, len(r.Rows))
	for _, row := range r.Rows {
		obj := make(map[string]any, len(r.Columns))
		for i, col := range r.Columns {
			if i < len(row) {
				obj[col] = row[i]
			}
		}
		objects = append(objects, obj)
	}

	enc := json.NewEncoder(ctx.Output)
	enc.SetIndent("", "  ")
	return enc.Encode(objects)
}

// writeDescribeJSON wraps a describe handler's output in a JSON envelope.
// In table/text mode it calls fn directly. In JSON mode it captures fn's output
// and wraps it as {"name": ..., "type": ..., "mdl": ...}.
func writeDescribeJSON(ctx *ExecContext, name, objectType string, fn func() error) error {
	return writeDescribeJSONAs(ctx, name, objectType, true, fn)
}

// writeDescribeJSONAs is writeDescribeJSON for an answer that may not be MDL:
// executable false marks a definition report (a widget type, a glyph, a
// contract), whose text is under "mdl" like every describe but is not runnable.
func writeDescribeJSONAs(ctx *ExecContext, name, objectType string, executable bool, fn func() error) error {
	if ctx.Format != FormatJSON {
		return fn()
	}

	// Capture the text output from fn.
	var buf bytes.Buffer
	origOutput := ctx.Output
	ctx.Output = &buf

	err := fn()
	ctx.Output = origOutput
	if err != nil {
		return err
	}

	result := map[string]any{
		"name":       name,
		"type":       objectType,
		"mdl":        buf.String(),
		"executable": executable,
	}
	enc := json.NewEncoder(ctx.Output)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// ----------------------------------------------------------------------------
// Executor method wrappers (for callers in unmigrated files)
// ----------------------------------------------------------------------------

func (e *Executor) writeResult(r *TableResult) error {
	return writeResult(e.newExecContext(context.Background()), r)
}

// formatCellValue formats a value for table cell display.
func formatCellValue(val any) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}
