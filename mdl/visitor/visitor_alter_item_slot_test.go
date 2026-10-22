// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// PROPOSAL_alter_object_list_item_address.md: an item address takes a slot,
// `acc1 item@1.headercontent`, carried as the target's Column exactly as
// `grid.controlbar` carries a widget's slot. The slot must not be read as the
// selector's widget — `item@1.headercontent` has two identifiers, the shape
// `item(t1)` used to be recognised by.
func TestAlterTarget_ItemSlot(t *testing.T) {
	stmt := buildAlterPage(t, `alter page M.P {
		insert into acc1 item@1.headercontent { dynamictext t1 (Content: 'x') };
		insert into acc1 item(btnIn).headercontent { dynamictext t2 (Content: 'x') };
		insert into acc1 item('One')@2.headercontent { dynamictext t3 (Content: 'x') };
		insert into acc1 item@1 { dynamictext t4 (Content: 'x') };
	};`)
	check := func(label string, got ast.WidgetRef, want ast.NavItemRef, spelled, slot string) {
		t.Helper()
		if got.Item == nil || *got.Item != want {
			t.Errorf("%s: Item = %+v, want %+v", label, got.Item, want)
		}
		if got.Widget != spelled || got.Column != slot || got.Ordinal != 0 {
			t.Errorf("%s: Widget %q Column %q Ordinal %d, want %q, %q, 0", label, got.Widget, got.Column, got.Ordinal, spelled, slot)
		}
	}
	ins := func(i int) ast.WidgetRef { return stmt.Operations[i].(*ast.InsertWidgetOp).Target }
	check("item@1.slot", ins(0), ast.NavItemRef{List: "acc1", Ordinal: 1}, "acc1 item@1", "headercontent")
	check("item(w).slot", ins(1), ast.NavItemRef{List: "acc1", Widget: "btnIn"}, "acc1 item(btnIn)", "headercontent")
	check("item('t')@2.slot", ins(2), ast.NavItemRef{List: "acc1", Text: "One", Ordinal: 2}, "acc1 item('One')@2", "headercontent")
	check("bare item", ins(3), ast.NavItemRef{List: "acc1", Ordinal: 1}, "acc1 item@1", "")
	if got := ins(0).Name(); got != "acc1 item@1.headercontent" {
		t.Errorf("Name() = %q, want the address as written", got)
	}
}
