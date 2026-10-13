// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"regexp"
	"strings"
)

// jsImportDone matches the end of an import statement written without a
// semicolon: `… from "x"` or a side-effect `import "x"`.
var jsImportDone = regexp.MustCompile(`(?s)(\bfrom\s*|^import\s*)["'][^"']*["']\s*$`)

// JavaScriptRetainedSections reads the two parts of an existing JavaScript
// action source that MDL cannot express but Studio Pro's regeneration keeps —
// its banner lists "the import list" and "the code between BEGIN EXTRA CODE and
// END EXTRA CODE" next to the user code, which the statement always supplies.
//
// Imports are returned as whole statements, a multi-line `import { a, b }
// from "x";` as one entry with its line breaks. The import list is what stands
// before the EXTRA CODE section (or, without one, before the function). Marker
// matching is case-insensitive, as for Java actions. existing is "" when there
// is no source file yet, which yields neither.
//
// Before this, a `create or modify` with an edited body regenerated the file
// from the statement alone, deleting a hand-restored helper and the imports the
// user code relies on (mendixlabs/mxcli#1429).
func JavaScriptRetainedSections(existing string) (imports []string, extraCode string) {
	existing = strings.ReplaceAll(existing, "\r\n", "\n")
	seen := map[string]bool{}
	var cur []string
	for line := range strings.SplitSeq(existing, "\n") {
		t := strings.TrimSpace(line)
		if cur == nil {
			if strings.Contains(asciiLower(t), "// begin extra code") || strings.HasPrefix(t, "export ") ||
				strings.HasPrefix(t, "function ") || strings.HasPrefix(t, "async function ") || strings.HasPrefix(t, "/**") {
				break // the import list ends where the code begins
			}
			if !strings.HasPrefix(t, "import ") && !strings.HasPrefix(t, "import{") && !strings.HasPrefix(t, `import"`) {
				continue
			}
		}
		cur = append(cur, strings.TrimRight(line, " \t"))
		stmt := strings.TrimSpace(strings.Join(cur, "\n"))
		if strings.HasSuffix(t, ";") || jsImportDone.MatchString(stmt) {
			if !seen[stmt] {
				seen[stmt] = true
				imports = append(imports, stmt)
			}
			cur = nil
		}
	}
	if ec, ok := sliceBetweenFold(existing, "// BEGIN EXTRA CODE", "// END EXTRA CODE"); ok {
		lines := strings.Split(ec, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		extraCode = strings.Join(lines, "\n")
	}
	return imports, extraCode
}
