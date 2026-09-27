// SPDX-License-Identifier: Apache-2.0

// Package formatter provides MDL code formatting.
package formatter

import (
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// Format formats MDL source code:
//   - Lowercase MDL keywords, the canonical case (R8, ako/mxcli#752). Only the
//     words the parse tree shows are keywords change: a name spelled like a
//     keyword (`Issue64.User`, an attribute `Title`), a property key
//     (`Folder:`) and a CamelCase value (`ButtonStyle: Success`) keep the
//     author's case.
//   - Normalize indentation to 2 spaces.
//   - Remove trailing whitespace.
//   - Normalize blank lines (max 1 consecutive).
//
// Text the model stores as written — expressions, XPath, OQL, queries, and
// string literals or code blocks that span lines — keeps its case and layout,
// so formatting never changes what a script builds. A script that does not
// parse keeps its case and is only re-indented.
func Format(input string) string {
	spans, ok := visitor.FormatSpans(input)
	if ok {
		input = visitor.LowercaseKeywords(input, spans.Keywords)
	}
	inside := verbatimIndex(spans.Verbatim)

	lines := strings.Split(input, "\n")
	var result []string
	prevBlank := false
	offset := 0 // rune offset of the current line's start

	for _, line := range lines {
		start, end := offset, offset+len([]rune(line)) // end: the '\n' after the line
		offset = end + 1

		// A line that starts inside verbatim text is part of that text.
		if inside(start) {
			result = append(result, line)
			prevBlank = false
			continue
		}
		trimmed := line
		if !inside(end) {
			trimmed = strings.TrimRight(line, " \t\r")
		}

		// Collapse multiple blank lines
		if trimmed == "" {
			if !prevBlank {
				result = append(result, "")
			}
			prevBlank = true
			continue
		}
		prevBlank = false

		// Normalize indentation: count leading spaces, normalize to 2-space units
		stripped := strings.TrimLeft(trimmed, " \t")
		// Count effective indent (tabs = 2 spaces)
		indent := 0
		for _, ch := range trimmed {
			if ch == ' ' {
				indent++
			} else if ch == '\t' {
				indent += 2
			} else {
				break
			}
		}
		// Round to nearest 2-space unit
		normalizedIndent := strings.Repeat("  ", indent/2)
		result = append(result, normalizedIndent+stripped)
	}

	// Remove trailing blank line
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n") + "\n"
}

// verbatimIndex returns whether a rune offset lies strictly inside one of the
// spans: after its first rune and before its end.
func verbatimIndex(spans [][2]int) func(int) bool {
	sorted := append([][2]int(nil), spans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i][0] < sorted[j][0] })
	return func(off int) bool {
		for _, s := range sorted {
			if s[0] >= off {
				return false
			}
			if off < s[1] {
				return true
			}
		}
		return false
	}
}
