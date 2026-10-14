// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestDatasourceXPathVariableTraversal covers the page/widget-datasource arm of
// MDL055 (mendixlabs/mxcli#1466). The microflow-retrieve rule (#831) never saw a
// widget's `database … where […]`, so a constraint reaching two hops off
// `$currentObject` passed `mxcli check` and `exec` and the build failed CE0161.
//
// Measured on fresh 11.13.0 and 11.14.0 projects, inside a data view over the
// helper entity — the boundary is the same as for a retrieve:
//
//	[Assoc = $currentObject/Mod.Assoc]        0 errors  combobox, list view, data grid 2
//	[Mod.A/Mod.B/Mod.C = $currentObject]      0 errors  combobox (the inverted form)
//	[Name = $currentObject/Label]             0 errors  combobox
//	[Name = $currentObject/Mod.Assoc/Name]    CE0161    combobox, list view, data grid 2, gallery
//
// The one-hop form the issue reported is valid; flagging it would refuse MDL
// that builds, so the controls below are as load-bearing as the flagged cases.
func TestDatasourceXPathVariableTraversal(t *testing.T) {
	const twoHop = `[Name = $currentObject/MyModule.AreaPicker_Region/Name]`
	const oneHop = `[MyModule.District_Region = $currentObject/MyModule.AreaPicker_Region]`
	const inverted = `[MyModule.District_Region/MyModule.Region/MyModule.AreaPicker_Region = $currentObject]`
	const ownAttr = `[Name = $currentObject/Label]`

	combobox := func(where string) string {
		return `combobox cbDistrict (Association: MyModule.AreaPicker_District,
      DataSource: database MyModule.District where ` + where + `, CaptionAttribute: Name)`
	}
	listview := func(where string) string {
		return `listview lv (DataSource: database from MyModule.District where ` + where + `) {
      dynamictext t (Content: '{1}', ContentParams: ({1} = Name))
    }`
	}
	datagrid := func(where string) string {
		return `datagrid dg (DataSource: database from MyModule.District where ` + where + `) {
      column (Attribute: Name, Caption: 'Name')
    }`
	}
	gallery := func(where string) string {
		return `gallery gal (DataSource: database from MyModule.District where ` + where + `) {
      template { dynamictext t (Content: '{1}', ContentParams: ({1} = Name)) }
    }`
	}

	cases := []struct {
		name   string
		widget string
		flag   bool
	}{
		{"combobox two hops", combobox(twoHop), true},
		{"list view two hops", listview(twoHop), true},
		{"data grid 2 two hops", datagrid(twoHop), true},
		{"gallery two hops", gallery(twoHop), true},
		// Controls: each built at 0 errors.
		{"combobox one hop — the reported form", combobox(oneHop), false},
		{"list view one hop", listview(oneHop), false},
		{"data grid 2 one hop", datagrid(oneHop), false},
		{"combobox inverted form", combobox(inverted), false},
		{"combobox own attribute", combobox(ownAttr), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := `create page MyModule.P (
  Params: ($Picker: MyModule.AreaPicker), Title: 'Area', Layout: Atlas_Core.Atlas_Default
) {
  dataview dvPicker (DataSource: $Picker) {
    ` + c.widget + `
  }
};`
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			got := false
			var msg string
			for _, v := range ValidateWidgetProperties(prog, "") {
				if v.RuleID == "MDL055" {
					got = true
					msg = v.Message + " / " + v.Suggestion
				}
			}
			if got != c.flag {
				t.Fatalf("MDL055 fired = %v, want %v\n  message: %s", got, c.flag, msg)
			}
			if got && !strings.Contains(msg, "$currentObject/MyModule.AreaPicker_Region/Name") {
				t.Errorf("message does not name the offending path: %s", msg)
			}
		})
	}
}
