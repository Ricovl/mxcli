// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#826 on the MCP route: an input or a template parameter read through
// a data view sends pg_patch_page a sourceVariable naming the widget. Given
// {widget: dataView1}, Studio Pro 11.14 stores {widget: dataView1,
// pageParameter: Car} itself — measured through this server.
func TestMCPWidgetScopedSourceVariable(t *testing.T) {
	w := inputWidget("Pages$TextBox", "tb", "M", "M.Car.Model", "", "",
		&pages.WidgetVariable{Widget: "dataView1", Variable: "Car"})
	sv, ok := w["sourceVariable"].(map[string]any)
	if !ok || sv["widget"] != "dataView1" || sv["pageParameter"] != "Car" {
		t.Errorf("input sourceVariable = %#v, want widget dataView1 + pageParameter Car", w["sourceVariable"])
	}
	if _, has := inputWidget("Pages$TextBox", "tb", "M", "M.Car.Model", "", "", nil)["sourceVariable"]; has {
		t.Error("control: an input bound to its context sent a sourceVariable")
	}
	p := clientTemplateParam(&pages.ClientTemplateParameter{AttributeRef: "M.Car.Brand", SourceWidget: "dataView1", SourceVariable: "Car"})
	sv, ok = p["sourceVariable"].(map[string]any)
	if !ok || sv["widget"] != "dataView1" {
		t.Errorf("template parameter sourceVariable = %#v, want widget dataView1", p["sourceVariable"])
	}
}
