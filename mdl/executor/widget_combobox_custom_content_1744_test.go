// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1744 (mendixlabs/mxcli#1663): a combo box's custom content could
// not be written. `mxcli widget describe combobox` listed
// optionsSourceAssociationCustomContent / optionsSourceDatabaseCustomContent as
// `widgets` properties, but a block for one was refused —
//
//	MDL-WIDGET26 "`optionssourceassociationcustomcontent` is not a container
//	of `combobox`, which declares no containers"
//
// — while the enumeration that switches it on WAS written, so
// `optionsSourceAssociationCustomContentType: 'yes'` on its own passed check
// and exec and failed mx check with CE0642 "Property 'Custom content' is
// required.". describe printed the type and, for content Studio Pro wrote, only
// a NOT SHOWN note naming the widgets.

const customContentProject = "../../testdata/expr-checker/minimal.mpr"

// associationComboWithContent is the reported shape in the harness's model: an
// association-mode combo box whose option list is Mod.Parent, showing the
// option's Name through custom content.
const associationComboWithContent = `combobox cbParent (DataSource: microflow Mod.GET_Parents, CaptionAttribute: Name,
  optionsSourceAssociationCustomContentType: 'yes') {
  optionssourceassociationcustomcontent cc {
    dynamictext ccName (Content: '{1}', ContentParams: ({1} = Name))
  }
}`

const databaseComboWithContent = `combobox cbCode (optionsSourceDatabaseDataSource: microflow Mod.GET_Parents,
  Attribute: Code, CaptionAttribute: Name, optionsSourceDatabaseValueAttribute: Name,
  optionsSourceDatabaseCustomContentType: 'yes') {
  optionssourcedatabasecustomcontent {
    dynamictext ccName (Content: '{1}', ContentParams: ({1} = Name))
  }
}`

// slotWidgets returns the widgets stored in a widgets-typed property value.
func slotWidgets(value bson.D) []bson.D {
	arr, _ := bsonField(value, "Widgets").(bson.A)
	var out []bson.D
	for _, e := range arr {
		if d, ok := e.(bson.D); ok {
			out = append(out, d)
		}
	}
	return out
}

func assertCustomContent(t *testing.T, src, slot string, emptySlots ...string) {
	t.Helper()
	cw := buildComboBox(t, src)
	got := slotWidgets(customWidgetValue(t, cw, slot))
	if len(got) != 1 {
		t.Fatalf("%s holds %d widgets, want the one written", slot, len(got))
	}
	if name, _ := bsonField(got[0], "Name").(string); name != "ccName" {
		t.Errorf("%s holds %q, want ccName", slot, name)
	}
	// The content is shown per option, so it binds to the option entity — not
	// to the data view's Mod.Child the combo box sits in.
	if refs := modelRefs(got[0]); !refs["Attribute=Mod.Parent.Name"] {
		t.Errorf("the content's parameter is not bound to the option entity: %v", refs)
	}
	for _, other := range emptySlots {
		if n := len(slotWidgets(customWidgetValue(t, cw, other))); n != 0 {
			t.Errorf("%s holds %d widgets, want none — the content belongs to %s", other, n, slot)
		}
	}
}

func TestComboBoxCustomContent1744_AssociationModeIsWritten(t *testing.T) {
	assertCustomContent(t, associationComboWithContent,
		"optionsSourceAssociationCustomContent", "optionsSourceDatabaseCustomContent")
}

func TestComboBoxCustomContent1744_DatabaseModeIsWritten(t *testing.T) {
	assertCustomContent(t, databaseComboWithContent,
		"optionsSourceDatabaseCustomContent", "optionsSourceAssociationCustomContent")
}

// The reported script, checked against a project: the slot is a container of
// the combo box, so MDL-WIDGET26 must not fire.
func TestComboBoxCustomContent1744_CheckAcceptsTheSlot(t *testing.T) {
	for _, src := range []string{associationComboWithContent, databaseComboWithContent} {
		got := widgetKindViolations(t, customContentProject, []*ast.WidgetV3{parseSnippetWidget(t, src)})
		for _, rule := range []string{"MDL-WIDGET26", "MDL-WIDGET30"} {
			if containsRule(got, rule) {
				t.Errorf("%s reported for custom content in its own mode: %v", rule, got)
			}
		}
	}
}

// A slot of another mode is the wrong place: the content would land in a
// property the combo box never renders. Refused at check and at exec, naming
// the slot this combo box's mode does have.
func TestComboBoxCustomContent1744_WrongModeSlotIsRefused(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"database slot on an association combo box",
			strings.Replace(associationComboWithContent, "optionssourceassociationcustomcontent cc", "optionssourcedatabasecustomcontent cc", 1),
			"optionssourceassociationcustomcontent"},
		{"association slot on a database combo box",
			strings.Replace(databaseComboWithContent, "optionssourcedatabasecustomcontent", "optionssourceassociationcustomcontent", 1),
			"optionssourcedatabasecustomcontent"},
		{"any slot on an enumeration combo box",
			"combobox cbEnum (Attribute: Code) {\n  optionssourceassociationcustomcontent { dynamictext t (Content: 'x') }\n}",
			"declares no slot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := parseSnippetWidget(t, tc.src)
			_, err := comboBoxDatabaseEngine(t).Build(realComboBoxDef(t), w)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("exec: got %v, want a refusal naming %q", err, tc.want)
			}
			got := widgetKindViolations(t, customContentProject, []*ast.WidgetV3{w})
			if !containsRule(got, "MDL-WIDGET30") || !containsText(got, tc.want) {
				t.Errorf("check: got %v, want MDL-WIDGET30 naming %q", got, tc.want)
			}
		})
	}
}

// storedAssociationComboWithContent is an association-mode combo box as
// Studio Pro stores it, with custom content holding one dynamic text bound to
// the option entity.
func storedAssociationComboWithContent(t *testing.T) map[string]any {
	t.Helper()
	w := widgetFromTemplate(t, comboBoxWidgetID, "cbParent")
	gridValue(t, w, "source")["PrimitiveValue"] = "context"
	gridValue(t, w, "optionsSourceType")["PrimitiveValue"] = "association"
	gridValue(t, w, "optionsSourceAssociationDataSource")["DataSource"] =
		dsBSON(dsTypeMicroflow, "MicroflowSettings", map[string]any{"Microflow": "Mod.GET_Parents"})
	gridValue(t, w, "optionsSourceAssociationCaptionAttribute")["AttributeRef"] =
		map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "Mod.Parent.Name"}
	gridValue(t, w, "optionsSourceAssociationCustomContentType")["PrimitiveValue"] = "yes"
	gridValue(t, w, "optionsSourceAssociationCustomContent")["Widgets"] = []any{int32(2), map[string]any{
		"$Type": "Forms$DynamicText", "Name": "ccName", "RenderMode": "Text",
		"Content": map[string]any{
			"$Type": "Forms$ClientTemplate",
			"Template": map[string]any{"$Type": "Texts$Text", "Items": []any{int32(3),
				map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "{1}"}}},
			"Parameters": []any{int32(2), map[string]any{
				"$Type":        "Forms$ClientTemplateParameter",
				"AttributeRef": map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": "Mod.Parent.Name"},
			}},
		},
	}}
	return w
}

// describe prints the content as the block that writes it, and the
// description builds the same content again — a describe → exec round trip
// keeps it instead of naming it in a note.
func TestDescribeComboBox1744_CustomContentRoundTrips(t *testing.T) {
	got := describeParsed(t, storedAssociationComboWithContent(t))
	for _, want := range []string{"optionssourceassociationcustomcontent {", "dynamictext ccName", "optionsSourceAssociationCustomContentType: 'yes'"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "NOT SHOWN") {
		t.Errorf("describe still names the content as not shown:\n%s", got)
	}
	if _, errs := visitor.Build("create snippet M.S {\n" + got + "\n};"); len(errs) > 0 {
		t.Fatalf("description does not parse: %v\n%s", errs, got)
	}
	assertCustomContent(t, got[strings.Index(got, "combobox"):], "optionsSourceAssociationCustomContent")
}

// Content in a slot describe cannot restate in the described mode — here the
// database mode's, on an association-mode combo box — stays named in the note.
func TestDescribeComboBox1744_OtherModesContentStaysNamed(t *testing.T) {
	w := storedAssociationComboWithContent(t)
	gridValue(t, w, "optionsSourceDatabaseCustomContent")["Widgets"] = []any{int32(2),
		map[string]any{"$Type": "Forms$DynamicText", "Name": "staleText"}}
	got := describeParsed(t, w)
	if !strings.Contains(got, "NOT SHOWN") || !strings.Contains(got, "staleText") ||
		!strings.Contains(got, "optionsSourceDatabaseCustomContent") {
		t.Errorf("content of another mode is not named:\n%s", got)
	}
	if strings.Contains(got, "optionssourcedatabasecustomcontent {") {
		t.Errorf("content of another mode printed as a block, which exec refuses:\n%s", got)
	}
	if !strings.Contains(got, "optionssourceassociationcustomcontent {") {
		t.Errorf("the mode's own content is no longer printed:\n%s", got)
	}
}

// The slot's key written as a property value: a widgets property takes its
// widgets in the block, so the value is not written, and check refuses it as
// any container written as a value (MDL-WIDGET27), naming the block. It was a
// known-but-unmapped key (MDL-WIDGET06) until the slot existed, and the
// explicit pass stored the text as the widgets value's PrimitiveValue.
func TestComboBoxCustomContent1744_SlotKeyAsValueIsNotWritten(t *testing.T) {
	src := `combobox cbParent (DataSource: microflow Mod.GET_Parents, CaptionAttribute: Name,
  optionsSourceAssociationCustomContent: 'x')`
	cw := buildComboBox(t, src)
	if pv, _ := bsonField(customWidgetValue(t, cw, "optionsSourceAssociationCustomContent"), "PrimitiveValue").(string); pv != "" {
		t.Errorf("the widgets property stores PrimitiveValue %q", pv)
	}
	var msgs []string
	for _, v := range validateObjectEntryProperties(parseSnippetWidget(t, src), LoadWidgetRegistry(""), "page P") {
		msgs = append(msgs, v.RuleID+": "+v.Message+" | "+v.Suggestion)
	}
	if !containsRule(msgs, "MDL-WIDGET27") || !containsText(msgs, "optionssourceassociationcustomcontent") {
		t.Errorf("check does not refuse the value and name the block: %v", msgs)
	}
}

// ako/mxcli#1648 refused a rewrite that changes a combo box holding custom
// content, because the statement could not restate the content. Content
// describe prints as a block is restated, so changing the combo box is no
// longer refused — the rebuild writes it. Content describe does not print still
// is (TestChangedComboBoxWithHiddenContentIsRefused is the control).
func TestPassthrough1744_DescribedContentIsNotRefused(t *testing.T) {
	const withBlock = describedCombo + ` {
  optionssourceassociationcustomcontent {
    dynamictext txtOptionName (Content: 'x')
  }
}`
	typ, obj := storedComboWithContent("txtOptionName")
	declared := parseSnippetWidget(t, withBlock)
	hidden := undescribedContent(comboBoxPackageWidgetID, storedHiddenContent(comboBoxPackageWidgetID, typ, obj), declared)
	if len(hidden) != 0 {
		t.Fatalf("content describe printed is still hidden: %+v", hidden)
	}
	pt := &pluggablePassthrough{byName: map[string]*storedPluggable{
		"cbBooking": {widgetID: comboBoxPackageWidgetID, typ: typ, obj: obj, declared: declared, hidden: hidden},
	}}
	changed := parseSnippetWidget(t, strings.Replace(withBlock, "Label: 'Booking'", "Label: 'Booking (changed)'", 1))
	if err := pt.refuseChangedWithHiddenContent([]*ast.WidgetV3{changed}); err != nil {
		t.Errorf("a change restating the content was refused: %v", err)
	}
	if err := pt.refuseLostHiddenContent(realComboBoxDef(t), changed); err != nil {
		t.Errorf("exec refused a rebuild that writes the content: %v", err)
	}

	// Control: the same stored content with a description that does not print
	// it stays hidden, and the change is refused.
	if got := undescribedContent(comboBoxPackageWidgetID, storedHiddenContent(comboBoxPackageWidgetID, typ, obj),
		parseSnippetWidget(t, describedCombo)); len(got) != 1 {
		t.Errorf("content describe did not print was dropped from the refusal: %+v", got)
	}
}
