// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"
)

func applyFix(t *testing.T, src string, f Fix) string {
	t.Helper()
	out, err := newSource(src).apply(f.Edits)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A fix rewrites its own use and nothing else, and a deprecated spelling's fix
// is the rewrite Upgrade applies to it.
func TestFixesOnLinesRewritesOneUse(t *testing.T) {
	src := "create or replace entity M.A (Name: String(20));\n" +
		"create or replace entity M.B (Name: String(20));\n"
	fixes, err := FixesOnLines(src, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 1 {
		t.Fatalf("want one fix on line 2, got %+v", fixes)
	}
	f := fixes[0]
	if f.Code != "MDL-DEPR001" || f.Line != 2 || f.Column != 10 {
		t.Errorf("fix = %+v, want MDL-DEPR001 at 2:10", f)
	}
	want := "create or replace entity M.A (Name: String(20));\n" +
		"create or modify entity M.B (Name: String(20));\n"
	if got := applyFix(t, src, f); got != want {
		t.Errorf("fix gave\n%s\nwant\n%s", got, want)
	}
}

// A header-gated construct whose rewrite keeps its meaning only under the
// header has no fix on its own; a version-neutral one does.
func TestFixesOnLinesGatedConstructs(t *testing.T) {
	limit := "create microflow M.F ()\nbegin\n  retrieve $x from M.E limit 1;\nend;\n"
	fixes, err := FixesOnLines(limit, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 0 {
		t.Errorf("MDL-V1-LIMIT1 is not version-neutral, so it has no fix without the header: %+v", fixes)
	}

	quoted := "create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {\n" +
		"  container c1 (dynamicclasses: '@M.CardClass') { }\n};\n"
	prog, errs := parse(quoted)
	if errs != nil {
		t.Fatal(errs)
	}
	if uses(prog, "MDL-V1-QUOTEDEXPR") != 1 {
		t.Fatalf("fixture should record MDL-V1-QUOTEDEXPR once: %+v", prog.LanguageNotes)
	}
	fixes, err = FixesOnLines(quoted, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 1 || fixes[0].Code != "MDL-V1-QUOTEDEXPR" || fixes[0].Column != -1 {
		t.Fatalf("want one MDL-V1-QUOTEDEXPR fix, got %+v", fixes)
	}
	res, err := Upgrade(quoted, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := applyFix(t, quoted, fixes[0]); got != res.Source {
		t.Errorf("the fix gave\n%s\nfmt --upgrade gave\n%s", got, res.Source)
	}
}

// A use with no rewrite gets no fix.
func TestFixesOnLinesSkipsUseWithoutRewrite(t *testing.T) {
	src := "create or replace view entity M.V (Name: String) as select e.Name as Name from M.E as e;\n"
	fixes, err := FixesOnLines(src, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 0 {
		t.Errorf("want no fixes, got %+v", fixes)
	}
}
