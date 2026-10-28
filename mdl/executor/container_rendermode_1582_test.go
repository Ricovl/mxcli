// SPDX-License-Identifier: Apache-2.0

// A container's RenderMode (mendixlabs/mxcli#1582).
//
// Reported: "`create or modify layout` accepts `RenderMode` on a container,
// and `check` and `exec` both pass, but the value isn't stored. The container
// keeps the default render mode (`div`)." — read from `describe layout`, which
// printed `container ctnMain (Class: 'app-main') {`.
//
// The write was never the problem: `bson dump` shows `RenderMode: "Main"` on
// the stored Forms$DivContainer. DESCRIBE never read it, so the value looked
// dropped and a describe → exec round trip did drop it. The builder also
// wrote any string verbatim, so `RenderMode: 'main'` stored a member that is
// not in the enumeration.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func buildContainerFromMDL(t *testing.T, container string) (*pages.Container, error) {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n  " + container + "\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", container, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	widget, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		return nil, err
	}
	return widget.(*pages.Container), nil
}

func TestContainerRenderMode_DescribeEmitsIt(t *testing.T) {
	ctx, _ := newMockCtx(t)
	raw := map[string]any{
		"$Type":      "Forms$DivContainer",
		"Name":       "ctnMain",
		"RenderMode": "Main",
		"Appearance": map[string]any{"Class": "app-main"},
	}
	got := parseRawWidget(ctx, raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(got))
	}

	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, got[0], 0)
	if out := buf.String(); !strings.Contains(out, "RenderMode: Main") {
		t.Errorf("describe dropped the container's RenderMode:\n%s", out)
	}
}

// Control: the default render mode is not printed, so a plain container
// describes as it always has.
func TestContainerRenderMode_DescribeOmitsDefault(t *testing.T) {
	ctx, _ := newMockCtx(t)
	for _, rm := range []any{"Div", nil} {
		raw := map[string]any{"$Type": "Forms$DivContainer", "Name": "c"}
		if rm != nil {
			raw["RenderMode"] = rm
		}
		var buf bytes.Buffer
		outputWidgetMDLV3(&ExecContext{Output: &buf}, parseRawWidget(ctx, raw)[0], 0)
		if out := buf.String(); strings.Contains(out, "RenderMode") {
			t.Errorf("RenderMode %v: the default must not be printed:\n%s", rm, out)
		}
	}
}

// The describe output re-parses into the same container: the round trip the
// report's `describe` → `exec` depends on.
func TestContainerRenderMode_RoundTrip(t *testing.T) {
	for _, form := range []string{"RenderMode: Main", "RenderMode: 'Main'", "RenderMode: 'main'"} {
		c, err := buildContainerFromMDL(t, "container ctnMain ("+form+") { dynamictext t (Content: 'x') }")
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if c.RenderMode != "Main" {
			t.Errorf("%s: RenderMode = %q, want Main", form, c.RenderMode)
		}
	}
}

// An unknown member is refused before anything is written: the codec would
// store it verbatim, and a document with an unknown enum member is one Studio
// Pro cannot open.
func TestContainerRenderMode_UnknownRefused(t *testing.T) {
	_, err := buildContainerFromMDL(t, "container c (RenderMode: 'Banner') { dynamictext t (Content: 'x') }")
	if err == nil {
		t.Fatal("RenderMode 'Banner' must be refused")
	}
	for _, want := range []string{"Banner", "Main", "Nav"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name %q: %v", want, err)
		}
	}
}
