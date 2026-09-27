// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func pageBody(t *testing.T, body string) []*ast.WidgetV3 {
	t.Helper()
	prog, errs := visitor.Build(`create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { ` + body + ` };`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog.Statements[0].(*ast.CreatePageStmtV3).Widgets
}

// ako/mxcli#749: the name is optional in the grammar, so `check` says up front
// — no project needed — when a widget that Mendix stores a name for has none.
func TestCheckMissingWidgetNames(t *testing.T) {
	refused := map[string]string{
		`textbox (Attribute: Name)`:                                                            "textbox",
		`container { dynamictext t (Content: 'x') }`:                                           "container",
		`layoutgrid { row { column (DesktopWidth: 12) { } } }`:                                 "layoutgrid",
		`row { column (DesktopWidth: 12) { } }`:                                                "row", // a container; its column is a grid column
		`container c { column (DesktopWidth: 12) { } }`:                                        "column",
		`container c { row { } }`:                                                              "row",
		`layoutgrid lg { row { column (DesktopWidth: 12) { actionbutton (Caption: 'Go') } } }`: "actionbutton",
	}
	for body, kind := range refused {
		errs := checkMissingWidgetNames(pageBody(t, body))
		if len(errs) != 1 || !strings.Contains(errs[0], kind+" needs a name") {
			t.Errorf("%s: got %v, want one '%s needs a name'", body, errs, kind)
		}
	}
	for _, body := range []string{
		`layoutgrid lg { row { column (DesktopWidth: 12) { dynamictext t (Content: 'x') } } }`,
		`datagrid dg (DataSource: database from M.E) { controlbar { actionbutton b (Caption: 'x') } column (Attribute: Name) }`,
		`gallery g (DataSource: database from M.E) { filter { textfilter tf } template { dynamictext t (Content: 'x') } }`,
		`pluggablewidget 'com.mendix.widget.web.accordion.Accordion' acc { group (header: 'One') { } }`,
		`listview lv (DataSource: database from M.E) { template for M.Sub { dynamictext t (Content: 'x') } }`,
	} {
		if errs := checkMissingWidgetNames(pageBody(t, body)); len(errs) != 0 {
			t.Errorf("%s: refused %v", body, errs)
		}
	}
}

// `check` reports it with no project: the rule runs in the widget-tree pass.
func TestCheckMissingWidgetNames_ReportedByCheck(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range validateWidgetTree(pageBody(t, `textbox (Attribute: Name)`), reg, "page M.P") {
		got = append(got, v.RuleID)
	}
	if len(got) == 0 || got[len(got)-1] != "MDL-WIDGET35" {
		t.Errorf("validateWidgetTree reported %v, want MDL-WIDGET35", got)
	}
}
