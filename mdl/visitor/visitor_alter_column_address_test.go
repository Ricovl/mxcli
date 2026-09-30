// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#749: a DataGrid 2 column is addressed explicitly, by what describe
// prints in it — `dg column(Name)`, `dg column('Total')` — because Mendix stores
// no column name for `dg.Name` to mean.
func TestAlterTarget_ColumnAddress(t *testing.T) {
	stmt := buildAlterPage(t, `alter page M.P {
		set (Caption: 'Login') on dg column(Name);
		drop dg column(FullName)@2, dg column('Total'), dg column(UserRoles/Name);
		insert after dg column("Status") { column (Attribute: Code) }
		replace dg column(Name) with { column (Attribute: Name, Caption: 'User') }
	};`)
	set := stmt.Operations[0].(*ast.SetPropertyOp)
	if want := (ast.WidgetRef{Widget: "dg", ColumnAttribute: "Name"}); set.Target != want {
		t.Errorf("set target = %+v, want %+v", set.Target, want)
	}
	drop := stmt.Operations[1].(*ast.DropWidgetOp)
	want := []ast.WidgetRef{
		{Widget: "dg", ColumnAttribute: "FullName", Ordinal: 2},
		{Widget: "dg", ColumnCaption: "Total"},
		{Widget: "dg", ColumnAttribute: "UserRoles/Name"},
	}
	if len(drop.Targets) != len(want) {
		t.Fatalf("drop targets = %+v", drop.Targets)
	}
	for i := range want {
		if drop.Targets[i] != want[i] {
			t.Errorf("drop target %d = %+v, want %+v", i, drop.Targets[i], want[i])
		}
	}
	if got := stmt.Operations[2].(*ast.InsertWidgetOp).Target; got != (ast.WidgetRef{Widget: "dg", ColumnAttribute: "Status"}) {
		t.Errorf("insert target = %+v (a quoted identifier is an attribute, not a caption)", got)
	}
	if got := stmt.Operations[3].(*ast.ReplaceWidgetOp).Target; !got.IsColumn() || got.Name() != "dg column(Name)" {
		t.Errorf("replace target = %+v, name %q", got, got.Name())
	}
	for _, r := range drop.Targets {
		if !r.IsColumn() {
			t.Errorf("%+v: IsColumn() = false", r)
		}
	}
	if got := drop.Targets[0].Name(); got != "dg column(FullName)@2" {
		t.Errorf("Name() = %q", got)
	}
	if got := drop.Targets[1].Name(); got != "dg column('Total')" {
		t.Errorf("Name() = %q", got)
	}
}
