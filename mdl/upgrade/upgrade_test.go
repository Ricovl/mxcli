// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func mustUpgrade(t *testing.T, src string, opts Options) Result {
	t.Helper()
	res, err := Upgrade(src, opts)
	if err != nil {
		t.Fatalf("Upgrade(%q): %v", src, err)
	}
	return res
}

// Every registry entry's own example upgrades to exactly its canonical example.
// A new entry is covered the moment it is registered.
func TestUpgrade_EveryRegistryExample(t *testing.T) {
	for _, e := range deprecation.All() {
		t.Run(e.Code, func(t *testing.T) {
			res := mustUpgrade(t, e.Example, Options{})
			if res.Source != e.CanonicalExample {
				t.Errorf("upgrade of %q\n got: %q\nwant: %q", e.Example, res.Source, e.CanonicalExample)
			}
			if res.Rewritten[e.Code] != 1 {
				t.Errorf("Rewritten = %v, want one %s", res.Rewritten, e.Code)
			}
		})
	}
}

func TestUpgrade_RewritesInPlaceKeepingCaseCommentsAndLayout(t *testing.T) {
	src := "-- create or replace in a comment stays\n" +
		"CREATE OR REPLACE ENUMERATION M.Color (Red 'show me');   -- trailing\n" +
		"  Show entities in M; show   associations;\n" +
		"create or modify enumeration M.Size (S 's');\n" +
		"show entity M.Customer;\n"
	want := "-- create or replace in a comment stays\n" +
		"CREATE OR MODIFY ENUMERATION M.Color (Red 'show me');   -- trailing\n" +
		"  List entities in M; list   associations;\n" +
		"create or modify enumeration M.Size (S 's');\n" +
		// `show entity X` becomes `describe`, which is not registered yet: untouched.
		"show entity M.Customer;\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if res.Rewritten[deprecation.CreateOrReplace] != 1 || res.Rewritten[deprecation.Show] != 2 {
		t.Errorf("Rewritten = %v", res.Rewritten)
	}
}

// Non-ASCII text before the token on the same line: ANTLR columns count runes,
// and a byte-indexed rewrite would land in the wrong place.
func TestUpgrade_ColumnsAreRunes(t *testing.T) {
	src := "/* café ☕ */ create or replace enumeration M.Color (Red 'Rood ☕');"
	res := mustUpgrade(t, src, Options{})
	want := "/* café ☕ */ create or modify enumeration M.Color (Red 'Rood ☕');"
	if res.Source != want {
		t.Fatalf("got  %q\nwant %q", res.Source, want)
	}
}

// Where `or replace` does not mean `or modify` it is not an alias, is not
// recorded, and must not be rewritten.
func TestUpgrade_LeavesNonAliasesAlone(t *testing.T) {
	for _, src := range []string{
		"create or replace view entity M.V (Name: String(100)) as (select c.Name as Name from M.Customer as c);",
		"create or replace translations in Administration for nl_NL ('Save' as 'Opslaan');",
		"create or replace user role Clerk ( ModuleRoles: (M.User) );",
		"show version;",
	} {
		res := mustUpgrade(t, src, Options{})
		if res.Source != src || res.Changed() {
			t.Errorf("%q was rewritten to %q", src, res.Source)
		}
	}
}

func TestUpgrade_IsIdempotent(t *testing.T) {
	src := "create or replace enumeration M.Color (Red 'Red');\nshow entities;\n"
	for _, opts := range []Options{{}, {AddHeader: true}} {
		once := mustUpgrade(t, src, opts)
		twice := mustUpgrade(t, once.Source, opts)
		if twice.Source != once.Source || twice.Changed() {
			t.Errorf("opts %+v: second upgrade changed %q to %q", opts, once.Source, twice.Source)
		}
	}
}

func TestUpgrade_HeaderOnlyWhenAsked(t *testing.T) {
	src := "show entities;\n"
	if res := mustUpgrade(t, src, Options{}); res.HeaderAdded || strings.HasPrefix(res.Source, "mdl") {
		t.Fatalf("header added without being asked: %q", res.Source)
	}
	res := mustUpgrade(t, src, Options{AddHeader: true})
	if !res.HeaderAdded || res.Source != "mdl 1;\nlist entities;\n" {
		t.Fatalf("got %q (HeaderAdded=%v)", res.Source, res.HeaderAdded)
	}
	prog, errs := visitor.Build(res.Source)
	if len(errs) > 0 || prog.LanguageVersion != langver.Latest {
		t.Fatalf("upgraded script is not %s: %v", langver.Latest, errs)
	}
	// An existing header is kept, not doubled.
	again := mustUpgrade(t, res.Source, Options{AddHeader: true})
	if again.HeaderAdded || again.Source != res.Source {
		t.Fatalf("header added twice: %q", again.Source)
	}
}

// mdl 1 is frozen (ako/mxcli#714), so `fmt --upgrade` adds its header by
// default; DefaultOptions follows langver.Frozen, with no edit here.
func TestDefaultOptions_FollowsFrozen(t *testing.T) {
	if got, want := DefaultOptions().AddHeader, langver.Latest <= langver.Frozen; got != want {
		t.Fatalf("DefaultOptions().AddHeader = %v, want %v", got, want)
	}
	if !DefaultOptions().AddHeader {
		t.Fatal("mdl 1 is frozen: fmt --upgrade must add the header without --header")
	}
}

func TestUpgrade_RefusesUnparseableInput(t *testing.T) {
	if _, err := Upgrade("create or replace enumeration (", Options{}); err == nil {
		t.Fatal("upgraded a script that does not parse")
	}
}

// An entry without a rewrite is reported, not guessed at, and the use is left.
func TestUpgrade_EntryWithoutRewriteIsReported(t *testing.T) {
	u := upgrader{
		lookup: func(code string) (deprecation.Entry, bool) {
			e, ok := deprecation.Lookup(code)
			if code == deprecation.Show {
				e.Rewrite = deprecation.Rewrite{}
			}
			return e, ok
		},
		gated: map[string]GatedRewriter{},
	}
	src := "show entities;\ncreate or replace enumeration M.Color (Red 'Red');"
	res, err := u.upgrade(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Source, "show entities;\ncreate or modify") {
		t.Fatalf("got %q", res.Source)
	}
	if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.Show || res.Unrewritten[0].Line != 1 {
		t.Fatalf("Unrewritten = %+v, want the show on line 1", res.Unrewritten)
	}
}

// A rewrite that does not produce the canonical spelling is caught by the
// re-parse, before anything is returned.
func TestUpgrade_WrongRewriteIsCaught(t *testing.T) {
	u := upgrader{
		lookup: func(code string) (deprecation.Entry, bool) {
			e, ok := deprecation.Lookup(code)
			e.Rewrite.Replacement = e.Rewrite.Token // rewrites to itself
			return e, ok
		},
		gated: map[string]GatedRewriter{},
	}
	if _, err := u.upgrade("show entities;", Options{}); err == nil || !strings.Contains(err.Error(), "still uses") {
		t.Fatalf("a rewrite that changes nothing was accepted: %v", err)
	}
	u.lookup = func(code string) (deprecation.Entry, bool) {
		e, ok := deprecation.Lookup(code)
		e.Rewrite.Replacement = "frobnicate" // not a statement: ANTLR drops it silently
		return e, ok
	}
	if _, err := u.upgrade("show entities;", Options{}); err == nil || !strings.Contains(err.Error(), "rewrite is wrong") {
		t.Fatalf("a rewrite to unparseable text was accepted: %v", err)
	}
}

// The header-gated hook. A construct whose meaning the header would change
// must be rewritten before the header goes on; without a rewrite the header is
// refused. The visitor has no gated construct yet, so the test drives the hook
// through a Program the way Builder.gate would fill it.
func TestUpgrade_GatedConstructs(t *testing.T) {
	const code = "MDL-V1-TEST"
	src := "show entities;\nshow modules;\n"
	withNote := func(src string) (*ast.Program, error) {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			return nil, errs[0]
		}
		if prog.LanguageHeaderLine == 0 {
			prog.LanguageNotes = append(prog.LanguageNotes, ast.LanguageNote{Line: 2, Code: code})
		}
		return prog, nil
	}

	// No rewriter: the header is refused and nothing is returned.
	u := upgrader{lookup: deprecation.Lookup, gated: map[string]GatedRewriter{}}
	res, err := u.upgradeProg(src, Options{AddHeader: true}, withNote)
	if err == nil || !strings.Contains(err.Error(), code) {
		t.Fatalf("header added over a gated construct with no rewrite: %v %q", err, res.Source)
	}
	// Without the header nothing changes meaning, so the aliases still upgrade.
	res, err = u.upgradeProg(src, Options{}, withNote)
	if err != nil || res.Source != "list entities;\nlist modules;\n" {
		t.Fatalf("got %q, %v", res.Source, err)
	}

	// With a rewriter: its edits are applied along with the alias rewrites.
	u.gated = map[string]GatedRewriter{code: func(s *Source, _ *ast.Program, n ast.LanguageNote) ([]Edit, error) {
		got, _ := s.At(n.Line, 5, 7)
		if got != "modules" {
			t.Errorf("note does not point at the construct: %q", got)
		}
		off, _ := s.Offset(n.Line, 5)
		return []Edit{{Start: off, Stop: off + 7, Text: "microflows"}}, nil
	}}
	res, err = u.upgradeProg(src, Options{AddHeader: true}, withNote)
	if err != nil {
		t.Fatal(err)
	}
	if want := "mdl 1;\nlist entities;\nlist microflows;\n"; res.Source != want || res.GatedRewritten[code] != 1 {
		t.Fatalf("got %q (%v), want %q", res.Source, res.GatedRewritten, want)
	}
}

// A brace error handler (MDL-DEPR540) becomes `begin … end error`, nested ones
// included, keeping the body, comments and layout; it needs no header.
func TestUpgrade_OnErrorBracesToBeginEndError(t *testing.T) {
	for _, c := range [][2]string{
		{mf + "  commit $o on error {\n    log warning 'x'; -- keep\n  };\nend;\n",
			mf + "  commit $o on error begin\n    log warning 'x'; -- keep\n  end error;\nend;\n"},
		{mf + "  commit $o on error without rollback { };\nend;\n",
			mf + "  commit $o on error without rollback begin end error;\nend;\n"},
		{mf + "  COMMIT $o ON ERROR WITHOUT ROLLBACK {\n    COMMIT $o ON ERROR { LOG INFO 'inner'; };\n  };\nend;\n",
			mf + "  COMMIT $o ON ERROR WITHOUT ROLLBACK BEGIN\n    COMMIT $o ON ERROR BEGIN LOG INFO 'inner'; END ERROR;\n  END ERROR;\nend;\n"},
	} {
		res, err := Upgrade(c[0], Options{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Source != c[1] {
			t.Errorf("got:\n%s\nwant:\n%s", res.Source, c[1])
		}
		if res.Rewritten["MDL-DEPR540"] == 0 {
			t.Errorf("no MDL-DEPR540 rewrite recorded: %+v", res.Rewritten)
		}
		sameStatements(t, c[0], res.Source)
	}
}
