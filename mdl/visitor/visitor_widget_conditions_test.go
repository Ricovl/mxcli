// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): a conditional Visible / Editable is a bare expression,
// stored as written. `Visible: [expr]` is the deprecated alias MDL-DEPR081.

func textboxProps(t *testing.T, props string) (map[string]any, *ast.Program) {
	t.Helper()
	prog := mustBuild(t, "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $E) { textbox t (Attribute: Name, "+props+") } };")
	return prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0].Children[0].Properties, prog
}

func TestWidgetCondition_BareIsCanonical(t *testing.T) {
	for _, tc := range []struct{ bracketed, bare string }{
		{"Visible: [Active]", "Visible: $currentObject/Active"},
		{"Visible: [$currentObject/Price > 3 and Active]", "Visible: $currentObject/Price > 3 and $currentObject/Active"},
		{"Editable: [$currentObject/Status = 'Open']", "Editable: $currentObject/Status = 'Open'"},
		{"Visible: [not($currentObject/Active)]", "Visible: not($currentObject/Active)"},
	} {
		old, oldProg := textboxProps(t, tc.bracketed)
		canon, canonProg := textboxProps(t, tc.bare)
		if !reflect.DeepEqual(old, canon) {
			t.Errorf("%s built %#v\n%s built %#v", tc.bracketed, old, tc.bare, canon)
		}
		if got := deprecationCodes(oldProg); !reflect.DeepEqual(got, []string{deprecation.BracketedWidgetCondition}) {
			t.Errorf("%s recorded %v, want [%s]", tc.bracketed, got, deprecation.BracketedWidgetCondition)
		}
		if got := deprecationCodes(canonProg); len(got) != 0 {
			t.Errorf("%s recorded %v, want none", tc.bare, got)
		}
	}
}

// The bare form stores the expression as written: nothing is rooted, spacing
// is kept.
func TestWidgetCondition_BareIsStoredAsWritten(t *testing.T) {
	props, _ := textboxProps(t, "Visible: $currentObject/Status  =  'Open' or $Flag")
	if got := props["VisibleIf"]; got != "$currentObject/Status  =  'Open' or $Flag" {
		t.Errorf("VisibleIf = %#v", got)
	}
}

// A plain value keeps its old reading: it is not a conditional expression.
func TestWidgetCondition_PlainValuesUnchanged(t *testing.T) {
	for _, tc := range []struct {
		props, key string
		want       any
	}{
		{"Visible: false", "Visible", false},
		{"Visible: Active", "Visible", "Active"},
		{"Editable: Never", "Editable", "Never"},
	} {
		props, prog := textboxProps(t, tc.props)
		if props[tc.key] != tc.want || props["VisibleIf"] != nil || props["EditableIf"] != nil {
			t.Errorf("%s built %#v", tc.props, props)
		}
		if got := deprecationCodes(prog); len(got) != 0 {
			t.Errorf("%s recorded %v", tc.props, got)
		}
	}
}

func alterSet(t *testing.T, set string) (map[string]any, *ast.Program) {
	t.Helper()
	prog := mustBuild(t, "alter page M.P { set ("+set+") on t };")
	return prog.Statements[0].(*ast.AlterPageStmt).Operations[0].(*ast.SetPropertyOp).Properties, prog
}

func TestWidgetCondition_AlterPageSet(t *testing.T) {
	old, oldProg := alterSet(t, "Visible: [Active]")
	canon, canonProg := alterSet(t, "Visible: $currentObject/Active")
	if !reflect.DeepEqual(old, canon) || canon["VisibleIf"] != "$currentObject/Active" {
		t.Errorf("bracketed built %#v, bare %#v", old, canon)
	}
	if got := deprecationCodes(oldProg); !reflect.DeepEqual(got, []string{deprecation.BracketedWidgetCondition}) {
		t.Errorf("bracketed recorded %v", got)
	}
	if got := deprecationCodes(canonProg); len(got) != 0 {
		t.Errorf("bare recorded %v", got)
	}
	if p, _ := alterSet(t, "Editable: $currentObject/Status != 'Closed'"); p["EditableIf"] != "$currentObject/Status != 'Closed'" {
		t.Errorf("Editable built %#v", p)
	}
	if p, _ := alterSet(t, "Visible: false"); p["Visible"] != false {
		t.Errorf("Visible: false built %#v", p)
	}
}

func TestBareWidgetCondition(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"$currentObject/Active", true},
		{"$currentObject/Status = 'Open'", true},
		{"true", false},             // the plain value
		{"Active", false},           // a plain name
		{"Status in (A, B)", false}, // `based on attribute value`
		{"", false},
	} {
		if got := BareWidgetCondition("Visible", tc.expr); got != tc.want {
			t.Errorf("BareWidgetCondition(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}
