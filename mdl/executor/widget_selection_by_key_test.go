// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1618: a Data grid 2 or Gallery selection written by the
// widget's own key — `itemSelection: Single` — passed check, exec and mx check,
// and was stored as the property's PrimitiveValue with its Selection left at
// the default, so the grid had no selection and a gallery could not turn its
// default selection off:
//
//	dgSingle (itemSelection: Single)  →  Selection "None", PrimitiveValue "Single"
//	galNone  (itemSelection: None)    →  Selection "Single", PrimitiveValue "None"
//
// The mapping pass read only the `Selection:` keyword, and the explicit-key pass
// skipped "already handled" names by comparing what the author wrote with the
// mappings' source keywords (`Selection`), not their property keys, so the key
// fell through to the scalar branch.
//
// Two neighbours of the same silence were in the report: an invalid value
// (`itemSelection: Bogus`) was accepted, and a definition constant overridden by
// key on a widget that does not declare the property (Gallery 3.4.0 has no
// `advanced`; its hand-written definition maps one to the constant false) was
// discarded.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// selectionEngine builds with the REAL embedded templates over Shop.Ord (Code).
func selectionEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-shop")}, Name: "Shop"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-shop")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-ord")}, Name: "Ord",
						Attributes: []*domainmodel.Attribute{{Name: "Code"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

// selectionDef is the definition exec builds the widget from: the Gallery's
// hand-written one, and the Data grid 2's generated from its package.
func selectionDef(t *testing.T, keyword string) *WidgetDefinition {
	t.Helper()
	if keyword == "DATAGRID" {
		d, err := mpk.ParseMPKWidget("../../testdata/expr-checker/widgets/com.mendix.widget.web.Datagrid.mpk", pages.WidgetIDDataGrid2)
		if err != nil {
			t.Fatalf("parse Datagrid.mpk: %v", err)
		}
		return GenerateDefJSON(d, "DATAGRID")
	}
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	def, ok := reg.Get(keyword)
	if !ok {
		t.Fatalf("no %s definition", keyword)
	}
	return def
}

func buildSelectionWidget(t *testing.T, keyword, src string) (*pages.CustomWidget, error) {
	t.Helper()
	w := parseSnippetWidget(t, src)
	return selectionEngine(t).Build(selectionDef(t, keyword), w)
}

const (
	dgBody  = " { column (Attribute: Code) }"
	galBody = " { dynamictext t (Content: 'x') }"
)

func TestBuild_SelectionByKeyIsStoredAsTheSelection(t *testing.T) {
	cases := []struct {
		name, keyword, src, want string
	}{
		{"datagrid single", "DATAGRID", "datagrid dg (DataSource: database Shop.Ord, itemSelection: Single)" + dgBody, "Single"},
		{"datagrid multi", "DATAGRID", "datagrid dg (DataSource: database Shop.Ord, itemSelection: Multi)" + dgBody, "Multi"},
		{"gallery none", "GALLERY", "gallery g (DataSource: database Shop.Ord, itemSelection: None)" + galBody, "None"},
		{"gallery multi", "GALLERY", "gallery g (DataSource: database Shop.Ord, itemSelection: Multi)" + galBody, "Multi"},
		// The two spellings agreeing is one value, not two.
		{"gallery both spellings, agreeing", "GALLERY", "gallery g (DataSource: database Shop.Ord, Selection: None, itemSelection: None)" + galBody, "None"},
		// CONTROL: the keyword, which always worked, and the defaults.
		{"control: datagrid keyword", "DATAGRID", "datagrid dg (DataSource: database Shop.Ord, Selection: Single)" + dgBody, "Single"},
		{"control: gallery keyword", "GALLERY", "gallery g (DataSource: database Shop.Ord, Selection: None)" + galBody, "None"},
		{"control: gallery default", "GALLERY", "gallery g (DataSource: database Shop.Ord)" + galBody, "Single"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cw, err := buildSelectionWidget(t, tc.keyword, tc.src)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			v := customWidgetValue(t, cw, "itemSelection")
			sel, _ := bsonField(v, "Selection").(string)
			prim, _ := bsonField(v, "PrimitiveValue").(string)
			if sel != tc.want || prim != "" {
				t.Errorf("itemSelection stored Selection %q, PrimitiveValue %q; want Selection %q, PrimitiveValue \"\" "+
					"(mendixlabs/mxcli#1618)", sel, prim, tc.want)
			}
		})
	}
}

// An invalid selection is refused rather than stored. By key it used to land
// in PrimitiveValue, where nothing reads it.
func TestBuild_InvalidSelectionIsRefused(t *testing.T) {
	for _, src := range []string{
		"gallery g (DataSource: database Shop.Ord, itemSelection: Bogus)" + galBody,
		"gallery g (DataSource: database Shop.Ord, Selection: Bogus)" + galBody,
	} {
		_, err := buildSelectionWidget(t, "GALLERY", src)
		if err == nil || !strings.Contains(err.Error(), "Bogus") || !strings.Contains(err.Error(), "None, Single or Multi") {
			t.Errorf("%s\n  built with err %v; want the value refused, naming None, Single or Multi", src, err)
		}
	}
}

// The keyword and the key both name itemSelection; two different values for it
// cannot both be written.
func TestBuild_DisagreeingSelectionSpellingsAreRefused(t *testing.T) {
	_, err := buildSelectionWidget(t, "GALLERY",
		"gallery g (DataSource: database Shop.Ord, Selection: Single, itemSelection: Multi)"+galBody)
	if err == nil || !strings.Contains(err.Error(), "itemSelection") || !strings.Contains(err.Error(), "Selection") {
		t.Fatalf("built with err %v; want the two values refused", err)
	}
}

// A definition constant the author overrides by key, on a widget that does not
// declare the property, cannot be written. The Gallery's hand-written
// definition maps `advanced` to false; Gallery 3.4.0 (and the embedded 11.6
// template) declare no `advanced`.
func TestBuild_ConstantOverrideOfAnUndeclaredPropertyIsRefused(t *testing.T) {
	_, err := buildSelectionWidget(t, "GALLERY", "gallery g (DataSource: database Shop.Ord, advanced: true)"+galBody)
	if err == nil || !strings.Contains(err.Error(), "advanced") || !strings.Contains(err.Error(), "does not declare") {
		t.Fatalf("built with err %v; want `advanced` refused as undeclared", err)
	}
	// CONTROL: the constant alone is the definition's, not the script's, and
	// does not stop a gallery being built.
	if _, err := buildSelectionWidget(t, "GALLERY", "gallery g (DataSource: database Shop.Ord)"+galBody); err != nil {
		t.Fatalf("a plain gallery: %v", err)
	}
}

// check reports what exec refuses, before exec.
func TestCheck_SelectionValuesAndSpellings(t *testing.T) {
	page := func(widget string) string {
		return "mdl 1;\ncreate page Shop.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n  " + widget + "\n};"
	}
	cases := []struct {
		name, widget, rule, want string
	}{
		{"invalid by key", "gallery g (DataSource: database Shop.Ord, itemSelection: Bogus)" + galBody,
			"MDL-WIDGET50", "None, Single or Multi"},
		{"invalid by key on a datagrid", "datagrid dg (DataSource: database Shop.Ord, itemSelection: Bogus)" + dgBody,
			"MDL-WIDGET50", "None, Single or Multi"},
		{"two values", "gallery g (DataSource: database Shop.Ord, Selection: Single, itemSelection: Multi)" + galBody,
			"MDL-WIDGET50", "two values"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs := widgetViolations(t, page(tc.widget), tc.rule)
			if len(vs) != 1 || !strings.Contains(vs[0].Message, tc.want) {
				t.Fatalf("violations %+v; want one %s saying %q", vs, tc.rule, tc.want)
			}
		})
	}
	// CONTROL: valid values, in either spelling, report nothing.
	for _, w := range []string{
		"gallery g (DataSource: database Shop.Ord, itemSelection: None)" + galBody,
		"gallery g (DataSource: database Shop.Ord, Selection: Single)" + galBody,
		"datagrid dg (DataSource: database Shop.Ord, itemSelection: Multi)" + dgBody,
		"gallery g (DataSource: database Shop.Ord, Selection: None, itemSelection: none)" + galBody,
	} {
		if vs := widgetViolations(t, page(w), "MDL-WIDGET50"); len(vs) != 0 {
			t.Errorf("%s: %+v", w, vs)
		}
	}
}

// The Gallery's click-ambiguity rule reads the selection the builder now
// writes, by either spelling: `itemSelection: None` turns the selection off.
func TestCheck_GalleryClickAmbiguityReadsTheKey(t *testing.T) {
	src := func(sel string) string {
		return "mdl 1;\ncreate page Shop.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" +
			"  gallery g (DataSource: database Shop.Ord, " + sel + "onClick: call nanoflow Shop.Open)" + galBody + "\n};"
	}
	if vs := widgetViolations(t, src("itemSelection: None, "), "MDL-WIDGET36"); len(vs) != 0 {
		t.Errorf("`itemSelection: None` is no selection, yet the click was called ambiguous: %+v", vs)
	}
	// CONTROL: a selection by key is still one.
	if vs := widgetViolations(t, src("itemSelection: Single, "), "MDL-WIDGET36"); len(vs) != 1 {
		t.Errorf("`itemSelection: Single` with an onClick: %+v, want MDL-WIDGET36", vs)
	}
}

// check -p reports the undeclared override too: it reads the installed
// package (testdata/expr-checker ships Gallery 3.4.0, which has no `advanced`).
// Without a project it cannot know which version is installed, and stays
// quiet — exec, which has the project, refuses it.
func TestCheck_ConstantOverrideOfAnUndeclaredProperty(t *testing.T) {
	const src = "mdl 1;\ncreate page Shop.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  gallery g (DataSource: database Shop.Ord, advanced: true)" + galBody + "\n};"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	find := func(vs []linter.Violation) []linter.Violation {
		var out []linter.Violation
		for _, v := range vs {
			if v.RuleID == "MDL-WIDGET01" && strings.Contains(v.Message, "advanced") {
				out = append(out, v)
			}
		}
		return out
	}
	withProject := find(ValidateWidgetProperties(prog, "../../testdata/expr-checker/minimal.mpr"))
	if len(withProject) != 1 || !strings.Contains(withProject[0].Message, "does not declare") {
		t.Fatalf("check -p: %+v; want one MDL-WIDGET01 saying the installed Gallery does not declare `advanced`", withProject)
	}
	// CONTROL: no project, no installed version to judge by.
	if got := find(ValidateWidgetProperties(prog, "")); len(got) != 0 {
		t.Errorf("check without a project reported %+v", got)
	}
}
