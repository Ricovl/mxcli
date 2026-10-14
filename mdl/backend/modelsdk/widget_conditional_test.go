// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func encodeWidget(t *testing.T, w pages.Widget) bsonv1.D {
	t.Helper()
	el, err := widgetToGen(w)
	if err != nil {
		t.Fatalf("widgetToGen: %v", err)
	}
	out, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var doc bsonv1.D
	if err := bsonv1.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

func docGet(doc bsonv1.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// Issue #627 — the codec used to hardcode ConditionalVisibilitySettings to null
// (silently dropping the expression). A container that carries one must now
// serialize the settings node with the expression.
func TestWidgetConditionalVisibility_Serialized(t *testing.T) {
	ctn := &pages.Container{}
	ctn.Name = "ctn"
	ctn.ConditionalVisibility = &pages.ConditionalVisibilitySettings{
		BaseElement: model.BaseElement{TypeName: "Forms$ConditionalVisibilitySettings"},
		Expression:  "$currentObject/Name != ''",
	}

	doc := encodeWidget(t, ctn)
	cvs, ok := docGet(doc, "ConditionalVisibilitySettings").(bsonv1.D)
	if !ok {
		t.Fatalf("ConditionalVisibilitySettings not serialized (got %T)", docGet(doc, "ConditionalVisibilitySettings"))
	}
	if got := docGet(cvs, "Expression"); got != "$currentObject/Name != ''" {
		t.Errorf("Expression = %v, want $currentObject/Name != ''", got)
	}
	// Studio Pro's sub-field set must be present (via TypeDefaults).
	for _, k := range []string{"Conditions", "ModuleRoles", "Attribute", "SourceVariable", "IgnoreSecurity"} {
		found := false
		for _, e := range cvs {
			if e.Key == k {
				found = true
			}
		}
		if !found {
			t.Errorf("CVS missing sub-field %q", k)
		}
	}
}

// A widget without a conditional setting still emits the null slot (unchanged).
func TestWidgetConditionalVisibility_NullWhenUnset(t *testing.T) {
	ctn := &pages.Container{}
	ctn.Name = "ctn"
	doc := encodeWidget(t, ctn)
	if v := docGet(doc, "ConditionalVisibilitySettings"); v != nil {
		t.Errorf("expected null ConditionalVisibilitySettings when unset, got %T", v)
	}
}

// A navigation-list item's visibility is written like a widget's (#1460); with
// none it stays the null Studio Pro stores (ako/mxcli#950).
func TestNavigationListItemConditionalVisibility_ModuleRoles(t *testing.T) {
	item := &pages.NavigationListItem{Action: &pages.NoClientAction{},
		ConditionalVisibility: &pages.ConditionalVisibilitySettings{
			BaseElement: model.BaseElement{TypeName: "Forms$ConditionalVisibilitySettings"},
			ModuleRoles: []string{"M.Admin"},
		}}
	el, err := navListItemToGen(item)
	if err != nil {
		t.Fatal(err)
	}
	cvs, ok := encodeToMap(t, el)["ConditionalVisibilitySettings"].(map[string]any)
	if !ok {
		t.Fatal("item ConditionalVisibilitySettings not written — the role restriction was dropped")
	}
	if roles, _ := cvs["ModuleRoles"].(bsonv1.A); len(roles) != 2 || roles[1] != "M.Admin" {
		t.Errorf("ModuleRoles = %v, want [1 M.Admin]", cvs["ModuleRoles"])
	}
	el, err = navListItemToGen(&pages.NavigationListItem{Action: &pages.NoClientAction{}})
	if err != nil {
		t.Fatal(err)
	}
	if v := encodeToMap(t, el)["ConditionalVisibilitySettings"]; v != nil {
		t.Errorf("an item without visibility must store null, got %v", v)
	}
}

// mendixlabs/mxcli#1460: role-based visibility is ConditionalVisibilitySettings.
// ModuleRoles, a by-name list — [1, "WorkflowCommons.Administrator"] on TestApp's
// Studio Pro-authored WorkflowCommons snippets, beside an expression or alone.
// The writer emitted the list empty whatever the model held.
func TestWidgetConditionalVisibility_ModuleRoles(t *testing.T) {
	ctn := &pages.Container{}
	ctn.Name = "ctn"
	ctn.ConditionalVisibility = &pages.ConditionalVisibilitySettings{
		BaseElement: model.BaseElement{TypeName: "Forms$ConditionalVisibilitySettings"},
		Expression:  "$currentObject/Name != ''",
		ModuleRoles: []string{"M.Admin", "M.User"},
	}
	cvs, ok := docGet(encodeWidget(t, ctn), "ConditionalVisibilitySettings").(bsonv1.D)
	if !ok {
		t.Fatal("ConditionalVisibilitySettings not serialized")
	}
	roles, _ := docGet(cvs, "ModuleRoles").(bsonv1.A)
	want := bsonv1.A{int32(1), "M.Admin", "M.User"}
	if len(roles) != len(want) {
		t.Fatalf("ModuleRoles = %v, want %v", roles, want)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Errorf("ModuleRoles[%d] = %v (%T), want %v", i, roles[i], roles[i], want[i])
		}
	}
	if got := docGet(cvs, "Expression"); got != "$currentObject/Name != ''" {
		t.Errorf("Expression = %v — the role list must not displace it", got)
	}
}
