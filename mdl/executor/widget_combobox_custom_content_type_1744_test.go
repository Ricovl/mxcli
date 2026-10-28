// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1744, the other half: the enumeration that switches a combo box's
// custom content on was written on its own, so
//
//	combobox cbPerson (…, optionsSourceAssociationCustomContentType: 'yes')
//
// passed check and exec and failed mx check with CE0642 "Property 'Custom
// content' is required." at Widgets container 'Custom content' of combo box
// 'cbPerson'. The content is required while the type is 'yes' or 'listItem',
// and hidden while it is 'no'.

const associationComboTypeOnly = `combobox cbParent (DataSource: microflow Mod.GET_Parents, CaptionAttribute: Name,
  optionsSourceAssociationCustomContentType: 'yes')`

// customContentTypeViolations is MDL-WIDGET53 for one combo box, checked with
// no project: the combo box's definition is embedded.
func customContentTypeViolations(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, v := range validateWidgetTree([]*ast.WidgetV3{parseSnippetWidget(t, src)}, LoadWidgetRegistry(""), "page X") {
		if v.RuleID == "MDL-WIDGET53" {
			out = append(out, v.RuleID+": "+v.Message)
		}
	}
	return out
}

func TestComboBoxCustomContentType1744_OnWithoutContentIsRefused(t *testing.T) {
	cases := map[string]string{
		"association yes":      associationComboTypeOnly,
		"association listItem": strings.Replace(associationComboTypeOnly, "'yes'", "'listItem'", 1),
		"database yes": `combobox cbCode (optionsSourceDatabaseDataSource: microflow Mod.GET_Parents,
  Attribute: Code, CaptionAttribute: Name, optionsSourceDatabaseValueAttribute: Name,
  optionsSourceDatabaseCustomContentType: 'yes')`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := comboBoxDatabaseEngine(t).Build(realComboBoxDef(t), parseSnippetWidget(t, src))
			if err == nil || !strings.Contains(err.Error(), "CE0642") || !strings.Contains(err.Error(), "customcontent {") {
				t.Errorf("exec: got %v, want a refusal naming CE0642 and the content block", err)
			}
			if got := customContentTypeViolations(t, src); len(got) != 1 || !strings.Contains(got[0], "CE0642") {
				t.Errorf("check: got %v, want one MDL-WIDGET53 naming CE0642", got)
			}
		})
	}
}

// Content under a type of 'no' is stored and never shown: the editor hides the
// content while the type is 'no'.
func TestComboBoxCustomContentType1744_ContentWithTypeNoIsRefused(t *testing.T) {
	src := strings.Replace(associationComboWithContent, "'yes'", "'no'", 1)
	_, err := comboBoxDatabaseEngine(t).Build(realComboBoxDef(t), parseSnippetWidget(t, src))
	if err == nil || !strings.Contains(err.Error(), "'no'") {
		t.Errorf("exec: got %v, want a refusal of content under 'no'", err)
	}
	if got := customContentTypeViolations(t, src); len(got) != 1 {
		t.Errorf("check: got %v, want one MDL-WIDGET53", got)
	}
}

// Content written without the type switches it on: the content is what the
// author asked to show, and 'yes' is the one value that shows it as written.
func TestComboBoxCustomContentType1744_ContentSwitchesTheTypeOn(t *testing.T) {
	src := strings.Replace(associationComboWithContent, ",\n  optionsSourceAssociationCustomContentType: 'yes'", "", 1)
	if strings.Contains(src, "CustomContentType") {
		t.Fatalf("fixture still names the type:\n%s", src)
	}
	cw := buildComboBox(t, src)
	if got := storedPrimitive(t, cw, "optionsSourceAssociationCustomContentType"); got != "yes" {
		t.Errorf("optionsSourceAssociationCustomContentType = %q, want yes", got)
	}
	if n := len(slotWidgets(customWidgetValue(t, cw, "optionsSourceAssociationCustomContent"))); n != 1 {
		t.Errorf("custom content holds %d widgets, want 1", n)
	}
	if got := customContentTypeViolations(t, src); len(got) != 0 {
		t.Errorf("check: %v", got)
	}
}

// Stored content under the type 'no' is hidden. Printed as a block it would
// re-execute as content that is SHOWN — the write turns the type on — so
// describe names it instead, and prints no block.
func TestDescribeComboBox1744_ContentUnderTypeNoStaysNamed(t *testing.T) {
	w := storedAssociationComboWithContent(t)
	gridValue(t, w, "optionsSourceAssociationCustomContentType")["PrimitiveValue"] = "no"
	got := describeParsed(t, w)
	if strings.Contains(got, "optionssourceassociationcustomcontent {") {
		t.Errorf("hidden content printed as a block:\n%s", got)
	}
	if !strings.Contains(got, "NOT SHOWN") || !strings.Contains(got, "ccName") {
		t.Errorf("hidden content is not named:\n%s", got)
	}
}

// Controls: the type left at 'no', or not written, with no content is the
// ordinary combo box; 'listItem' with content is as valid as 'yes'.
func TestComboBoxCustomContentType1744_Controls(t *testing.T) {
	for name, src := range map[string]string{
		"type no, no content":        strings.Replace(associationComboTypeOnly, "'yes'", "'no'", 1),
		"no type, no content":        `combobox cbParent (DataSource: microflow Mod.GET_Parents, CaptionAttribute: Name)`,
		"type yes with content":      associationComboWithContent,
		"type listItem, content":     strings.Replace(associationComboWithContent, "'yes'", "'listItem'", 1),
		"database type with content": databaseComboWithContent,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := comboBoxDatabaseEngine(t).Build(realComboBoxDef(t), parseSnippetWidget(t, src)); err != nil {
				t.Errorf("exec refused: %v", err)
			}
			if got := customContentTypeViolations(t, src); len(got) != 0 {
				t.Errorf("check: %v", got)
			}
		})
	}
}
