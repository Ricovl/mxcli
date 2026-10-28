// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1466, the two one-hop comparands that still passed `check`
// after MDL055 learned the two-hop form. Each page below was one page in a
// fresh `mxcli new` project, built with mxbuild 11.14.0 and 11.13.0 (identical);
// a "want" rule means that page was
//
//	[error] [CE0161] "Error(s) in XPath constraint." at Combo box '…'
//
// and no rule means it built at 0 errors.
const comparandDomain = `
create module Bm;
create persistent entity Bm.Region (Name: String(100));
create persistent entity Bm.SubRegion extends Bm.Region (Extra: String(10));
create persistent entity Bm.District (Name: String(100));
create persistent entity Bm.City (Name: String(100));
create persistent entity Bm.AreaPicker (Label: String(50));
create association Bm.District_Region from Bm.District to Bm.Region type Reference;
create association Bm.District_SubRegion from Bm.District to Bm.SubRegion type Reference;
create association Bm.City_Region from Bm.City to Bm.Region type Reference;
create association Bm.AreaPicker_Region from Bm.AreaPicker to Bm.Region type Reference;
create association Bm.AreaPicker_SubRegion from Bm.AreaPicker to Bm.SubRegion type Reference;
create association Bm.AreaPicker_District from Bm.AreaPicker to Bm.District type Reference;
create association Bm.AreaPicker_City from Bm.AreaPicker to Bm.City type Reference;
`

func comparandPage(widget string) string {
	return `create page Bm.P (Params: ($Picker: Bm.AreaPicker), Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dataview dv (DataSource: $Picker) {
    ` + widget + `
  }
};`
}

func comparandCombo(where string) string {
	return `combobox cb (Association: Bm.AreaPicker_District, DataSource: database Bm.District where [` + where + `], CaptionAttribute: Name)`
}

func comparandViolations(t *testing.T, src string, project func() backend.FullBackend) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if project == nil {
		project = func() backend.FullBackend { return nil }
	}
	return ValidateDatasourceXPathComparands(prog, project)
}

func TestDatasourceXPathComparand_Measured(t *testing.T) {
	cases := []struct {
		name   string
		widget string
		rule   string // "" = builds clean
	}{
		// Shape 1: the comparand lands on another entity than the left side.
		{"target entity differs", comparandCombo(`Bm.District_Region = $currentObject/Bm.AreaPicker_City`), "MDL-XPATH02"},
		{"long left side, target differs", comparandCombo(`Bm.District_Region/Bm.Region/Bm.City_Region = $currentObject/Bm.AreaPicker_Region`), "MDL-XPATH02"},
		{"page parameter as the root", comparandCombo(`Bm.District_Region = $Picker/Bm.AreaPicker_City`), "MDL-XPATH02"},
		{"list view", `listview lv (DataSource: database from Bm.District where [Bm.District_Region = $currentObject/Bm.AreaPicker_City]) { dynamictext t (Content: '{1}', ContentParams: ({1} = Name)) }`, "MDL-XPATH02"},
		{"association against an attribute", comparandCombo(`Bm.District_Region = $currentObject/Label`), "MDL-XPATH02"},
		// Shape 2: the comparand's association is unqualified.
		{"unqualified association", comparandCombo(`Bm.District_Region = $currentObject/AreaPicker_Region`), "MDL-XPATH01"},

		// Controls.
		{"one hop, same entity (the issue's form)", comparandCombo(`Bm.District_Region = $currentObject/Bm.AreaPicker_Region`), ""},
		{"comparand is a specialization", comparandCombo(`Bm.District_Region = $currentObject/Bm.AreaPicker_SubRegion`), ""},
		{"comparand is a generalization", comparandCombo(`Bm.District_SubRegion = $currentObject/Bm.AreaPicker_Region`), ""},
		{"long left side, same entity", comparandCombo(`Bm.District_Region/Bm.Region/Bm.City_Region = $currentObject/Bm.AreaPicker_City`), ""},
		{"inverted form", comparandCombo(`Bm.District_Region/Bm.Region/Bm.AreaPicker_Region = $currentObject`), ""},
		{"own attribute", comparandCombo(`Name = $currentObject/Label`), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := comparandViolations(t, comparandDomain+comparandPage(c.widget), nil)
			if c.rule == "" {
				if len(vs) != 0 {
					t.Fatalf("builds clean, want no violation, got %q", vs[0].Message)
				}
				return
			}
			if len(vs) != 1 || vs[0].RuleID != c.rule || vs[0].Severity != linter.SeverityError {
				t.Fatalf("want one %s error, got %+v", c.rule, vs)
			}
			for _, want := range []string{"CE0161", "Bm.P"} {
				if !strings.Contains(vs[0].Message, want) {
					t.Errorf("message should mention %q, got: %s", want, vs[0].Message)
				}
			}
		})
	}
}

// The messages name the two entities, and the unqualified one its spelling.
func TestDatasourceXPathComparand_Messages(t *testing.T) {
	vs := comparandViolations(t, comparandDomain+comparandPage(comparandCombo(`Bm.District_Region = $currentObject/Bm.AreaPicker_City`)), nil)
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "Bm.Region") || !strings.Contains(vs[0].Message, "Bm.City") {
		t.Fatalf("mismatch should name Bm.Region and Bm.City, got %+v", vs)
	}
	vs = comparandViolations(t, comparandDomain+comparandPage(comparandCombo(`Bm.District_Region = $currentObject/AreaPicker_Region`)), nil)
	if len(vs) != 1 || !strings.Contains(vs[0].Suggestion, "Bm.AreaPicker_Region") {
		t.Fatalf("unqualified should suggest Bm.AreaPicker_Region, got %+v", vs)
	}
}

// Silence wherever an entity cannot be established.
func TestDatasourceXPathComparand_SilentWhenUnknown(t *testing.T) {
	// No enclosing data container: $currentObject has no entity.
	top := `create page Bm.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  ` + comparandCombo(`Bm.District_Region = $currentObject/Bm.AreaPicker_City`) + `
};`
	if vs := comparandViolations(t, comparandDomain+top, nil); len(vs) != 0 {
		t.Errorf("no enclosing context: want silence, got %q", vs[0].Message)
	}
	// Entities the script does not declare and no project.
	other := strings.ReplaceAll(comparandPage(comparandCombo(`Bm.District_Region = $currentObject/Bm.AreaPicker_City`)), "Bm.", "Ext.")
	if vs := comparandViolations(t, other, nil); len(vs) != 0 {
		t.Errorf("undeclared entities: want silence, got %q", vs[0].Message)
	}
}

// The project answers for entities the script does not declare.
func TestDatasourceXPathComparand_FromProject(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-ext"}, Name: "Ext"}
	ent := func(id, name string) *domainmodel.Entity {
		return &domainmodel.Entity{BaseElement: model.BaseElement{ID: model.ID(id)}, Name: name,
			Attributes: []*domainmodel.Attribute{{Name: "Name", Type: &domainmodel.StringAttributeType{}}}}
	}
	assoc := func(name, from, to string) *domainmodel.Association {
		return &domainmodel.Association{BaseElement: model.BaseElement{ID: model.ID("a-" + name)}, Name: name,
			ParentID: model.ID(from), ChildID: model.ID(to)}
	}
	b := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: "dm-ext"}, ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{ent("e-r", "Region"), ent("e-d", "District"), ent("e-c", "City"), ent("e-p", "AreaPicker")},
				Associations: []*domainmodel.Association{
					assoc("District_Region", "e-d", "e-r"), assoc("AreaPicker_Region", "e-p", "e-r"),
					assoc("AreaPicker_District", "e-p", "e-d"), assoc("AreaPicker_City", "e-p", "e-c"),
				},
			}}, nil
		},
	}
	project := func() backend.FullBackend { return b }
	page := func(where string) string {
		return strings.ReplaceAll(comparandPage(comparandCombo(where)), "Bm.", "Ext.")
	}
	if vs := comparandViolations(t, page(`Ext.District_Region = $currentObject/Ext.AreaPicker_City`), project); len(vs) != 1 || vs[0].RuleID != "MDL-XPATH02" {
		t.Errorf("project-resolved mismatch: want one MDL-XPATH02, got %+v", vs)
	}
	if vs := comparandViolations(t, page(`Ext.District_Region = $currentObject/AreaPicker_Region`), project); len(vs) != 1 || vs[0].RuleID != "MDL-XPATH01" {
		t.Errorf("project-resolved unqualified: want one MDL-XPATH01, got %+v", vs)
	}
	if vs := comparandViolations(t, page(`Ext.District_Region = $currentObject/Ext.AreaPicker_Region`), project); len(vs) != 0 {
		t.Errorf("control: want silence, got %q", vs[0].Message)
	}
}
