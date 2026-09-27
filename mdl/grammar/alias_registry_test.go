// SPDX-License-Identifier: Apache-2.0

package grammar

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// aliasMarker is how a grammar token or alternative is marked as a deprecated
// alias: a block comment naming its registry code, e.g.
//
//	CREATE (OR (MODIFY | REPLACE /* @alias MDL-DEPR001 */))?
//
// ANTLR drops the comment; this test is its only reader.
var aliasMarker = regexp.MustCompile(`@alias\b\s*(\S*)`)

var deprCode = regexp.MustCompile(`^MDL-DEPR\d{3}$`)

// checkAliasMarkers returns one problem per marker that is malformed or names a
// code with no registry entry, and per registry entry that no marker names.
// sources maps a file name to its grammar text.
func checkAliasMarkers(sources map[string]string, entries []deprecation.Entry) []string {
	registered := map[string]bool{}
	for _, e := range entries {
		registered[e.Code] = true
	}
	marked := map[string]bool{}
	var problems []string

	names := make([]string, 0, len(sources))
	for n := range sources {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		for i, line := range strings.Split(sources[name], "\n") {
			for _, m := range aliasMarker.FindAllStringSubmatch(line, -1) {
				code := strings.TrimSuffix(m[1], "*/")
				switch {
				case !deprCode.MatchString(code):
					problems = append(problems, fmt.Sprintf(
						"%s:%d: malformed alias marker %q — write /* @alias MDL-DEPRnnn */", name, i+1, m[0]))
				case !registered[code]:
					problems = append(problems, fmt.Sprintf(
						"%s:%d: grammar alias %s has no entry in mdl/deprecation — add one "+
							"(old form, canonical form, rewrite, removed-in version, examples)", name, i+1, code))
				default:
					marked[code] = true
				}
			}
		}
	}
	for _, e := range entries {
		if !marked[e.Code] {
			problems = append(problems, fmt.Sprintf(
				"registry entry %s (%s) is named by no /* @alias %s */ marker in the grammar — "+
					"mark the alias where it is parsed", e.Code, e.Old, e.Code))
		}
	}
	return problems
}

func readGrammarSources(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.g4")
	if err != nil {
		t.Fatal(err)
	}
	domain, err := filepath.Glob("domains/*.g4")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, domain...)
	if len(files) == 0 {
		t.Fatal("no .g4 files found")
	}
	sources := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		sources[f] = string(b)
	}
	return sources
}

// An alias in the grammar must have a registry entry, so that it warns and
// `fmt --upgrade` can rewrite it (ADR-0011, decision 1) — and an entry must
// point at a real alias.
func TestGrammarAliasesAreRegistered(t *testing.T) {
	for _, p := range checkAliasMarkers(readGrammarSources(t), deprecation.All()) {
		t.Error(p)
	}
}

// The control: the checker does fail on an alias with no entry, on an entry
// with no alias, and on a malformed marker. Without this, a checker that found
// nothing would pass the test above against any grammar.
func TestAliasMarkerCheckerDetectsProblems(t *testing.T) {
	entries := []deprecation.Entry{{Code: "MDL-DEPR001", Old: "a"}, {Code: "MDL-DEPR002", Old: "b"}}
	grammar := map[string]string{"X.g4": strings.Join([]string{
		"r1 : A /* @alias MDL-DEPR001 */ | B ;",
		"r2 : C /* @alias MDL-DEPR999 */ | D ;",
		"r3 : E /* @alias DEPR3 */ ;",
	}, "\n")}
	problems := checkAliasMarkers(grammar, entries)
	want := []string{"MDL-DEPR999 has no entry", "malformed alias marker", "MDL-DEPR002 (b) is named by no"}
	for _, w := range want {
		found := false
		for _, p := range problems {
			if strings.Contains(p, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("checker did not report %q; got %q", w, problems)
		}
	}
	if len(problems) != len(want) {
		t.Errorf("got %d problems, want %d: %q", len(problems), len(want), problems)
	}
}
