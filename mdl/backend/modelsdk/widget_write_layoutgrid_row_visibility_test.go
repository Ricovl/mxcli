// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1461: a layout-grid row's visibility condition is stored in
// Forms$LayoutGridRow.ConditionalVisibilitySettings. layoutGridRowToGen never
// set it, so the registered null default was written for every row — a
// describe → exec (or create or replace) of a Studio Pro page whose rows were
// conditional made each of them always visible. Control: a row without a
// condition keeps Studio Pro's null.
func TestLayoutGridRow_WritesConditionalVisibility(t *testing.T) {
	lg := &pages.LayoutGrid{
		BaseWidget: pages.BaseWidget{Name: "lg"},
		Rows: []*pages.LayoutGridRow{
			{},
			{ConditionalVisibility: &pages.ConditionalVisibilitySettings{Expression: "$currentObject/Outcome != empty"}},
		},
	}
	el, err := widgetToGen(lg)
	if err != nil {
		t.Fatal(err)
	}
	rows := subDocs(t, encodeElement(t, el), "Rows")
	field := func(doc bsonv1.D, key string) (any, bool) {
		for _, e := range doc {
			if e.Key == key {
				return e.Value, true
			}
		}
		return nil, false
	}
	if v, ok := field(rows[0], "ConditionalVisibilitySettings"); !ok || v != nil {
		t.Errorf("row 1: ConditionalVisibilitySettings = %v (present %v), want Studio Pro's null", v, ok)
	}
	v, _ := field(rows[1], "ConditionalVisibilitySettings")
	cvs, ok := v.(bsonv1.D)
	if !ok {
		t.Fatalf("row 2: ConditionalVisibilitySettings = %v, want the settings — the condition was dropped (row always visible)", v)
	}
	if typ, _ := field(cvs, "$Type"); typ != "Forms$ConditionalVisibilitySettings" {
		t.Errorf("row 2: settings $Type = %v", typ)
	}
	if expr, _ := field(cvs, "Expression"); expr != "$currentObject/Outcome != empty" {
		t.Errorf("row 2: Expression = %v", expr)
	}
}
