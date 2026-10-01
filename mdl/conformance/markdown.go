// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"strings"
)

// MarkdownLanguages are the fence info strings that hold MDL. The docs fence
// most MDL as ```sql, for the highlighting; a block that is not MDL — a
// counterexample, a template, real SQL — belongs in a ```text fence. A
// ```mdl-test fence is a test block, the MDL the test runner reads from a .md.
var MarkdownLanguages = map[string]bool{"mdl": true, "sql": true, "mdl-test": true}

// MarkdownUnits returns the MDL fenced blocks of a markdown document. A fence
// may be indented (a block inside a list item); the indentation of its opening
// line is removed from its content.
//
// An untagged fence is returned too, as a Lenient unit: it holds MDL often
// enough that a deprecated spelling in one must be seen, and something else (a
// transcript, a file tree) often enough that one that does not parse is not a
// finding.
//
// A fence inside a blockquote ("> ```mdl", the example in a callout) is read
// like any other: the quote marker is removed from its lines, which are the
// lines up to the matching quoted closing fence.
func MarkdownUnits(source, content string) []Unit {
	var out []Unit
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		quoted := false
		indent, marker, lang, ok := openFence(lines[i])
		if !ok {
			if q, isQuote := unquote(lines[i]); isQuote {
				indent, marker, lang, ok = openFence(q)
				quoted = ok
			}
		}
		if !ok {
			continue
		}
		at := func(n int) string {
			if quoted {
				q, _ := unquote(lines[n])
				return q
			}
			return lines[n]
		}
		start := i + 1
		end := start
		for end < len(lines) && !closesFence(at(end), marker) {
			end++
		}
		if MarkdownLanguages[lang] || lang == "" {
			body := make([]string, 0, end-start)
			for n := start; n < min(end, len(lines)); n++ {
				body = append(body, dedent(at(n), indent))
			}
			out = append(out, Unit{Source: source, Line: start + 1, Text: strings.Join(body, "\n"), Lenient: lang == ""})
		}
		i = end
	}
	return out
}

// openFence recognises a fence opening: optional indentation, three or more
// backticks or tildes, and an info string whose first word is the language.
func openFence(line string) (indent int, marker, lang string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	indent = len(line) - len(trimmed)
	for _, ch := range []string{"`", "~"} {
		n := 0
		for n < len(trimmed) && string(trimmed[n]) == ch {
			n++
		}
		if n < 3 {
			continue
		}
		info := strings.TrimSpace(trimmed[n:])
		if ch == "`" && strings.Contains(info, "`") {
			return 0, "", "", false // inline code, not a fence
		}
		if f := strings.Fields(info); len(f) > 0 {
			lang = strings.ToLower(f[0])
		}
		return indent, strings.Repeat(ch, n), lang, true
	}
	return 0, "", "", false
}

func closesFence(line, marker string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, marker) && strings.Trim(t, marker[:1]) == ""
}

// dedent removes up to n leading spaces or tabs.
func dedent(line string, n int) string {
	i := 0
	for i < n && i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[i:]
}

// unquote removes a blockquote marker — optional indentation, `>`, and one
// optional space — from a line, and reports whether the line had one.
func unquote(line string) (string, bool) {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, ">") {
		return line, false
	}
	t = t[1:]
	if strings.HasPrefix(t, " ") {
		t = t[1:]
	}
	return t, true
}
