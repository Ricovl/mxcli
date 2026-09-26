// SPDX-License-Identifier: Apache-2.0

package javaactions

import "strings"

// generatedImports are the imports GenerateSource writes itself; they are not
// part of what a regeneration has to retain.
var generatedImports = map[string]bool{
	"import com.mendix.systemwideinterfaces.core.IContext;":   true,
	"import com.mendix.systemwideinterfaces.core.UserAction;": true,
}

// RetainedSections reads the two parts of an existing Java action source that
// MDL cannot express but Studio Pro's regeneration keeps — its banner says so:
// "Only the following code will be retained when actions are regenerated: the
// import list, … the code between BEGIN EXTRA CODE and END EXTRA CODE". The
// user code is the third, and the statement always supplies that.
//
// Imports are returned as their full lines, minus the two GenerateSource writes
// itself. Marker matching is case-insensitive, as for JavaScript actions.
func RetainedSections(source string) (imports []string, extraCode string) {
	for line := range strings.SplitSeq(source, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "public class ") {
			break // the import list ends where the class begins
		}
		if strings.HasPrefix(t, "import ") && strings.HasSuffix(t, ";") && !generatedImports[t] {
			imports = append(imports, t)
		}
	}
	if ec, ok := sliceBetweenFold(source, "// BEGIN EXTRA CODE", "// END EXTRA CODE"); ok {
		extraCode = trimSection(ec)
	}
	return imports, extraCode
}

// RetainSections merges what a statement supplies with what the existing source
// retains, the way Studio Pro regenerates an action: the stored import list is
// kept and the statement's imports are added to it, and the stored extra code is
// kept unless the statement supplies its own. existing is "" when there is no
// source file yet.
//
// Before this, a rewrite kept only the user code, so an action whose user code
// calls a helper in its EXTRA CODE section — FeedbackModule.XSS_Sanitizer in the
// Blank template — was regenerated into Java that does not compile
// (ako/mxcli#705).
func RetainSections(existing string, imports []string, extraCode string) ([]string, string) {
	kept, keptExtra := RetainedSections(existing)
	seen := make(map[string]bool, len(kept)+len(imports))
	var out []string
	for _, imp := range append(kept, imports...) {
		imp = strings.TrimSpace(imp)
		if imp == "" || seen[imp] || generatedImports[imp] {
			continue
		}
		seen[imp] = true
		out = append(out, imp)
	}
	if extraCode == "" {
		extraCode = keptExtra
	}
	return out, extraCode
}

// trimSection drops the blank lines around a marker section and the one tab of
// indentation GenerateSource adds back to each line.
func trimSection(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "\t")
	}
	return strings.Join(lines, "\n")
}

// sliceBetweenFold returns the substring of s between the first case-insensitive
// occurrence of begin and the following case-insensitive occurrence of end.
func sliceBetweenFold(s, begin, end string) (string, bool) {
	lower := strings.ToLower(s)
	bi := strings.Index(lower, strings.ToLower(begin))
	if bi == -1 {
		return "", false
	}
	rest := bi + len(begin)
	ei := strings.Index(lower[rest:], strings.ToLower(end))
	if ei == -1 {
		return "", false
	}
	return s[rest : rest+ei], true
}
