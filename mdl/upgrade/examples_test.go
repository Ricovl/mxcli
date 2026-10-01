// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
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
// nothing else. It does not run formatter.Format, whose line normalisation is
// a separate choice (`fmt` without --upgrade).
func fmtUpgrade(src string, opts Options) (string, Result, error) {
	res, err := Upgrade(src, opts)
	return res.Source, res, err
}

// TestFormatterKeepsMeaning: formatter.Format must build exactly what the
// script built. It used to upper-case every word on a keyword list, names
// included (`Issue64.User` became `Issue64.USER`), and changed what 50-odd
// example scripts built; lowercase-canonical keywords from the parse tree
// (R8, ako/mxcli#752) change only the case of syntax.
func TestFormatterKeepsMeaning(t *testing.T) {
	checked := 0
	for _, path := range exampleScripts(t) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want, errs := visitor.Build(string(b))
		if len(errs) > 0 {
			continue
		}
		checked++
		formatted := formatter.Format(string(b))
		got, errs := visitor.Build(formatted)
		if len(errs) > 0 {
			t.Errorf("%s: formatted script does not parse: %v", path, errs[0])
			continue
		}
		if !reflect.DeepEqual(want.Statements, got.Statements) {
			t.Errorf("%s: formatter.Format changes what the script builds", path)
			continue
		}
		if again := formatter.Format(formatted); again != formatted {
			t.Errorf("%s: formatter.Format is not idempotent", path)
		}
	}
	if checked < 100 {
		t.Fatalf("checked only %d scripts", checked)
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

// keepsItsVersion lists the example scripts `fmt --upgrade --header` refuses,
// with the codes of the constructs that block it: each has no mechanical
// rewrite (HeaderBlockedError says why), so the script keeps mdl 0. The list
// may only shrink — a script that starts upgrading must be removed, and one
// that stops upgrading is a regression.
var keepsItsVersion = map[string][]string{
	// Nested list operations (`count(filter(…))`) and a non-variable operand:
	// one activity takes a variable, so the inner call must become a statement
	// of its own, which needs a variable name nobody chose.
	"bug-tests/1101-nested-list-operand-dropped.fail.mdl": {"MDL-V1-LIST"},
	// Scripts that exercise session commands — `help <topic>` and `lint` —
	// which are REPL commands under mdl 1 (R7, ako/mxcli#755). They test the
	// commands, so they stay mdl 0 scripts rather than lose what they test.
	"bug-tests/904-lint-rules-discovery.mdl":    {"MDL-V1-SESSION"},
	"bug-tests/syntax-1025-topic-drilldown.mdl": {"MDL-V1-SESSION"},
	"doctype-tests/20-help-examples.mdl":        {"MDL-V1-SESSION"},
}

// buildsTheSameModelNotTheSameAST lists the example scripts whose upgrade
// builds a different AST that the executor writes identically, so the AST
// comparison is skipped for them; the execute-both test in mdl/roundtrip is
// the proof. May only shrink.
var buildsTheSameModelNotTheSameAST = map[string]string{
	// `set $x = contains($Hay, $Needle)` / `find(…)` on a String: mdl 0 builds
	// a List operation statement, which the flow builder turns into a Change
	// variable with the string function because $Hay is declared String; under
	// mdl 1 it is that Change variable directly.
	//
	// The mdl-0 copies in the alias corpus: the originals were upgraded to
	// `mdl 1;` with the rest of mdl-examples.
	"deprecated-aliases/bug-tests--ledger-53-string-contains.mdl": "string contains: a List operation turned into the string function",
	"deprecated-aliases/bug-tests--ledger-63-string-find.mdl":     "string find: a List operation turned into the string function",
}

// foldSourceText erases the one part of an ast.SourceExpr the executor never
// writes: the parsed expression kept beside the source text for checks
// (expressionToString writes Source whenever it is set). A string escape in a
// multi-line expression changes that parsed value and nothing it writes.
func foldSourceText(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return
		}
		if se, ok := v.Interface().(*ast.SourceExpr); ok && se.Source != "" {
			se.Expression = nil
			return
		}
		foldSourceText(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				foldSourceText(v.Field(i))
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			foldSourceText(v.Index(i))
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			foldSourceText(v.MapIndex(k))
		}
	}
}

func TestUpgrade_ExamplesKeepTheirStatements(t *testing.T) {
	var parsed, upgraded, skipped int
	blocked := map[string]bool{}
	for _, path := range exampleScripts(t) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, examplesDir+"/"))
		src := string(b)
		if _, errs := visitor.Build(src); len(errs) > 0 {
			skipped++ // a negative test, or a script that no longer parses: not upgradable
			continue
		}
		parsed++
		for _, opts := range []Options{{}, {AddHeader: true}} {
			out, res, err := fmtUpgrade(src, opts)
			var hb *HeaderBlockedError
			if opts.AddHeader && errors.As(err, &hb) {
				blocked[rel] = true
				allowed := map[string]bool{}
				for _, c := range keepsItsVersion[rel] {
					allowed[c] = true
				}
				for _, c := range hb.Constructs {
					if !allowed[c.Code] {
						t.Errorf("%s: the header is refused over %s at line %d (%s), which is not listed in keepsItsVersion",
							path, c.Code, c.Line, c.Reason)
					}
				}
				continue
			}
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
			// A use the upgrade reported as unrewritable (Result.Unrewritten) stays,
			// by contract: it is reported, never guessed at. Anything beyond those
			// is a rewrite that did not produce the canonical form.
			if len(got.Deprecations) > len(res.Unrewritten) {
				t.Errorf("%s (%+v): upgraded output still records %d deprecation(s) (%d reported unrewritable), first %s at line %d",
					path, opts, len(got.Deprecations), len(res.Unrewritten), got.Deprecations[0].Code, got.Deprecations[0].Line)
			}
			want, _ := visitor.Build(src)
			foldModeFlags(want.Statements)
			foldModeFlags(got.Statements)
			foldSourceText(reflect.ValueOf(want.Statements))
			foldSourceText(reflect.ValueOf(got.Statements))
			if len(want.Statements) != len(got.Statements) {
				t.Errorf("%s (%+v): %d statements became %d", path, opts, len(want.Statements), len(got.Statements))
				continue
			}
			differs := false
			for i := range want.Statements {
				if !reflect.DeepEqual(want.Statements[i], got.Statements[i]) {
					differs = true
					if _, known := buildsTheSameModelNotTheSameAST[rel]; !known {
						t.Errorf("%s (%+v): statement %d builds differently after the upgrade:\n before: %#v\n after:  %#v",
							path, opts, i+1, want.Statements[i], got.Statements[i])
					}
					break
				}
			}
			if _, known := buildsTheSameModelNotTheSameAST[rel]; known && opts.AddHeader && !differs {
				t.Errorf("%s now builds the same AST after the upgrade: remove it from buildsTheSameModelNotTheSameAST", rel)
			}
			// Idempotent: fmt --upgrade over its own output changes nothing.
			again, res2, err := fmtUpgrade(out, opts)
			if err != nil || again != out || res2.Changed() {
				t.Errorf("%s (%+v): fmt --upgrade is not idempotent (err=%v)", path, opts, err)
			}
		}
	}
	for rel := range keepsItsVersion {
		if !blocked[rel] {
			t.Errorf("%s now takes the header: remove it from keepsItsVersion", rel)
		}
	}
	t.Logf("%d scripts parse, %d of them upgraded, %d do not parse and are out of scope, %d keep mdl 0",
		parsed, upgraded, skipped, len(blocked))
	if upgraded == 0 {
		t.Error("no example needed an upgrade — the corpus no longer exercises the rewrites")
	}
}
