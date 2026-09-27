// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/formatter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// examplesDir is the corpus the upgrade is proven on. The execute-both half of
// the proof (same model written) is mdl/roundtrip's upgrade property test; this
// is the half that needs no project: every script upgrades to one that parses,
// records no deprecation, builds the same statements, and is a fixed point of
// `fmt --upgrade`.
const examplesDir = "../../mdl-examples"

func exampleScripts(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(examplesDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".mdl") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	if len(out) < 100 {
		t.Fatalf("found only %d scripts under %s — the walk is broken", len(out), examplesDir)
	}
	return out
}

// fmtUpgrade is what `mxcli fmt --upgrade` writes: the upgrade rewrites and
// nothing else. It deliberately does not run formatter.Format, whose keyword
// upper-casing changes identifiers (see TestFormatterChangesMeaning).
func fmtUpgrade(src string, opts Options) (string, Result, error) {
	res, err := Upgrade(src, opts)
	return res.Source, res, err
}

// TestFormatterChangesMeaning records why `fmt --upgrade` does not compose with
// the heuristic formatter: formatter.Format upper-cases every word on its
// keyword list, identifiers included (`Issue64.User` becomes `Issue64.USER`),
// so it changes what a script builds. Plan item 3.2 (lowercase canonical)
// replaces it. When this test starts failing, the formatter is safe and
// fmt --upgrade may format as well.
func TestFormatterChangesMeaning(t *testing.T) {
	changed := 0
	for _, path := range exampleScripts(t) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want, errs := visitor.Build(string(b))
		if len(errs) > 0 {
			continue
		}
		got, errs := visitor.Build(formatter.Format(string(b)))
		if len(errs) > 0 || !reflect.DeepEqual(want.Statements, got.Statements) {
			changed++
		}
	}
	t.Logf("formatter.Format changes what %d example scripts build", changed)
	if changed == 0 {
		t.Error("formatter.Format no longer changes any example's statements: let fmt --upgrade format too, and drop this test")
	}
}

// foldModeFlags erases the one AST difference between `or replace` and
// `or modify` that means nothing: pages, snippets and layouts keep both flags,
// and every reader in the executor tests `IsModify || IsReplace` (pinned by
// mdl/visitor's TestCreateOrReplaceMatchesModifyExceptExemptKinds).
func foldModeFlags(stmts []ast.Statement) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *ast.CreatePageStmtV3:
			n.IsModify, n.IsReplace = n.IsModify || n.IsReplace, false
		case *ast.CreateSnippetStmtV3:
			n.IsModify, n.IsReplace = n.IsModify || n.IsReplace, false
		case *ast.CreateLayoutStmt:
			n.IsModify, n.IsReplace = n.IsModify || n.IsReplace, false
		}
	}
}

func TestUpgrade_ExamplesKeepTheirStatements(t *testing.T) {
	var parsed, upgraded, skipped int
	for _, path := range exampleScripts(t) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if _, errs := visitor.Build(src); len(errs) > 0 {
			skipped++ // a negative test, or a script that no longer parses: not upgradable
			continue
		}
		parsed++
		for _, opts := range []Options{{}, {AddHeader: true}} {
			out, res, err := fmtUpgrade(src, opts)
			if err != nil {
				t.Errorf("%s (%+v): %v", path, opts, err)
				continue
			}
			if res.Changed() && !opts.AddHeader {
				upgraded++
			}
			got, errs := visitor.Build(out)
			if len(errs) > 0 {
				t.Errorf("%s (%+v): upgraded output does not parse: %v", path, opts, errs[0])
				continue
			}
			if len(got.Deprecations) > 0 {
				t.Errorf("%s (%+v): upgraded output still records %d deprecation(s), first %s at line %d",
					path, opts, len(got.Deprecations), got.Deprecations[0].Code, got.Deprecations[0].Line)
			}
			want, _ := visitor.Build(src)
			foldModeFlags(want.Statements)
			foldModeFlags(got.Statements)
			if len(want.Statements) != len(got.Statements) {
				t.Errorf("%s (%+v): %d statements became %d", path, opts, len(want.Statements), len(got.Statements))
				continue
			}
			for i := range want.Statements {
				if !reflect.DeepEqual(want.Statements[i], got.Statements[i]) {
					t.Errorf("%s (%+v): statement %d builds differently after the upgrade:\n before: %#v\n after:  %#v",
						path, opts, i+1, want.Statements[i], got.Statements[i])
					break
				}
			}
			// Idempotent: fmt --upgrade over its own output changes nothing.
			again, res2, err := fmtUpgrade(out, opts)
			if err != nil || again != out || res2.Changed() {
				t.Errorf("%s (%+v): fmt --upgrade is not idempotent (err=%v)", path, opts, err)
			}
		}
	}
	t.Logf("%d scripts parse, %d of them upgraded, %d do not parse and are out of scope", parsed, upgraded, skipped)
	if upgraded == 0 {
		t.Error("no example needed an upgrade — the corpus no longer exercises the rewrites")
	}
}
