// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1676, the describe half: "`describe` prints `dataview dataView1
// (DataSource: microflow …, ShowFooter: true, ReadOnlyStyle: Text)`, without
// `Editable`".
//
// A data view stores its editability as `Editability`; input widgets store it as
// `Editable`. The data view case never read either, so a read-only data view
// described as an editable one.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func describeStoredDataView(t *testing.T, stored map[string]any) string {
	t.Helper()
	ctx, buf := newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, stored) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	return buf.String()
}

func TestDescribeDataView_EmitsEditability(t *testing.T) {
	out := describeStoredDataView(t, map[string]any{
		"$Type": "Forms$DataView", "Name": "dataView1",
		"Editability": "Never", "ReadOnlyStyle": "Text",
	})
	header := strings.SplitN(out, "\n", 2)[0]
	if !strings.Contains(header, "Editable: Never") {
		t.Errorf("describe dropped the data view's Editability: Never, so re-executing it "+
			"makes the data view editable. got:\n%s", out)
	}
}

// Control: Always is the default, so printing it would invent a clause the
// author never wrote.
func TestDescribeDataView_OmitsDefaultEditability(t *testing.T) {
	out := describeStoredDataView(t, map[string]any{
		"$Type": "Forms$DataView", "Name": "dataView1", "Editability": "Always",
	})
	if strings.Contains(out, "Editable") {
		t.Errorf("describe printed a default Editability: %s", out)
	}
}

// The MDL describe prints has to build back into a data view that carries the
// value, or the round trip still loses it at the next layer.
func TestBuildDataView_EditableNeverReachesTheWidget(t *testing.T) {
	prog, errs := visitor.Build("create page Mod.P (Title: 'T') {\n  dataview dataView1 (Editable: Never) {\n  }\n}")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	widget, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	dv, ok := widget.(*pages.DataView)
	if !ok {
		t.Fatalf("built %T, want *pages.DataView", widget)
	}
	if got := pages.WidgetEditability(&dv.BaseWidget); got != "Never" {
		t.Errorf("WidgetEditability = %q, want Never", got)
	}
}
