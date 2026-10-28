// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1677: "`Forms$DynamicText.NativeTextStyle: Heading3` becomes `Text`.
// The heading on the native home and login pages becomes body text." Describe
// printed `dynamictext text4 (Content: 'JTS boat log')`, and MDL had no spelling
// for the property: the writer stored Text on every dynamic text.
//
// NativeTextStyle is the native counterpart of RenderMode (Model SDK:
// Forms$DynamicText `nativeTextStyle` Introduced 8.0.0, enum Text and
// Heading1–Heading6).
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func describeStoredDynamicText(t *testing.T, style string) string {
	t.Helper()
	ctx, buf := newMockCtx(t)
	stored := map[string]any{"$Type": "Forms$DynamicText", "Name": "text4", "RenderMode": "Text"}
	if style != "" {
		stored["NativeTextStyle"] = style
	}
	for _, rw := range parseRawWidget(ctx, stored) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	return buf.String()
}

func TestDescribeDynamicText_EmitsNativeTextStyle(t *testing.T) {
	if out := describeStoredDynamicText(t, "Heading3"); !strings.Contains(out, "NativeTextStyle: Heading3") {
		t.Errorf("describe dropped NativeTextStyle Heading3, so re-executing it turns the heading into body text. got:\n%s", out)
	}
}

// Control: Text is what every dynamic text stores (284 of 284 in PedApp), so
// printing it would put a clause in every description.
func TestDescribeDynamicText_OmitsDefaultNativeTextStyle(t *testing.T) {
	for _, style := range []string{"", "Text"} {
		if out := describeStoredDynamicText(t, style); strings.Contains(out, "NativeTextStyle") {
			t.Errorf("describe printed NativeTextStyle for %q: %s", style, out)
		}
	}
}

func buildDynamicTextFromMDL(t *testing.T, props string) (*pages.DynamicText, error) {
	t.Helper()
	prog, errs := visitor.Build("create page Mod.P (Title: 'T') {\n  dynamictext text4 (" + props + ")\n}")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	w, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		return nil, err
	}
	return w.(*pages.DynamicText), nil
}

func TestBuildDynamicText_NativeTextStyle(t *testing.T) {
	for _, tc := range []struct{ written, want string }{
		{"Heading3", "Heading3"},
		{"heading6", "Heading6"}, // matched case-insensitively, stored canonically
		{"Text", "Text"},
	} {
		dt, err := buildDynamicTextFromMDL(t, "Content: 'JTS boat log', NativeTextStyle: "+tc.written)
		if err != nil {
			t.Fatalf("NativeTextStyle: %s: %v", tc.written, err)
		}
		if string(dt.NativeTextStyle) != tc.want {
			t.Errorf("NativeTextStyle: %s built %q, want %q", tc.written, dt.NativeTextStyle, tc.want)
		}
	}
	// Control: unwritten stays unauthored, so the writer keeps its Text default.
	dt, err := buildDynamicTextFromMDL(t, "Content: 'x'")
	if err != nil {
		t.Fatal(err)
	}
	if dt.NativeTextStyle != "" {
		t.Errorf("NativeTextStyle = %q for a text that declared none", dt.NativeTextStyle)
	}
}

// An unknown member is a document Studio Pro cannot open; refuse it.
func TestBuildDynamicText_NativeTextStyleRejectsUnknown(t *testing.T) {
	_, err := buildDynamicTextFromMDL(t, "Content: 'x', NativeTextStyle: H3")
	if err == nil || !strings.Contains(err.Error(), "Heading3") {
		t.Errorf("NativeTextStyle: H3 = %v, want an error listing the valid styles", err)
	}
}

// check must not warn that the property it now writes "will be silently dropped".
func TestValidateDynamicText_NativeTextStyleIsKnown(t *testing.T) {
	if !isKnownStaticWidgetProp("NativeTextStyle") {
		t.Error("NativeTextStyle is not in the built-in widget vocabulary, so check warns MDL-WIDGET07 that it is dropped")
	}
}
