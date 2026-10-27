// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1703 (mendixlabs/mxcli#1659): a scroll container's LayoutMode —
// Headline or Sidebar, which decides whether the top bar or the left region
// runs the full width — was neither described nor written, so the documented
// describe → rename → exec copy of Atlas_Core.Atlas_SideBar was a Headline
// layout, and `(LayoutMode: 'Sidebar')` was dropped in silence.

func scrollContainerOf(t *testing.T, props string) *ast.WidgetV3 {
	t.Helper()
	prog, errs := visitor.Build("create layout M.L (layouttype: 'Responsive') {\n  scrollcontainer sc " + props +
		" {\n    region center { placeholder Main }\n  }\n};\n")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog.Statements[0].(*ast.CreateLayoutStmt).Widgets[0]
}

func TestScrollContainerLayoutMode_IsWritten(t *testing.T) {
	for _, c := range []struct{ props, want string }{
		{"(LayoutMode: 'Sidebar')", "Sidebar"},
		{"(layoutmode: sidebar)", "Sidebar"}, // any case, bare or quoted; stored as the metamodel spells it
		{"(LayoutMode: 'Headline')", "Headline"},
		{"", ""}, // control: unsaid stays unwritten, which Studio Pro reads as Headline
	} {
		pb := visibleWhenPB("")
		pb.ctx = &ExecContext{Output: &bytes.Buffer{}, LanguageVersion: langver.V1}
		built, err := pb.buildWidgetV3(scrollContainerOf(t, c.props))
		if err != nil {
			t.Fatalf("%q: %v", c.props, err)
		}
		if got := built.(*pages.ScrollContainer).LayoutMode; got != c.want {
			t.Errorf("%q: LayoutMode = %q, want %q", c.props, got, c.want)
		}
	}
}

func TestScrollContainerLayoutMode_UnknownValueIsRefused(t *testing.T) {
	pb := visibleWhenPB("")
	pb.ctx = &ExecContext{Output: &bytes.Buffer{}, LanguageVersion: langver.V1}
	_, err := pb.buildWidgetV3(scrollContainerOf(t, "(LayoutMode: 'Topbar')"))
	if err == nil || !strings.Contains(err.Error(), "Headline or Sidebar") {
		t.Errorf("err = %v, want a refusal naming Headline or Sidebar", err)
	}
}

// The property is checked, not just built: `check` accepts it where, before
// the support, the per-widget verdict refused it (MDL-WIDGET52).
func TestScrollContainerLayoutMode_CheckAccepts(t *testing.T) {
	vs := nativePropViolations(t, "mdl 1;\ncreate layout M.L (layouttype: 'Responsive') {\n"+
		"  scrollcontainer sc (LayoutMode: 'Sidebar') { region center { placeholder Main } }\n};\n")
	if len(vs) != 0 {
		t.Errorf("check refuses a scroll container's LayoutMode:\n%s", violationText(vs))
	}
}

// describe prints Sidebar, and leaves Headline — the default — unsaid, so a
// layout that never set it describes as before.
func TestScrollContainerLayoutMode_Describe(t *testing.T) {
	for _, c := range []struct{ stored, want string }{
		{"Sidebar", "scrollcontainer layoutContainer (LayoutMode: 'Sidebar') {"},
		{"Headline", "scrollcontainer layoutContainer {"},
		{"", "scrollcontainer layoutContainer {"},
	} {
		stored := map[string]any{
			"$Type": "Forms$ScrollContainer",
			"Name":  "layoutContainer",
			"CenterRegion": map[string]any{
				"$Type":   "Forms$ScrollContainerRegion",
				"Widgets": []any{int32(2), map[string]any{"$Type": "Forms$Placeholder", "Name": "Main"}},
			},
		}
		if c.stored != "" {
			stored["LayoutMode"] = c.stored
		}
		var buf bytes.Buffer
		ctx := (&Executor{}).newExecContext(context.Background())
		ctx.Output = &buf
		outputWidgetMDLV3(ctx, parseRawWidget(ctx, stored)[0], 1)
		if !strings.Contains(buf.String(), c.want) {
			t.Errorf("stored %q: describe lacks %q:\n%s", c.stored, c.want, buf.String())
		}
	}
}

// The reported workflow end to end on the Studio Pro-authored PedApp: describe
// Atlas_SideBar, run it under a new name, and the copy stores Sidebar. The
// control is Atlas_Default, a Headline layout, whose copy stays Headline and
// whose describe says nothing about it.
func TestCopyAtlasSideBarKeepsItsLayoutMode(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	for _, c := range []struct{ name, want string }{
		{"Atlas_SideBar", "Sidebar"},
		{"Atlas_Default", "Headline"},
	} {
		if got := storedScrollLayoutModes(t, exec, "Atlas_Core."+c.name); len(got) != 1 || got[0] != c.want {
			t.Fatalf("fixture: %s stores LayoutMode %v, want [%s]; the test proves nothing", c.name, got, c.want)
		}
		out.Reset()
		if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeLayout, Name: ast.QualifiedName{Module: "Atlas_Core", Name: c.name}}); err != nil {
			t.Fatalf("describe layout %s: %v", c.name, err)
		}
		described := languageHeader.ReplaceAllString(out.String(), "")
		if says := strings.Contains(described, "LayoutMode:"); says != (c.want == "Sidebar") {
			t.Errorf("%s: describe prints LayoutMode = %v, want %v:\n%s", c.name, says, c.want == "Sidebar", described)
		}
		copied := strings.Replace(described, "create or modify layout Atlas_Core."+c.name+" ", "create layout MyFirstModule.Copy_"+c.name+" ", 1)
		if got := agreeCheck(t, exec, dir, "mdl 1;\n"+copied); len(got) != 0 {
			t.Fatalf("%s: check refuses the copy:\n%s", c.name, strings.Join(got, "\n"))
		}
		if err := agreeExec(t, exec, "mdl 1;\n"+copied); err != nil {
			t.Fatalf("%s: exec of the copy: %v", c.name, err)
		}
		if got := storedScrollLayoutModes(t, exec, "MyFirstModule.Copy_"+c.name); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: the copy stores LayoutMode %v, Studio Pro stores [%s]", c.name, got, c.want)
		}
	}
}

// storedScrollLayoutModes is the LayoutMode of every scroll container in a
// layout, in document order ("" for one that stores none).
func storedScrollLayoutModes(t *testing.T, exec *Executor, qn string) []string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	u, err := ctx.Backend.GetRawUnitByName("layout", qn)
	if err != nil || u == nil {
		t.Fatalf("raw layout %s: %v", qn, err)
	}
	var modes []string
	var walk func(v bson.RawValue)
	walk = func(v bson.RawValue) {
		switch v.Type {
		case bson.TypeEmbeddedDocument:
			doc := v.Document()
			if doc.Lookup("$Type").StringValue() == "Forms$ScrollContainer" {
				mode, _ := doc.Lookup("LayoutMode").StringValueOK()
				modes = append(modes, mode)
			}
			elems, _ := doc.Elements()
			for _, e := range elems {
				walk(e.Value())
			}
		case bson.TypeArray:
			vals, _ := v.Array().Values()
			for _, x := range vals {
				walk(x)
			}
		}
	}
	walk(bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: u.Contents})
	return modes
}
