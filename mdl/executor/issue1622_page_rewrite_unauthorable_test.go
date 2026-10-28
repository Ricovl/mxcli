// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1622 (ako/mxcli#1554): `describe page M.P` prints a classic
// data grid, a reference selector, a drop-down or a reference set selector as
// a `-- NOT re-executable` comment, and executing that output replaced the
// page with the widget gone — "Replaced page M.P", no warning. MDL cannot
// author those widgets, so a rewrite cannot keep them; it is refused instead,
// naming each one and the ALTER PAGE that edits the page without them.

func pageFixtureExecutor(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	exec := New(&out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	out.Reset()
	return exec, &out
}

// runErr executes mdl and returns the first statement error.
func runErr(t *testing.T, exec *Executor, mdl string) error {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", mdl, errs)
	}
	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			return err
		}
	}
	return nil
}

func toBsonD(t *testing.T, m map[string]any) bson.D {
	t.Helper()
	raw, err := bson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// referenceSelectorBSON is a Forms$ReferenceSelector as Studio Pro stores one
// (its kind is enough: describe has no case for it).
func referenceSelectorBSON() map[string]any {
	return map[string]any{
		"$Type": "Forms$ReferenceSelector",
		"Name":  "referenceSelector1",
	}
}

// unitIDByName finds a page or snippet unit.
func unitIDByName(t *testing.T, exec *Executor, kind, name string) string {
	t.Helper()
	switch kind {
	case "page":
		ps, err := exec.backend.ListPages()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range ps {
			if p.Name == name {
				return string(p.ID)
			}
		}
	case "snippet":
		ss, err := exec.backend.ListSnippets()
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range ss {
			if s.Name == name {
				return string(s.ID)
			}
		}
	}
	t.Fatalf("no %s %s", kind, name)
	return ""
}

// seedIntoContainer appends Studio Pro-authored widgets to the stored
// container named container, the way a page built in Studio Pro holds them.
func seedIntoContainer(t *testing.T, exec *Executor, unitID, container string, widgets ...map[string]any) {
	t.Helper()
	raw, err := exec.backend.GetRawUnitBytes(model.ID(unitID))
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	found := false
	var walk func(v any) any
	walk = func(v any) any {
		switch n := v.(type) {
		case bson.D:
			isTarget := false
			for _, e := range n {
				if e.Key == "Name" && e.Value == container {
					isTarget = true
				}
			}
			for i, e := range n {
				if isTarget && e.Key == "Widgets" {
					arr := e.Value.(bson.A)
					for _, w := range widgets {
						arr = append(arr, toBsonD(t, w))
					}
					n[i].Value = arr
					found = true
					continue
				}
				n[i].Value = walk(e.Value)
			}
			return n
		case bson.A:
			for i := range n {
				n[i] = walk(n[i])
			}
			return n
		}
		return v
	}
	doc = walk(doc).(bson.D)
	if !found {
		t.Fatalf("no container %s with a Widgets list in the stored unit", container)
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.backend.UpdateRawUnit(unitID, out); err != nil {
		t.Fatal(err)
	}
}

func storedHas(t *testing.T, exec *Executor, unitID, needle string) bool {
	t.Helper()
	raw, err := exec.backend.GetRawUnitBytes(model.ID(unitID))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Contains(raw, []byte(needle))
}

const issue1622Page = `create page MyFirstModule.Orders (Title: 'Orders', Layout: Atlas_Core.Atlas_Default) {
  container ctn1 {
    dynamictext txtHeading (Content: 'Orders')
  }
};`

// The report: describe → exec of a page holding a classic data grid and a
// reference selector.
func TestIssue1622_RewriteOfPageWithUnauthorableWidgetsIsRefused(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue1622Page)
	id := unitIDByName(t, exec, "page", "Orders")
	seedIntoContainer(t, exec, id, "ctn1", legacyDataGridBSON(), referenceSelectorBSON())

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Orders")
	described := out.String()
	for _, want := range []string{"-- Forms$DataGrid (grid1)", "-- Forms$ReferenceSelector (referenceSelector1)"} {
		if !strings.Contains(described, want) {
			t.Fatalf("describe output lacks %q:\n%s", want, described)
		}
	}

	out.Reset()
	err := runErr(t, exec, described)
	if err == nil {
		t.Fatalf("executing the describe output replaced the page:\n%s", out.String())
	}
	t.Log(err)
	for _, want := range []string{"grid1", "Forms$DataGrid", "referenceSelector1", "Forms$ReferenceSelector", "alter page MyFirstModule.Orders"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q:\n%v", want, err)
		}
	}
	for _, kept := range []string{"Forms$DataGrid", "Forms$ReferenceSelector"} {
		if !storedHas(t, exec, id, kept) {
			t.Errorf("the stored page lost its %s", kept)
		}
	}
}

// The refusal is about what would be lost, not about the page: once ALTER PAGE
// has removed the widgets on purpose, the rewrite goes through — and a page
// with no such widget was never affected.
func TestIssue1622_RewriteAfterDroppingTheWidgetsGoesThrough(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue1622Page)
	id := unitIDByName(t, exec, "page", "Orders")
	seedIntoContainer(t, exec, id, "ctn1", legacyDataGridBSON(), referenceSelectorBSON())

	run(t, exec, "alter page MyFirstModule.Orders { drop grid1, referenceSelector1; };")
	out.Reset()
	run(t, exec, "describe page MyFirstModule.Orders")
	described := out.String()
	if strings.Contains(described, "NOT re-executable") {
		t.Fatalf("the widgets were not dropped:\n%s", described)
	}
	if err := runErr(t, exec, described); err != nil {
		t.Errorf("a page with nothing MDL cannot author was refused: %v", err)
	}
}

// A snippet is rewritten by the same builder and loses the same widgets.
func TestIssue1622_RewriteOfSnippetWithUnauthorableWidgetIsRefused(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, `create snippet MyFirstModule.OrderList {
  container ctn1 {
    dynamictext txtHeading (Content: 'Orders')
  }
};`)
	id := unitIDByName(t, exec, "snippet", "OrderList")
	seedIntoContainer(t, exec, id, "ctn1", legacyDataGridBSON())

	out.Reset()
	run(t, exec, "describe snippet MyFirstModule.OrderList")
	err := runErr(t, exec, out.String())
	if err == nil || !strings.Contains(err.Error(), "grid1") || !strings.Contains(err.Error(), "alter snippet MyFirstModule.OrderList") {
		t.Errorf("snippet rewrite: %v", err)
	}
	if !storedHas(t, exec, id, "Forms$DataGrid") {
		t.Error("the stored snippet lost its data grid")
	}
}

// `check -p` says the same before anything is written.
func TestIssue1622_CheckReportsTheWidgetsARewriteWouldDrop(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue1622Page)
	id := unitIDByName(t, exec, "page", "Orders")
	seedIntoContainer(t, exec, id, "ctn1", legacyDataGridBSON(), referenceSelectorBSON())
	out.Reset()
	run(t, exec, "describe page MyFirstModule.Orders")

	prog, errs := visitor.Build(out.String())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	verrs := exec.ValidateProgram(prog)
	joined := ""
	for _, e := range verrs {
		joined += e.Error() + "\n"
	}
	if !strings.Contains(joined, "grid1") || !strings.Contains(joined, "referenceSelector1") {
		t.Errorf("check does not report the widgets a rewrite would drop:\n%s", joined)
	}
}

// A layout is described as `create or modify layout` and rebuilt by the same
// builder, so it loses the same widgets.
func TestIssue1622_RewriteOfLayoutWithUnauthorableWidgetIsRefused(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, `create layout MyFirstModule.Shell (layouttype: 'Responsive') {
  scrollcontainer sc1 {
    region center {
      container ctn1 {
        dynamictext txtBrand (Content: 'Shop')
      }
      placeholder Main
    }
  }
};`)
	ls, err := exec.backend.ListLayouts()
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, l := range ls {
		if l.Name == "Shell" {
			id = string(l.ID)
		}
	}
	seedIntoContainer(t, exec, id, "ctn1", referenceSelectorBSON())

	out.Reset()
	run(t, exec, "describe layout MyFirstModule.Shell")
	described := out.String()
	if !strings.Contains(described, "-- Forms$ReferenceSelector (referenceSelector1)") {
		t.Fatalf("describe output lacks the reference selector:\n%s", described)
	}
	err = runErr(t, exec, described)
	if err == nil || !strings.Contains(err.Error(), "referenceSelector1") || !strings.Contains(err.Error(), "alter layout MyFirstModule.Shell") {
		t.Errorf("layout rewrite: %v", err)
	}
	if !storedHas(t, exec, id, "Forms$ReferenceSelector") {
		t.Error("the stored layout lost its reference selector")
	}
}

// check predicts the stored state a statement meets: an ALTER earlier in the
// same script that drops the widgets means the rewrite after it loses nothing.
func TestIssue1622_CheckFollowsAnEarlierAlterInTheScript(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue1622Page)
	id := unitIDByName(t, exec, "page", "Orders")
	seedIntoContainer(t, exec, id, "ctn1", legacyDataGridBSON(), referenceSelectorBSON())
	out.Reset()
	run(t, exec, "describe page MyFirstModule.Orders")

	described := strings.Replace(out.String(), "mdl 1;\n", "", 1)
	prog, errs := visitor.Build("mdl 1;\nalter page MyFirstModule.Orders { drop grid1, referenceSelector1; };\n" + described)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, e := range exec.ValidateProgram(prog) {
		if strings.Contains(e.Error(), "cannot author") {
			t.Errorf("check refused a rewrite that follows the ALTER dropping the widgets: %v", e)
		}
	}
}

// A widget the statement declares by name is the author writing it: a rewrite
// that restates it does not count it as lost.
func TestIssue1622_DeclaredWidgetIsNotCountedAsLost(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue1622Page)
	id := unitIDByName(t, exec, "page", "Orders")
	seedIntoContainer(t, exec, id, "ctn1", legacyDataGridBSON(), referenceSelectorBSON())

	err := runErr(t, exec, `create or modify page MyFirstModule.Orders (Title: 'Orders', Layout: Atlas_Core.Atlas_Default) {
  container ctn1 {
    dynamictext txtHeading (Content: 'Orders')
    container grid1 { dynamictext txtGrid (Content: 'replaced on purpose') }
  }
};`)
	if err == nil || strings.Contains(err.Error(), "grid1 (") || !strings.Contains(err.Error(), "referenceSelector1") {
		t.Errorf("the declared grid1 was counted, or the undeclared selector was not: %v", err)
	}
}
