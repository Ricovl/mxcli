// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// foldElseIntoElsif turns an else branch that holds nothing but one if into an
// elsif arm (R12, #750).
//
// Mendix has no elsif: the visitor lowers `elsif C then …` into a nested if that
// is the whole of the previous arm's else branch, so the two spellings build
// the same graph and the fold is purely a matter of text. Before it, a chain
// written as if/elsif/elsif/else came back as three levels of nesting.
//
// The check is made on the emitted lines rather than on the graph, because the
// question is "is the else body exactly one if statement" and the emitted body
// answers it directly — whatever the traversal decided about guards, merges and
// continuations is already settled there. A body qualifies when its lines at the
// body's own indent are, in order:
//
//	@annotation / -- comment lines (the if's own annotations)
//	if … then
//	else | elsif … then | @annotation (arms of that if, already folded)
//	end if;   (the last line of the body)
//
// Anything else at that indent — a second statement, a `merge`/`join` label — is
// a statement the fold would move into the last arm, so the branch stays nested.
// Deeper lines are the if's own bodies. An element holding a multi-line
// expression carries its continuation inside the same element, so only its first
// line is re-indented and the expression text is left exactly as stored.
//
// The annotations of the nested if move with it and are written before the
// `elsif` keyword, which is where the grammar accepts an arm's annotations.
//
// elseIdx is the index of the `else` line; indent is the if's own indent.
// sourceMap line numbers recorded inside the branch are shifted to match.
func foldElseIntoElsif(lines *[]string, elseIdx, indent int, sourceMap map[string]elkSourceRange, headerLineCount int) {
	remap := foldElseIntoElsifLines(lines, elseIdx, indent)
	if remap == nil || sourceMap == nil {
		return
	}
	for id, r := range sourceMap {
		if r.EndLine-headerLineCount < elseIdx {
			continue
		}
		sourceMap[id] = elkSourceRange{
			StartLine: remap(r.StartLine-headerLineCount) + headerLineCount,
			EndLine:   remap(r.EndLine-headerLineCount) + headerLineCount,
		}
	}
}

// foldElseIntoElsifLines is the fold itself. It returns nil when the branch is
// not a lone if, and otherwise a function mapping an element index of the old
// lines to its index in the new ones, for callers that recorded line numbers
// inside the branch.
func foldElseIntoElsifLines(lines *[]string, elseIdx, indent int) func(int) int {
	if elseIdx < 0 || elseIdx >= len(*lines) {
		return nil
	}
	body := (*lines)[elseIdx+1:]
	if len(body) < 2 {
		return nil
	}
	inner := strings.Repeat("  ", indent+1)
	if body[len(body)-1] != inner+"end if;" {
		return nil
	}

	ifIdx := -1
	for i, l := range body {
		rest, ok := strings.CutPrefix(l, inner)
		if !ok || strings.HasPrefix(rest, " ") {
			continue // a line of a nested body, or an expression continuation
		}
		if ifIdx < 0 {
			switch {
			case strings.HasPrefix(rest, "@"), strings.HasPrefix(rest, "--"):
				continue
			case strings.HasPrefix(rest, "if "):
				ifIdx = i
				continue
			}
			return nil
		}
		switch {
		case i == len(body)-1, rest == "else", strings.HasPrefix(rest, "elsif "),
			strings.HasPrefix(rest, "@"), strings.HasPrefix(rest, "--"):
			continue
		}
		return nil
	}
	if ifIdx < 0 {
		return nil
	}

	folded := make([]string, 0, len(body)-1)
	for i, l := range body[:len(body)-1] {
		l = strings.TrimPrefix(l, "  ")
		if i == ifIdx {
			l = strings.Repeat("  ", indent) + "elsif " + strings.TrimPrefix(l, strings.Repeat("  ", indent)+"if ")
		}
		folded = append(folded, l)
	}
	*lines = append((*lines)[:elseIdx], folded...)

	// Element k of the old block moves to: k (up to the else, whose line the
	// first moved line now takes), k-1 (inside the branch, once the else line
	// is gone), and the nested if's `end if;` — now removed — maps to the last
	// line left in the branch.
	endIdx := elseIdx + len(body)
	return func(k int) int {
		switch {
		case k <= elseIdx:
			return k
		case k < endIdx:
			return k - 1
		default:
			return k - 2
		}
	}
}
