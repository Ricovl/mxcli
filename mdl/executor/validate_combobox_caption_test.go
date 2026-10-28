// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1466: a combo box with an options data source and no caption
// passed `check` and `exec`, and mxbuild then reported
//
//	[error] [CE0642] "Property 'Caption' is required." at Combo box 'cb'
//
// The issue's own script had this on both of its combo boxes. Each row below
// was one page in a fresh `mxcli new` project, built with mxbuild 11.14.0 and
// 11.13.0 (identical results); "want" is whether that page was CE0642.
func TestMDLWIDGET16_ComboBoxCaptionRequired(t *testing.T) {
	cases := []struct {
		name  string
		props string
		want  bool
	}{
		// Association mode (the def selects it on a data source), no caption.
		{"database source, no caption", `Association: Bm.AreaPicker_District, DataSource: database Bm.District`, true},
		{"microflow source, no caption", `Association: Bm.AreaPicker_District, DataSource: microflow Bm.DS_Districts`, true},
		{"Attribute: spelling of the association, no caption", `Attribute: AreaPicker_District, DataSource: database Bm.District`, true},
		{"caption type expression, no expression", `Association: Bm.AreaPicker_District, DataSource: database Bm.District, optionsSourceAssociationCaptionType: expression`, true},
		{"caption type expression, only an attribute", `Association: Bm.AreaPicker_District, DataSource: database Bm.District, optionsSourceAssociationCaptionType: expression, CaptionAttribute: Name`, true},
		{"expression without the type", `Association: Bm.AreaPicker_District, DataSource: database Bm.District, optionsSourceAssociationCaptionExpression: '$currentObject/Name'`, true},
		{"caption type attribute, only an expression", `Association: Bm.AreaPicker_District, DataSource: database Bm.District, optionsSourceAssociationCaptionType: attribute, optionsSourceAssociationCaptionExpression: '$currentObject/Name'`, true},

		// Controls: each built at 0 errors.
		{"CaptionAttribute", `Association: Bm.AreaPicker_District, DataSource: database Bm.District, CaptionAttribute: Name`, false},
		{"named caption attribute key", `Association: Bm.AreaPicker_District, DataSource: database Bm.District, optionsSourceAssociationCaptionAttribute: Name`, false},
		{"caption expression", `Association: Bm.AreaPicker_District, DataSource: database Bm.District, optionsSourceAssociationCaptionType: expression, optionsSourceAssociationCaptionExpression: '$currentObject/Name'`, false},
		{"enumeration mode needs no caption", `Attribute: Kind`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := parseOneComboBox(t, c.props)
			var hits []string
			for _, v := range validateComboBoxAssociation(w, "page Bm.P") {
				if v.RuleID == "MDL-WIDGET16" {
					hits = append(hits, v.Message)
				}
			}
			if got := len(hits) > 0; got != c.want {
				t.Fatalf("MDL-WIDGET16 = %v, want %v (%q)", got, c.want, hits)
			}
			if c.want && !strings.Contains(hits[0], "CE0642") || c.want && !strings.Contains(hits[0], "Caption") {
				t.Errorf("message should cite CE0642 and the Caption property, got: %s", hits[0])
			}
		})
	}
}

func parseOneComboBox(t *testing.T, props string) *ast.WidgetV3 {
	t.Helper()
	src := `create page Bm.P (Params: ($Picker: Bm.AreaPicker), Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dataview dv (DataSource: $Picker) {
    combobox cb (` + props + `)
  }
};`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s := prog.Statements[0].(*ast.CreatePageStmtV3)
	var found *ast.WidgetV3
	var walk func(ws []*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if strings.EqualFold(w.Type, "combobox") {
				found = w
			}
			walk(w.Children)
		}
	}
	walk(allPageWidgets(s))
	if found == nil {
		t.Fatal("no combobox parsed")
	}
	return found
}

// mendixlabs/mxcli#1583: the database mode has the same two required parts,
// and one more. Each row was one combo box on a fresh 11.12.5 project (Combo
// box 2.5.0) built with mxbuild; "want" is the CE0642 it reported, "" for a
// combo box that built clean.
func TestMDLWIDGET16_ComboBoxDatabaseModeRequired(t *testing.T) {
	const ds = `optionsSourceDatabaseDataSource: database Bm.District`
	cases := []struct {
		name  string
		props string
		want  string // the required property mxbuild named
	}{
		{"no caption", ds + `, Attribute: Name, optionsSourceDatabaseValueAttribute: Name`, "Caption"},
		{"target without a value attribute", ds + `, Attribute: Name, CaptionAttribute: Name`, "Value"},
		{"target by key without a value attribute", ds + `, databaseAttributeString: Name, CaptionAttribute: Name`, "Value"},
		{"caption type expression, no expression", ds + `, optionsSourceDatabaseCaptionType: expression, CaptionAttribute: Name`, "Caption"},

		// Controls: each built at 0 errors.
		{"target, value and caption", ds + `, Attribute: Name, CaptionAttribute: Name, optionsSourceDatabaseValueAttribute: Name`, ""},
		{"caption by key", ds + `, Attribute: Name, optionsSourceDatabaseCaptionAttribute: Name, optionsSourceDatabaseValueAttribute: Name`, ""},
		{"no target needs no value attribute", ds + `, CaptionAttribute: Name`, ""},
		{"caption expression", ds + `, optionsSourceDatabaseCaptionType: expression, optionsSourceDatabaseCaptionExpression: '$currentObject/Name'`, ""},
		{"Multi hides the target and value", ds + `, optionsSourceDatabaseItemSelection: Multi, CaptionAttribute: Name`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var hits []string
			for _, v := range validateComboBoxAssociation(parseOneComboBox(t, c.props), "page Bm.P") {
				if v.RuleID == "MDL-WIDGET16" {
					hits = append(hits, v.Message)
				}
			}
			if c.want == "" {
				if len(hits) > 0 {
					t.Fatalf("built clean, but MDL-WIDGET16 fired: %q", hits)
				}
				return
			}
			if len(hits) != 1 {
				t.Fatalf("want one MDL-WIDGET16 naming %q, got %q", c.want, hits)
			}
			if want := "Property '" + c.want + "' is required."; !strings.Contains(hits[0], "CE0642") || !strings.Contains(hits[0], want) {
				t.Errorf("message should cite CE0642 and %q, got: %s", want, hits[0])
			}
		})
	}
}
