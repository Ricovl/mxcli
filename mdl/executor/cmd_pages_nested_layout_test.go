// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1411: a layout built on another layout.
//
// Studio Pro stores such a layout's tree in Content.LayoutCall — a call to the
// outer layout whose arguments fill the outer layout's placeholders — and leaves
// Content.Widgets empty. `describe layout` read only Content.Widgets, so it
// printed an empty body, and the output was refused ("declares no placeholders
// at all", MDL081) when fed back in.

// storedNestedLayout is ConversationalUI.Layout_MasterBase exactly as the
// Studio Pro-authored marketplace module (Conversational UI 7.3.1, Mendix
// 11.12.4) stores it, $IDs and empty appearances aside.
func storedNestedLayout() map[string]any {
	return map[string]any{
		"$Type": "Forms$Layout",
		"Name":  "Layout_MasterBase",
		"Appearance": map[string]any{
			"$Type": "Forms$Appearance", "Class": "", "Style": "", "DynamicClasses": "",
		},
		"Content": map[string]any{
			"$Type":      "Forms$WebLayoutContent",
			"LayoutType": "Responsive",
			"Widgets":    []any{int32(2)},
			"LayoutCall": map[string]any{
				"$Type": "Forms$LayoutCall",
				"Form":  "Atlas_Core.Atlas_Default",
				"Arguments": []any{int32(2), map[string]any{
					"$Type":     "Forms$FormCallArgument",
					"Parameter": "Atlas_Core.Atlas_Default.Main",
					"Widgets": []any{int32(2), map[string]any{
						"$Type":    "Forms$Placeholder",
						"Name":     "Main",
						"TabIndex": int32(0),
						"Appearance": map[string]any{
							"$Type": "Forms$Appearance", "Class": "", "Style": "", "DynamicClasses": "",
						},
					}},
				}},
			},
		},
	}
}

func describeStoredLayout(t *testing.T, raw map[string]any) string {
	t.Helper()
	mod := mkModule("ConversationalUI")
	layout := &pages.Layout{
		BaseElement: model.BaseElement{ID: model.ID("layout-1")},
		ContainerID: mod.ID,
		Name:        "Layout_MasterBase",
		LayoutType:  pages.LayoutTypeResponsive,
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{layout}, nil },
		GetRawUnitFunc:  func(model.ID) (map[string]any, error) { return raw, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	if err := describeLayout(ctx, ast.QualifiedName{Module: "ConversationalUI", Name: "Layout_MasterBase"}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestDescribeLayout_PrintsTheOuterLayoutAndItsArguments(t *testing.T) {
	got := describeStoredLayout(t, storedNestedLayout())
	for _, want := range []string{
		"Layout: Atlas_Core.Atlas_Default",
		"placeholder Main {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe lacks %q:\n%s", want, got)
		}
	}
	// The argument's own placeholder is the inner layout's declaration — the
	// slot its pages bind to — and must appear inside the argument block.
	if strings.Count(got, "placeholder Main") != 2 {
		t.Errorf("want the argument block AND the placeholder it declares:\n%s", got)
	}
}

// The describe output is only useful if it builds the same layout back.
func TestDescribeLayout_NestedOutputRebuildsTheLayoutCall(t *testing.T) {
	got := describeStoredLayout(t, storedNestedLayout())

	prog, errs := visitor.Build(got)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, got)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateLayoutStmt)
	if !ok {
		t.Fatalf("parsed %T", prog.Statements[0])
	}
	if v := validateLayoutPlaceholders(stmt); len(v) != 0 {
		t.Fatalf("describe output refused by check: %+v\n%s", v, got)
	}

	pb := newPopupPageBuilder()
	outer := &pages.Layout{BaseElement: model.BaseElement{ID: "outer"}, ContainerID: "atlas", Name: "Atlas_Default"}
	pb.layoutsCache = []*pages.Layout{outer}
	pb.execCache = &executorCache{hierarchy: mkHierarchy(&model.Module{BaseElement: model.BaseElement{ID: "atlas"}, Name: "Atlas_Core"})}
	l, err := pb.buildLayoutV3(stmt)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Widgets) != 0 {
		t.Errorf("a nested layout's content widgets must be empty, got %d", len(l.Widgets))
	}
	if l.LayoutCall == nil {
		t.Fatal("no LayoutCall built")
	}
	if l.LayoutCall.LayoutName != "Atlas_Core.Atlas_Default" {
		t.Errorf("layout call names %q", l.LayoutCall.LayoutName)
	}
	if len(l.LayoutCall.Arguments) != 1 {
		t.Fatalf("arguments = %d, want 1", len(l.LayoutCall.Arguments))
	}
	arg := l.LayoutCall.Arguments[0]
	if string(arg.ParameterID) != "Atlas_Core.Atlas_Default.Main" {
		t.Errorf("argument parameter = %q", arg.ParameterID)
	}
	if len(arg.Widgets) != 1 {
		t.Fatalf("argument widgets = %d, want the declared placeholder", len(arg.Widgets))
	}
	if ph, ok := arg.Widgets[0].(*pages.LayoutPlaceholder); !ok || ph.Name != "Main" {
		t.Errorf("argument widget = %#v, want placeholder Main", arg.Widgets[0])
	}
}

// A layout built on another one declares its placeholders inside the argument
// blocks; those count. Without an outer layout a braced block is still the
// mistake MDL083 reports (#1063).
func TestValidateLayoutPlaceholders_CountsArgumentBlocks(t *testing.T) {
	parse := func(src string) *ast.CreateLayoutStmt {
		t.Helper()
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("does not parse: %v", errs)
		}
		return prog.Statements[0].(*ast.CreateLayoutStmt)
	}

	nested := parse(`create layout M.Inner (layouttype: 'Responsive', Layout: Atlas_Core.Atlas_Default) {
  placeholder Main {
    container c1 { placeholder Main }
  }
}`)
	if v := validateLayoutPlaceholders(nested); len(v) != 0 {
		t.Errorf("nested layout declaring Main inside its argument was refused: %+v", v)
	}

	empty := parse(`create layout M.Inner (layouttype: 'Responsive', Layout: Atlas_Core.Atlas_Default) {
  placeholder Main {
    container c1
  }
}`)
	if v := validateLayoutPlaceholders(empty); len(v) == 0 || v[0].RuleID != "MDL081" {
		t.Errorf("a nested layout that declares no Main must still be MDL081, got %+v", v)
	}

	plain := parse(`create layout M.Plain (layouttype: 'Responsive') {
  placeholder Main { container c1 }
}`)
	if v := validateLayoutPlaceholders(plain); len(v) == 0 || v[0].RuleID != "MDL083" {
		t.Errorf("without an outer layout a braced block is MDL083, got %+v", v)
	}
}

// An argument naming a placeholder the outer layout does not declare fills
// nothing; mxbuild reports it as CE1613 a build later. Refuse it up front.
func TestExecCreateLayout_RefusesAnArgumentTheOuterLayoutLacks(t *testing.T) {
	atlas := &model.Module{BaseElement: model.BaseElement{ID: "atlas"}, Name: "Atlas_Core"}
	own := &model.Module{BaseElement: model.BaseElement{ID: "own"}, Name: "M"}
	outer := &pages.Layout{BaseElement: model.BaseElement{ID: "outer"}, ContainerID: atlas.ID, Name: "Atlas_Default", LayoutType: pages.LayoutTypeResponsive}
	var written *pages.Layout
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{atlas, own}, nil },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{outer}, nil },
		LayoutPlaceholdersFunc: func(id model.ID) ([]string, error) {
			return []string{"Main"}, nil
		},
		CreateLayoutFunc: func(l *pages.Layout) error { written = l; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(atlas, own)))

	build := func(src string) *ast.CreateLayoutStmt {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("does not parse: %v", errs)
		}
		return prog.Statements[0].(*ast.CreateLayoutStmt)
	}

	bad := build(`create layout M.Inner (layouttype: 'Responsive', Layout: Atlas_Core.Atlas_Default) {
  placeholder Main { placeholder Main }
  placeholder Sidebar { container c1 }
}`)
	if err := execCreateLayout(ctx, bad); err == nil || !strings.Contains(err.Error(), "Sidebar") {
		t.Fatalf("want a refusal naming Sidebar, got %v", err)
	}

	// Control: the same layout without the unknown argument is written.
	good := build(`create layout M.Inner (layouttype: 'Responsive', Layout: Atlas_Core.Atlas_Default) {
  placeholder Main { placeholder Main }
}`)
	if err := execCreateLayout(ctx, good); err != nil {
		t.Fatalf("control failed: %v", err)
	}
	if written == nil || written.LayoutCall == nil {
		t.Fatal("control wrote no layout call")
	}
}
