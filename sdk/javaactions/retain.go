// SPDX-License-Identifier: Apache-2.0

package javaactions

import "strings"

// generatedImports are the imports GenerateSource writes into every action;
// RetainedSections leaves them out of what it reports.
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
	for _, imp := range importList(source) {
		if !generatedImports[imp] {
			imports = append(imports, imp)
		}
	}
	if ec, ok := sliceBetweenFold(source, "// BEGIN EXTRA CODE", "// END EXTRA CODE"); ok {
		extraCode = trimSection(ec)
	}
	return imports, extraCode
}

// importList is a source's import list as it stands: every import line before
// the class, in order.
func importList(source string) []string {
	var imports []string
	for line := range strings.SplitSeq(source, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "public class ") {
			break // the import list ends where the class begins
		}
		if strings.HasPrefix(t, "import ") && strings.HasSuffix(t, ";") {
			imports = append(imports, t)
		}
	}
	return imports
}

// RetainSections merges what a statement supplies with what the existing source
// retains, the way mxbuild regenerates an action: the stored import list is kept
// as it stands — order included, and the imports the generator needs wherever
// they sit — with the statement's new imports added after it, and the stored
// extra code is kept unless the statement supplies its own. existing is "" when
// there is no source file yet. GenerateSource appends whatever the generated
// code needs that the list still lacks.
//
// Before this, a rewrite kept only the user code, so an action whose user code
// calls a helper in its EXTRA CODE section — FeedbackModule.XSS_Sanitizer in the
// Blank template — was regenerated into Java that does not compile
// (ako/mxcli#705).
func RetainSections(existing string, imports []string, extraCode string) ([]string, string) {
	kept := importList(existing)
	_, keptExtra := RetainedSections(existing)
	seen := make(map[string]bool, len(kept)+len(imports))
	var out []string
	for _, imp := range append(kept, imports...) {
		imp = strings.TrimSpace(imp)
		if imp == "" || seen[imp] {
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
//
// Only ASCII letters are folded. strings.ToLower changes the byte length of some
// characters ("İ" 2 -> 3 bytes, the Kelvin sign 3 -> 1), so an index into its
// result is not an index into s, and one such character above the markers cut
// the section a byte off — the markers are ASCII, so nothing else needs folding.
func sliceBetweenFold(s, begin, end string) (string, bool) {
	lower := asciiLower(s)
	bi := strings.Index(lower, asciiLower(begin))
	if bi == -1 {
		return "", false
	}
	rest := bi + len(begin)
	ei := strings.Index(lower[rest:], asciiLower(end))
	if ei == -1 {
		return "", false
	}
	return s[rest : rest+ei], true
}

// asciiLower lower-cases ASCII letters only, so the result has s's byte offsets.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
