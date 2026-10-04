// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"crypto/rand"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#980: `describe navigation` printed a menu item whose action was not
// show page, call microflow or sign out with no action, and dropped a flow
// call's settings, so a re-run of the description stored Forms$NoAction and the
// default settings in their place. TestApp's own 'Item 4' (a nanoflow call)
// is covered by the whole-fixture round trip; this adds, to the Studio
// Pro-authored Responsive profile, one item per action kind in the shape
// Studio Pro stores it (measured on TestApp's buttons and menu), each with
// non-default settings, plus the two properties MDL cannot spell.
//
// Each must survive describe -> exec (GetPut: nothing written) and describe
// again (PutGet: the same text). Then the same description with every caption
// renamed — so no item pairs with a stored one and each action is built from
// MDL alone — must store each action equal to Studio Pro's apart from $IDs.
func TestNavigationMenuActionsRoundTrip_980(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()

	rel, nav := navigationUnit(t, h)
	items := []struct {
		caption string
		action  bson.D
		onClick string // what describe must print
		extra   bool   // carries a property MDL cannot spell (kept only when paired)
	}{
		{"NF settings", bson.D{
			{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$CallNanoflowClientAction"},
			{Key: "ConfirmationInfo", Value: confirmation("Run the report?", "Run", "Not now")},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "Nanoflow", Value: "MyFirstModule.Nanoflow"},
			{Key: "OutputMappings", Value: bson.A{int32(3)}},
			{Key: "ParameterMappings", Value: bson.A{int32(2)}},
			{Key: "ProgressBar", Value: "Blocking"},
			{Key: "ProgressMessage", Value: text("Working")},
		}, "call nanoflow MyFirstModule.Nanoflow with (DisabledDuringExecution: false, ProgressBar: Blocking, " +
			"ProgressMessage: 'Working', Confirmation: 'Run the report?', ProceedCaption: 'Run', CancelCaption: 'Not now')", false},
		{"MF settings", bson.D{
			{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$MicroflowAction"},
			{Key: "DisabledDuringExecution", Value: true},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Asynchronous", Value: true},
				{Key: "ConfirmationInfo", Value: nil},
				{Key: "FormValidations", Value: "None"},
				{Key: "Microflow", Value: "MyFirstModule.MyFirstLogic"},
				{Key: "OutputMappings", Value: bson.A{int32(3)}},
				{Key: "ParameterMappings", Value: bson.A{int32(2)}},
				{Key: "ProgressBar", Value: "NonBlocking"},
				{Key: "ProgressMessage", Value: text("Please wait")},
			}},
		}, "call microflow MyFirstModule.MyFirstLogic with (ProgressBar: NonBlocking, ProgressMessage: 'Please wait', " +
			"Asynchronous: true, FormValidations: None)", false},
		{"Docs", bson.D{
			{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$OpenLinkClientAction"},
			{Key: "Address", Value: bson.D{
				{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$StaticOrDynamicString"},
				{Key: "IsDynamic", Value: false}, {Key: "Value", Value: "https://docs.mendix.com"},
			}},
			{Key: "DisabledDuringExecution", Value: true},
			{Key: "LinkType", Value: "Web"},
		}, "open link 'https://docs.mendix.com'", false},
		{"New category", bson.D{
			{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$CreateObjectClientAction"},
			{Key: "DisabledDuringExecution", Value: true},
			{Key: "EntityRef", Value: bson.D{
				{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: "Rules.RuleCategory"},
			}},
			{Key: "NumberOfPagesToClose2", Value: ""},
			{Key: "PageSettings", Value: formSettings("Rules.RuleCategory_NewEdit", nil)},
		}, "create object Rules.RuleCategory then show page Rules.RuleCategory_NewEdit", false},
		{"Mail", bson.D{
			{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$OpenLinkClientAction"},
			{Key: "Address", Value: bson.D{
				{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$StaticOrDynamicString"},
				{Key: "IsDynamic", Value: false}, {Key: "Value", Value: "support@example.com"},
			}},
			{Key: "DisabledDuringExecution", Value: true},
			{Key: "LinkType", Value: "Email"},
		}, "open link 'support@example.com'", true},
		{"Titled", bson.D{
			{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$FormAction"},
			{Key: "DisabledDuringExecution", Value: true},
			{Key: "FormSettings", Value: formSettings("Pages.Vehicle_Overview", bson.D{
				{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Microflows$TextTemplate"},
				{Key: "Arguments", Value: bson.A{int32(2)}},
				{Key: "Text", Value: text("All vehicles")},
			})},
			{Key: "NumberOfPagesToClose2", Value: ""},
			{Key: "PagesForSpecializations", Value: bson.A{int32(2)}},
		}, "show page Pages.Vehicle_Overview", true},
	}
	for _, it := range items {
		nav = appendMenuItem(t, nav, "Responsive", it.caption, it.action)
	}
	out, err := bson.Marshal(nav)
	if err != nil {
		t.Fatal(err)
	}
	h.fixture[rel] = out
	h.restore()
	before := h.snapshot()

	first, err := h.describe("navigation Responsive")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	for _, it := range items {
		want := "menu item '" + it.caption + "' ( OnClick: " + it.onClick + " )"
		if !strings.Contains(first, want) {
			t.Errorf("describe lost %q's action; want line\n  %s\nin\n%s", it.caption, want, first)
		}
	}
	for _, note := range []string{"link type Email has no MDL form", "a page title override ('All vehicles') has no MDL form"} {
		if !strings.Contains(first, note) {
			t.Errorf("describe does not flag what it cannot spell: want %q in\n%s", note, first)
		}
	}

	// GetPut: the description writes nothing.
	if err := h.exec(first); err != nil {
		t.Fatalf("exec describe output: %v\n%s", err, first)
	}
	if changed := before.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("executing the describe output wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
	}
	// PutGet.
	if second, err := h.describe("navigation Responsive"); err != nil || second != first {
		t.Errorf("second describe differs (err=%v):\n%s", err, lineDiff(first, second))
	}

	// Built from MDL alone: rename every added caption so nothing pairs.
	renamed := first
	for _, it := range items {
		renamed = strings.Replace(renamed, "menu item '"+it.caption+"'", "menu item '"+it.caption+" 2'", 1)
	}
	if err := h.exec(renamed); err != nil {
		t.Fatalf("exec renamed: %v", err)
	}
	_, stored := navigationUnit(t, h)
	for _, it := range items {
		got := menuItemAction(stored, "Responsive", it.caption+" 2")
		if got == nil {
			t.Errorf("%q: renamed item not stored", it.caption)
			continue
		}
		diffs := actionDiff(it.action, got)
		if it.extra {
			// A renamed item cannot keep what MDL cannot spell; everything
			// else must still match.
			diffs = withoutKeys(diffs, "LinkType", "TitleOverride")
		}
		if len(diffs) > 0 {
			t.Errorf("%q built from MDL differs from Studio Pro's shape:\n  %s", it.caption, strings.Join(diffs, "\n  "))
		}
	}
}

func newID() bson.Binary {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return bson.Binary{Subtype: 0, Data: b}
}

func text(s string) bson.D {
	return bson.D{
		{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3), bson.D{
			{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: "en_US"}, {Key: "Text", Value: s},
		}}},
	}
}

func confirmation(q, proceed, cancel string) bson.D {
	return bson.D{
		{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$ConfirmationInfo"},
		{Key: "CancelButtonCaption", Value: text(cancel)},
		{Key: "ProceedButtonCaption", Value: text(proceed)},
		{Key: "Question", Value: text(q)},
	}
}

func formSettings(page string, title any) bson.D {
	return bson.D{
		{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Forms$FormSettings"},
		{Key: "Form", Value: page},
		{Key: "ParameterMappings", Value: bson.A{int32(2)}},
		{Key: "TitleOverride", Value: title},
	}
}

func menuItemDoc(caption string, action bson.D) bson.D {
	return bson.D{
		{Key: "$ID", Value: newID()}, {Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: action},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: text(caption)},
		{Key: "Icon", Value: nil},
		{Key: "Items", Value: bson.A{int32(3)}},
	}
}

// navigationUnit finds the navigation document in the working copy.
func navigationUnit(t *testing.T, h *harness) (string, bson.D) {
	t.Helper()
	for rel, b := range h.fixture {
		if filepath.Ext(rel) != ".mxunit" {
			continue
		}
		if typ, _ := typeAndName(b); typ == "Navigation$NavigationDocument" {
			id := strings.TrimSuffix(filepath.Base(rel), ".mxunit")
			var d bson.D
			if err := bson.Unmarshal(h.unitBytes(t, id), &d); err != nil {
				t.Fatal(err)
			}
			return rel, d
		}
	}
	t.Fatal("no navigation document in the fixture")
	return "", nil
}

func navField(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func profileMenu(nav bson.D, profile string) bson.D {
	for _, p := range navField(nav, "Profiles").(bson.A) {
		pd, ok := p.(bson.D)
		if ok && navField(pd, "Name") == profile {
			m, _ := navField(pd, "Menu").(bson.D)
			return m
		}
	}
	return nil
}

func appendMenuItem(t *testing.T, nav bson.D, profile, caption string, action bson.D) bson.D {
	t.Helper()
	menu := profileMenu(nav, profile)
	if menu == nil {
		t.Fatalf("profile %s has no menu", profile)
	}
	for i := range menu {
		if menu[i].Key == "Items" {
			menu[i].Value = append(menu[i].Value.(bson.A), menuItemDoc(caption, action))
		}
	}
	return nav
}

func menuItemAction(nav bson.D, profile, caption string) bson.D {
	menu := profileMenu(nav, profile)
	for _, it := range navField(menu, "Items").(bson.A) {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		items := navField(navField(d, "Caption").(bson.D), "Items").(bson.A)
		if tr, ok := items[1].(bson.D); ok && navField(tr, "Text") == caption {
			a, _ := navField(d, "Action").(bson.D)
			return a
		}
	}
	return nil
}

// actionDiff is bsonDiff of two action documents with $IDs ignored.
func actionDiff(want, got bson.D) []string {
	a, _ := bson.Marshal(want)
	b, _ := bson.Marshal(got)
	var out []string
	for _, d := range bsonDiff(a, b) {
		if !strings.Contains(d, "$ID") {
			out = append(out, d)
		}
	}
	return out
}

func withoutKeys(diffs []string, keys ...string) []string {
	var out []string
next:
	for _, d := range diffs {
		for _, k := range keys {
			if strings.Contains(d, k) {
				continue next
			}
		}
		out = append(out, d)
	}
	return out
}
