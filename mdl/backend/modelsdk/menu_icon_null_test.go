// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// Every Menus$MenuItem Studio Pro writes carries `Icon: null` when it has no
// icon (ako/TestApp, PedApp, Fixture1115: the key set is $ID, $Type, Action,
// AlternativeText, Caption, Icon, Items on every item). `create menu` wrote
// its items through the codec, which drops a null element property unless the
// type registers it, so an iconless item had no Icon key at all.
//
// It was invisible until an item reached the same document by another path:
// an ALTER MENU insert writes Studio Pro's shape, and the next describe ->
// `create or modify menu` then reported "Modified" for a menu nothing had
// changed, because the rebuild dropped the key the splice had written.
func TestCreateMenuDocument_ItemsCarryANullIcon(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("module: %v", err)
	}
	md := &types.MenuDocument{Name: "IconNull", ContainerID: mod.ID, Items: []*types.NavMenuItem{
		{Caption: "Plain", Page: "MyFirstModule.Home_Web"},
		{Caption: "Group", Items: []*types.NavMenuItem{{Caption: "Child", Page: "MyFirstModule.Home_Web"}}},
	}}
	if err := b.CreateMenuDocument(md); err != nil {
		t.Fatal(err)
	}
	doc, err := b.rawUnitDoc(md.ID)
	if err != nil {
		t.Fatal(err)
	}
	items := 0
	var walk func(v any)
	walk = func(v any) {
		switch d := v.(type) {
		case bson.D:
			if navGetString(d, "$Type") == "Menus$MenuItem" {
				items++
				has := false
				for _, e := range d {
					if e.Key == "Icon" {
						has = true
						if e.Value != nil {
							t.Errorf("an iconless item stores Icon %v, want null", e.Value)
						}
					}
				}
				if !has {
					t.Errorf("menu item has no Icon key; Studio Pro writes Icon: null")
				}
			}
			for _, e := range d {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range d {
				walk(e)
			}
		}
	}
	walk(doc)
	if items != 3 {
		t.Fatalf("found %d menu items, want 3", items)
	}
}
