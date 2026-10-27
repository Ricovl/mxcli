// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// describe fragment printed `call microflow M.F` for `call microflow M.F(Order
// = $currentObject)` — every flow and page argument dropped, as were a flow
// data source's arguments, what a create object opens, an open link's address,
// a fragment action parameter and the fragment's own parameters. The fragment
// it described was not the one `use fragment` expands. Its output must parse
// back to the same fragment.
const fragmentWithArgs = `create fragment OrderTools ($data: datasource, $onSave: action) as {
  dataview dvOrder (DataSource: microflow Shop.DS_Order(Customer = $Customer, Limit = 10)) {
    actionbutton btnDelete (Caption: 'Delete it''s order', Action: call microflow Shop.ACT_Order_Delete(Order = $currentObject) with (ProgressBar: Blocking))
    actionbutton btnCheck (Caption: 'Check', Action: call nanoflow Shop.NF_Order_Check(Order = $currentObject, Strict = true) returns ($result = $ActionReturnValue))
    actionbutton btnOpen (Caption: 'Open', Action: show page Shop.Order_Edit(Order = $currentObject))
    actionbutton btnNew (Caption: 'New', Action: create object Shop.Line then show page Shop.Line_Edit(Line = $currentObject))
    actionbutton btnWeb (Caption: 'Web', Action: open link 'https://example.com/?q=a''b')
    actionbutton btnUrl (Caption: 'Url', Action: open link $currentObject/Website)
    actionbutton btnSave (Caption: 'Save', Action: $onSave)
  }
  listview lvLines (DataSource: nanoflow Shop.NF_Lines(Order = $Order)) {
    dynamictext txt (Content: 'x')
  }
};`

func fragmentDescribedAndReparsed(t *testing.T, src string) (*ast.DefineFragmentStmt, *ast.DefineFragmentStmt, string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse the fragment: %v", errs)
	}
	frag := prog.Statements[0].(*ast.DefineFragmentStmt)
	ctx, buf := newMockCtx(t)
	assertNoError(t, execDefineFragment(ctx, frag))
	buf.Reset()
	assertNoError(t, describeFragment(ctx, ast.QualifiedName{Name: frag.Name}))
	described := buf.String()
	again, errs := visitor.Build(described)
	if len(errs) > 0 {
		t.Fatalf("describe fragment printed MDL that does not parse: %v\n%s", errs, described)
	}
	return frag, again.Statements[0].(*ast.DefineFragmentStmt), described
}

func TestDescribeFragmentPrintsArguments(t *testing.T) {
	frag, again, described := fragmentDescribedAndReparsed(t, fragmentWithArgs)
	for _, want := range []string{
		"microflow Shop.DS_Order(Customer = $Customer, Limit = 10)",
		"call microflow Shop.ACT_Order_Delete(Order = $currentObject) with (ProgressBar: Blocking)",
		"call nanoflow Shop.NF_Order_Check(Order = $currentObject, Strict = true) returns ($result = $ActionReturnValue)",
		"show page Shop.Order_Edit(Order = $currentObject)",
		"nanoflow Shop.NF_Lines(Order = $Order)",
	} {
		if !strings.Contains(described, want) {
			t.Errorf("describe fragment does not print %q:\n%s", want, described)
		}
	}
	if !reflect.DeepEqual(again.Params, frag.Params) {
		t.Errorf("parameters: described %+v, defined %+v", again.Params, frag.Params)
	}
	var walk func(a, b []*ast.WidgetV3)
	walk = func(a, b []*ast.WidgetV3) {
		if len(a) != len(b) {
			t.Fatalf("described %d widgets, defined %d:\n%s", len(b), len(a), described)
		}
		for i := range a {
			if !reflect.DeepEqual(a[i].Properties, b[i].Properties) {
				t.Errorf("%s: described\n  %#v\ndefined\n  %#v", a[i].Name, b[i].Properties, a[i].Properties)
			}
			walk(a[i].Children, b[i].Children)
		}
	}
	walk(frag.Widgets, again.Widgets)
}

// Control: a fragment with nothing to drop described the same before.
func TestDescribeFragmentPlainActionsUnchanged(t *testing.T) {
	_, _, described := fragmentDescribedAndReparsed(t, `create fragment Plain as {
  actionbutton btnSave (Caption: 'Save', Action: save changes close page)
  actionbutton btnGo (Caption: 'Go', Action: call microflow Shop.ACT_Go)
};`)
	for _, want := range []string{"create fragment Plain as {", "Action: save changes close page", "Action: call microflow Shop.ACT_Go,"} {
		if !strings.Contains(described, want) {
			t.Errorf("describe fragment does not print %q:\n%s", want, described)
		}
	}
}
