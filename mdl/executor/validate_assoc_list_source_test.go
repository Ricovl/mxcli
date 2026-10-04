// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// assocShapeCtx is a project with C88.A and C88.B and one stored association
// per measured shape (validate_assoc_list_source.go).
func assocShapeCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-c88"}, Name: "C88"}
	assoc := func(name string, typ domainmodel.AssociationType, owner domainmodel.AssociationOwner) *domainmodel.Association {
		return &domainmodel.Association{BaseElement: model.BaseElement{ID: model.ID("a-" + name)}, Name: name,
			ParentID: "e-a", ChildID: "e-b", Type: typ, Owner: owner}
	}
	b := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: "dm-c88"}, ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: "e-a"}, Name: "A"},
					{BaseElement: model.BaseElement{ID: "e-b"}, Name: "B"},
				},
				Associations: []*domainmodel.Association{
					assoc("R_def", domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerDefault),
					assoc("R_both", domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerBoth),
					assoc("RS_def", domainmodel.AssociationTypeReferenceSet, domainmodel.AssociationOwnerDefault),
					assoc("RS_both", domainmodel.AssociationTypeReferenceSet, domainmodel.AssociationOwnerBoth),
				},
			}}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(b))
	ctx.widgetRegistry = &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{}, byWidgetID: map[string]*WidgetDefinition{}}
	ctx.widgetRegistryLoaded = true
	return ctx
}

func assocListErrs(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	var out []string
	for _, st := range prog.Statements {
		if s, ok := st.(*ast.CreatePageStmtV3); ok {
			out = append(out, validatePluggableAttributeScopes(ctx, s.Layout, s.Parameters, allPageWidgets(s), sc)...)
		}
	}
	return out
}

func assocListPage(ent, widget, assoc string) string {
	return fmt.Sprintf(`create page C88.P (title: 'P', layout: Atlas_Core.Atlas_Default, params: ($P: C88.%s)) {
  dataview dv (datasource: $P) {
    %s w (datasource: $currentObject/C88.%s) { }
  }
}`, ent, widget, assoc)
}

// TestAssocListSource_MeasuredShapes pins the 11.13.0/11.14.0 measurement:
// exactly the shapes mxbuild rejects with CE8812 are reported, for each of the
// three list widgets, and the shapes it accepts are not (the controls).
func TestAssocListSource_MeasuredShapes(t *testing.T) {
	ctx := assocShapeCtx(t)
	for _, tc := range []struct {
		from, assoc string
		ce8812      bool
	}{
		{"A", "R_def", true},
		{"A", "R_both", true},
		{"B", "R_both", true},
		{"B", "R_def", false},
		{"A", "RS_def", false},
		{"B", "RS_def", false},
		{"A", "RS_both", false},
		{"B", "RS_both", false},
	} {
		for _, widget := range []string{"listview", "datagrid", "gallery"} {
			errs := assocListErrs(t, ctx, assocListPage(tc.from, widget, tc.assoc))
			got := len(errs) == 1 && strings.Contains(errs[0], "MDL-ASSOCDS01") && strings.Contains(errs[0], "CE8812")
			if got != tc.ce8812 || (!tc.ce8812 && len(errs) != 0) {
				t.Errorf("%s from %s over %s: got %q, want CE8812=%v", widget, tc.from, tc.assoc, errs, tc.ce8812)
			}
		}
	}
}

// A data view over the same single-object path is fine: only list widgets need
// a list.
func TestAssocListSource_DataViewIsNotJudged(t *testing.T) {
	if errs := assocListErrs(t, assocShapeCtx(t), assocListPage("A", "dataview", "R_def")); len(errs) != 0 {
		t.Errorf("a data view over a Reference was reported: %q", errs)
	}
}

// The association created by the same script is judged too — the issue's own
// shape, where everything is new.
func TestAssocListSource_ScriptDeclaredAssociation(t *testing.T) {
	src := `create association C88.New_both from C88.A to C88.B type Reference owner Both;
create association C88.New_set from C88.A to C88.B type ReferenceSet owner Both;
` + assocListPage("B", "listview", "New_both") + ";\n" + strings.Replace(assocListPage("B", "listview", "New_set"), "C88.P ", "C88.P2 ", 1)
	errs := assocListErrs(t, assocShapeCtx(t), src)
	if len(errs) != 1 || !strings.Contains(errs[0], "C88.New_both") {
		t.Errorf("want one MDL-ASSOCDS01 for New_both and none for the ReferenceSet control, got %q", errs)
	}
}
