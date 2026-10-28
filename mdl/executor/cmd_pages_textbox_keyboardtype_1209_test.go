// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1209: "when a native child widget is rebuilt, its KeyboardType …
// is lost, for example on text boxes in the native snippets."
//
// Forms$TextBox.KeyboardType (Default, NumberPad, DecimalPad, EmailAddress,
// PhonePad, URL — mendixlabs mendixmodelsdk, declared from 8.0) had no MDL
// spelling: describe printed nothing and the writer wrote "Default", so a
// rewrite reset the keyboard a native text box opens with.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

func TestBuildTextBox_KeyboardType(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"unset stays unset (the writer writes Default)", nil, ""},
		{"email", "EmailAddress", "EmailAddress"},
		{"lowercase is canonicalised", "numberpad", "NumberPad"},
		{"URL", "url", "URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pb := &pageBuilder{widgetScope: map[string]model.ID{}}
			w := &ast.WidgetV3{Name: "txtEmail", Type: "textbox", Properties: map[string]any{}}
			if tc.value != nil {
				w.Properties["KeyboardType"] = tc.value
			}
			tb, err := pb.buildTextBoxV3(w)
			if err != nil {
				t.Fatalf("buildTextBoxV3: %v", err)
			}
			if tb.KeyboardType != tc.want {
				t.Errorf("KeyboardType = %q, want %q", tb.KeyboardType, tc.want)
			}
		})
	}
}

// An unknown member is refused: Studio Pro cannot load a document whose enum
// holds one, and mxbuild does not catch it.
func TestBuildTextBox_KeyboardTypeRejectsUnknownValue(t *testing.T) {
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	w := &ast.WidgetV3{Name: "txtEmail", Type: "textbox", Properties: map[string]any{"KeyboardType": "Email"}}
	_, err := pb.buildTextBoxV3(w)
	if err == nil || !strings.Contains(err.Error(), "EmailAddress") {
		t.Fatalf("an unknown KeyboardType must be refused naming the members, got %v", err)
	}
}

func TestDescribeTextBox_KeyboardType(t *testing.T) {
	ctx, buf := newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, map[string]any{
		"$Type": "Forms$TextBox", "Name": "txtEmail", "KeyboardType": "EmailAddress",
	}) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	out := buf.String()
	if !strings.Contains(out, "KeyboardType: EmailAddress") {
		t.Errorf("describe dropped the text box's KeyboardType, so a rewrite resets it to Default. got:\n%s", out)
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + out + "}\n"); len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, out)
	}

	// Control: Default is what the writer writes when unset, so it is not printed.
	ctx, buf = newMockCtx(t)
	for _, rw := range parseRawWidget(ctx, map[string]any{
		"$Type": "Forms$TextBox", "Name": "txtName", "KeyboardType": "Default",
	}) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	if strings.Contains(buf.String(), "KeyboardType") {
		t.Errorf("describe printed the default KeyboardType: %s", buf.String())
	}
}

// `mxcli check` reports what exec refuses, from the same function.
func TestCheckTextBox_KeyboardType(t *testing.T) {
	bad := &ast.WidgetV3{Name: "txtEmail", Type: "textbox", Properties: map[string]any{"KeyboardType": "Email"}}
	vs := validateInputSubmitAndGridWidth(bad, "page M.P")
	if len(vs) != 1 || vs[0].RuleID != "MDL-WIDGET18" || !strings.Contains(vs[0].Message, "EmailAddress") {
		t.Errorf("an unknown KeyboardType must be MDL-WIDGET18 naming the members, got %+v", vs)
	}
	good := &ast.WidgetV3{Name: "txtEmail", Type: "textbox", Properties: map[string]any{"KeyboardType": "emailaddress"}}
	if vs := validateInputSubmitAndGridWidth(good, "page M.P"); len(vs) != 0 {
		t.Errorf("a valid KeyboardType was reported: %+v", vs)
	}
}
