// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// sameStatements requires two scripts to build the same statements, with the
// folds TestUpgrade_ExamplesKeepTheirStatements applies.
func sameStatements(t *testing.T, before, after string) {
	t.Helper()
	a, errs := visitor.Build(before)
	if len(errs) > 0 {
		t.Fatalf("original does not parse: %v", errs[0])
	}
	b, errs := visitor.Build(after)
	if len(errs) > 0 {
		t.Fatalf("upgrade does not parse: %v\n%s", errs[0], after)
	}
	foldModeFlags(a.Statements)
	foldModeFlags(b.Statements)
	foldSourceText(reflect.ValueOf(a.Statements))
	foldSourceText(reflect.ValueOf(b.Statements))
	if !reflect.DeepEqual(a.Statements, b.Statements) {
		t.Errorf("the upgrade builds different statements:\n before: %#v\n after:  %#v", a.Statements, b.Statements)
	}
}

const mf = "create microflow M.F ($L: List of M.E, $S: String, $o: M.E) begin\n"

// Each header-gated construct, rewritten so that under `mdl 1;` it means what
// it meant under mdl 0. The execute-both test in mdl/roundtrip proves the same
// on the example corpus against a real project.
func TestUpgrade_GatedRewrites(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		sameAST         bool // false where the AST differs but the executor writes the same (see buildsTheSameModelNotTheSameAST)
	}{
		{"missing semicolon", "create module M\ncreate module N -- note\n",
			"create module M;\ncreate module N; -- note\n", true},
		{"slash line", "create module M;\n/\ncreate module N;\n  /  \n",
			"create module M;\ncreate module N;\n", true},
		{"slash and no semicolon", "create module M\n/\n", "create module M;\n", true},
		{"slash on the statement's line", "create module M; /\n", "create module M; \n", true},
		{"flow ending in a slash", mf + "  log info 'x';\nend;\n/\n", mf + "  log info 'x';\nend;\n", true},
		{"escapes in a plain literal", "create enumeration M.C (A 'it\\'s\\ta\\\\b');\n",
			"create enumeration M.C (A 'it''s\ta\\b');\n", true},
		{"escaped line break outside an expression", "create enumeration M.C (A 'a\\nb');\n",
			"create enumeration M.C (A 'a\nb');\n", true},
		{"escape in an expression stored as written", mf + "  declare $s String = 'a' +\n    '\\n';\nend;\n",
			mf + "  declare $s String = 'a' +\n    '\\n';\nend;\n", true},
		{"escaped line break in a log template", mf + "  log info node 'N' 'line 1\\nline 2';\nend;\n",
			mf + "  log info node 'N' 'line 1\nline 2';\nend;\n", true},
		{"escaped line break in a message template", mf + "  show message 'it\\'s\\n{1}' type Error with ({1} = $S);\nend;\n",
			mf + "  show message 'it''s\n{1}' type Error with ({1} = $S);\nend;\n", true},
		{"a log template over two lines is a parameter", mf + "  log info 'a\n  b';\nend;\n",
			mf + "  log info '{1}' with ({1} = 'a\n  b');\nend;\n", false},
		{"a message template over two lines is a parameter", mf + "  SHOW MESSAGE 'a\nb' TYPE Error BLOCKING;\nend;\n",
			mf + "  SHOW MESSAGE '{1}' TYPE Error WITH ({1} = 'a\nb') BLOCKING;\nend;\n", false},
		{"escape in a log template over two lines, with parameters", mf + "  log info 'x\\ty\nz {1}' with ({1} = $S);\nend;\n",
			mf + "  log info 'x\ty\nz {1}' with ({1} = $S);\nend;\n", true},
		{"escape in a message template over two lines, with parameters", mf + "  show message 'it\\'s\\\\\n{1}' type Error with ({1} = $S);\nend;\n",
			mf + "  show message 'it''s\\\n{1}' type Error with ({1} = $S);\nend;\n", true},
		// An escaped line break in an expression the builder re-renders: the
		// line break goes into the literal, and the expression becomes the one
		// mdl 0 stored, since under mdl 1 a source over lines is stored as
		// written (ako/mxcli#804). sameAST is false: mdl 1 keeps the source.
		{"escaped line break in an expression", mf + "  declare $s String = 'a\\nb';\nend;\n",
			mf + "  declare $s String = 'a\nb';\nend;\n", false},
		{"escaped line break in a compound expression is written as stored",
			mf + "  declare $s String = 'it''s\\r\\n' + TOSTRING( $S ) + 'C:\\\\new';\nend;\n",
			mf + "  declare $s String = 'it''s\r\n' + toString($S) + 'C:\\\\new';\nend;\n", false},
		{"escaped line break in a change member", mf + "  change $o (Name = '{\\n  1\\n}', Code = 'x');\nend;\n",
			mf + "  change $o (Name = '{\n  1\n}', Code = 'x');\nend;\n", false},
		{"escaped tab in an expression", mf + "  declare $s String = 'a\\tb';\nend;\n",
			mf + "  declare $s String = 'a\tb';\nend;\n", true},
		{"limit 1 is an object", mf + "  retrieve $X from M.E where Name = 'a' limit 1;\nend;\n",
			mf + "  retrieve $X from M.E where Name = 'a' first;\nend;\n", true},
		{"LIMIT 1 in upper case", mf + "  RETRIEVE $X FROM M.E LIMIT 1;\nend;\n",
			mf + "  RETRIEVE $X FROM M.E FIRST;\nend;\n", true},
		{"reassignment says set", mf + "  declare $n Integer = 1;\n  $n = 2;\nend;\n",
			mf + "  declare $n Integer = 1;\n  set $n = 2;\nend;\n", true},
		{"set with a list call", mf + "  set $T = head($L);\nend;\n", mf + "  $T = head $L;\nend;\n", true},
		{"set with an aggregate", mf + "  set $N = sum($L.Amount);\nend;\n", mf + "  $N = sum $L by Amount;\nend;\n", true},
		{"find on a list, by member", mf + "  $F = find($L, Name = 'x');\nend;\n", mf + "  $F = find $L by Name = 'x';\nend;\n", true},
		{"find on a list, by expression", mf + "  $F = FIND($L, $currentObject/Name = 'x');\nend;\n",
			mf + "  $F = FIND $L WHERE $currentObject/Name = 'x';\nend;\n", true},
		{"contains on a list", mf + "  $b = contains($L, $o);\nend;\n", mf + "  $b = contains $o in $L;\nend;\n", true},
		{"find on a retrieved list", mf + "  retrieve $R from M.E;\n  $F = find($R, Name = 'x');\nend;\n",
			mf + "  retrieve $R from M.E;\n  $F = find $R by Name = 'x';\nend;\n", true},
		{"find on a String is the string function", mf + "  $i = find($S, 'x');\nend;\n",
			mf + "  set $i = find($S, 'x');\nend;\n", false},
		{"set contains on a String", mf + "  declare $b Boolean = false;\n  set $b = contains($S, $S);\nend;\n",
			mf + "  declare $b Boolean = false;\n  set $b = contains($S, $S);\nend;\n", false},
		{"create or replace user role is a plain create", "create or replace user role R ( ModuleRoles: (M.Admin) );\n",
			"create user role R ( ModuleRoles: (M.Admin) );\n", true},
		{"while without begin", mf + "  while $S = 'a'\n    log info 'x';\n  end while;\nend;\n",
			mf + "  while $S = 'a' begin\n    log info 'x';\n  end while;\nend;\n", true},
		{"while ending in a bare end", mf + "  while $S = 'a' begin\n    log info 'x';\n  end; -- note\nend;\n",
			mf + "  while $S = 'a' begin\n    log info 'x';\n  end while; -- note\nend;\n", true},
		{"WHILE without either, upper case", mf + "  WHILE true\n    LOG INFO 'x';\n  END;\nend;\n",
			mf + "  WHILE true BEGIN\n    LOG INFO 'x';\n  END WHILE;\nend;\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, err := Upgrade(c.src, Options{AddHeader: true})
			if err != nil {
				t.Fatal(err)
			}
			if want := "mdl 1;\n" + c.want; res.Source != want {
				t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
			}
			if len(res.GatedRewritten) == 0 {
				t.Errorf("no gated rewrite recorded")
			}
			if c.sameAST {
				sameStatements(t, c.src, res.Source)
			}
			again, err := Upgrade(res.Source, Options{AddHeader: true})
			if err != nil || again.Source != res.Source || again.Changed() {
				t.Errorf("not idempotent: %v %q", err, again.Source)
			}
		})
	}
}

// A construct with no mechanical rewrite is reported with its reason and
// blocks the header; nothing is written.
func TestUpgrade_UnrewritableBlocksTheHeader(t *testing.T) {
	for _, c := range []struct{ name, src, code, reason string }{
		{"nested list operation", mf + "  $n = count(filter($L, Name = 'x'));\nend;\n", "MDL-V1-LIST", "nested call"},
		{"find on a call's result", mf + "  $R = call microflow M.G();\n  $F = find($R, Name = 'x');\nend;\n",
			"MDL-V1-LIST", "does not state"},
		{"view entity replace", "create or replace view entity M.V (Name: String(100)) as (select c.Name as Name from M.Customer as c);\n",
			"MDL-V1-REPLACE01", "drops and recreates"},
		{"unknown property", "create rest client M.Api (BaseUrl: 'https://x', Authentication: none) " +
			"{ operation GetUser { Method: get, pathh: '/users', Response: none } };\n", "MDL-V1-PROP", "cannot be guessed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, err := Upgrade(c.src, Options{AddHeader: true})
			var hb *HeaderBlockedError
			if !errors.As(err, &hb) {
				t.Fatalf("header added over a construct with no rewrite: %v\n%s", err, res.Source)
			}
			found := false
			for _, b := range hb.Constructs {
				if b.Code == c.code && strings.Contains(b.Reason, c.reason) {
					found = true
				}
			}
			if !found {
				t.Errorf("want %s (%q) reported, got %+v", c.code, c.reason, hb.Constructs)
			}
			// Without the header the script keeps its meaning, so it upgrades.
			if _, err := Upgrade(c.src, Options{}); err != nil {
				t.Errorf("without the header: %v", err)
			}
		})
	}
}

// The call form of every list operation and aggregate becomes its statement
// form without the header (MDL-DEPR003/004 are aliases under every version),
// building the same activity.
func TestUpgrade_ListCallFormToStatementForm(t *testing.T) {
	for _, c := range [][2]string{
		{"$X = head($L);", "$X = head $L;"},
		{"$X = TAIL( $L );", "$X = TAIL $L;"},
		{"$X = filter($L, Name = 'x');", "$X = filter $L by Name = 'x';"},
		{"$X = filter($L, Qty > 0 and Name != 'a');", "$X = filter $L where Qty > 0 and Name != 'a';"},
		{"$X = sort($L, \"Status\" asc, Name desc);", "$X = sort $L by \"Status\" asc, Name desc;"},
		{"$X = union($L, $L2);", "$X = union $L with $L2;"},
		{"$X = intersect($L, $L2);", "$X = intersect $L with $L2;"},
		{"$X = subtract($L, $L2);", "$X = subtract $L2 from $L;"},
		{"$X = equals($L, $L2);", "$X = equals $L and $L2;"},
		{"$X = range($L, 5);", "$X = range $L offset 5;"},
		{"$X = range($L, $o2, 10);", "$X = range $L offset $o2 limit 10;"},
		{"$X = count($L);", "$X = count $L;"},
		{"$X = sum($L.Amount);", "$X = sum $L by Amount;"},
		{"$X = average($L/Amount);", "$X = average $L by Amount;"},
		{"$X = minimum($L, $currentObject/Amount * 2);", "$X = minimum $L of $currentObject/Amount * 2;"},
		{"$X = MAXIMUM($L.Amount);", "$X = MAXIMUM $L BY Amount;"},
		{"$X = all($L, $currentObject/Paid);", "$X = all $L where $currentObject/Paid;"},
		{"$X = any($L, not($currentObject/Paid));", "$X = any $L where not($currentObject/Paid);"},
		{"$X = reduce($L, $currentResult + $currentObject/Amount, initial: 0, returns: Decimal);",
			"$X = reduce $L from 0 as Decimal using $currentResult + $currentObject/Amount;"},
	} {
		src := "create microflow M.F ($L: List of M.E, $L2: List of M.E, $o2: Integer) begin\n  " + c[0] + "\nend;\n"
		want := "create microflow M.F ($L: List of M.E, $L2: List of M.E, $o2: Integer) begin\n  " + c[1] + "\nend;\n"
		t.Run(c[0], func(t *testing.T) {
			res, err := Upgrade(src, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if res.Source != want {
				t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
			}
			sameStatements(t, src, res.Source)
		})
	}
}

// Every change of meaning the visitor gates has a rewrite or is listed as
// having none, never both. The list of changes with none may only shrink.
func TestGatedRegistryIsComplete(t *testing.T) {
	// MDL-V1-SESSION (R7, ako/mxcli#755) was added with its change: moving a
	// session command out of a script is a decision about how the script is run.
	pinned := map[string]bool{"MDL-V1-PROP": true, "MDL-V1-PROPVALUE": true, "MDL-V1-REPLACE01": true,
		"MDL-V1-SESSION": true,
		// MDL-V1-SHOWSUMMARY (R6, ako/mxcli#755) was added with its change:
		// the summary is removed, not respelt, so the choice of replacement
		// is the author's.
		"MDL-V1-SHOWSUMMARY": true}
	known := map[string]bool{}
	for _, c := range visitor.LanguageChanges() {
		known[c.Code] = true
		_, rw := gatedRewriters[c.Code]
		_, none := unrewritable[c.Code]
		if rw == none {
			t.Errorf("%s: needs exactly one of a rewrite in gatedRewriters or a reason in unrewritable (rewrite=%v, reason=%v)",
				c.Code, rw, none)
		}
	}
	for code := range gatedRewriters {
		if !known[code] {
			t.Errorf("gatedRewriters has %s, which the visitor does not gate", code)
		}
	}
	for code := range unrewritable {
		if !pinned[code] {
			t.Errorf("%s was added to unrewritable: the list may only shrink — give the change a rewrite", code)
		}
	}
}
