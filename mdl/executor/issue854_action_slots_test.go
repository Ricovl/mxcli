// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// ako/mxcli#854: the hand-written built-in definitions left action slots of
// their widgets unmapped — a data grid filter's `onChange`, the barcode
// scanner's `onDetect`, the combo box's `onEnterEvent`, `onLeaveEvent` and
// `onChangeFilterInputEvent`. An action written on one was refused under
// `mdl 1;` and dropped with a warning under mdl 0 (MDL-V1-ACTIONSLOT for
// `OnChange:`, MDL-WIDGET06 for a slot named by its key), and describe printed
// none of a filter's, so the action could not round-trip.
//
// A hand-written definition beats the generator, which maps every action slot
// of a generated widget (the change slot under `OnChange:`, the click slot
// under `onClick:`, every other by its own key), so a gap in one is invisible
// until someone writes the action. This test is the audit the #842 finding
// asked for: every action property the widget's package declares must be
// mapped, and a slot addressed by its own key must be mapped in every mode,
// since modes are exclusive.

// actionKeysFromTestdataMPKs indexes the action-typed top-level properties of
// every widget package in testdata, by widget ID.
func actionKeysFromTestdataMPKs(t *testing.T) map[string][]string {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "expr-checker", "widgets")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read widget testdata: %v", err)
	}
	mpk.ClearCache()
	out := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mpk") {
			continue
		}
		defs, err := mpk.ParseMPKAll(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, d := range defs {
			for _, p := range d.Properties {
				if p.Type == "action" {
					out[d.ID] = append(out[d.ID], p.Key)
				}
			}
		}
	}
	return out
}

func TestBuiltinDefsMapEveryActionSlot(t *testing.T) {
	actions := actionKeysFromTestdataMPKs(t)
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	compared := 0
	for _, def := range reg.All() {
		keys, ok := actions[def.WidgetID]
		if !ok {
			continue // no action slot, or no package in testdata
		}
		sort.Strings(keys)
		// Each mode is a complete mapping set and exactly one applies — the
		// engine ignores the top-level mappings of a definition with modes
		// (selectMappings); a definition without modes has the one set.
		sets := [][]PropertyMapping{def.PropertyMappings}
		if len(def.Modes) > 0 {
			sets = nil
			for _, m := range def.Modes {
				sets = append(sets, m.PropertyMappings)
			}
		}
		for _, key := range keys {
			compared++
			var mappedIn int
			named := false
			for _, set := range sets {
				for _, m := range set {
					if m.Operation == "action" && m.PropertyKey == key {
						mappedIn++
						named = named || m.Source == ""
						break
					}
				}
			}
			switch {
			case mappedIn == 0:
				t.Errorf("%s: action slot %q has no mapping — an action written on it is dropped", def.MDLName, key)
			case named && mappedIn != len(sets):
				t.Errorf("%s: action slot %q is addressed by its own key but mapped in %d of %d modes — "+
					"in the others the action is dropped", def.MDLName, key, mappedIn, len(sets))
			}
		}
	}
	if compared == 0 {
		t.Fatal("no action slot compared; the test cannot detect anything")
	}
}

// issue854SlotsSetup is a project with one widget per unmapped slot, each
// holding a microflow call.
const issue854SlotsSetup = `create enumeration MyFirstModule.Colour (Red 'Red', Blue 'Blue');
create persistent entity MyFirstModule.Chore (Title: String(100), Colour: Enumeration(MyFirstModule.Colour), Points: Integer, Due: DateTime);
create microflow MyFirstModule.ACT_Changed () begin end;
create microflow MyFirstModule.ACT_Enter () begin end;
create microflow MyFirstModule.ACT_Leave () begin end;
create microflow MyFirstModule.ACT_Typed () begin end;
create microflow MyFirstModule.ACT_Detected () begin end;`

const issue854SlotsPage = `create page MyFirstModule.Slots (Title: 'Slots', Layout: Atlas_Core.Atlas_Default, Params: ($Chore: MyFirstModule.Chore)) {
  dataview dvChore (DataSource: $Chore) {
    combobox cbColour (Label: 'Colour', Attribute: Colour, onEnterEvent: call microflow MyFirstModule.ACT_Enter(), onLeaveEvent: call microflow MyFirstModule.ACT_Leave(), onChangeFilterInputEvent: call microflow MyFirstModule.ACT_Typed())
    barcodescanner bsTitle (Attribute: Title, onDetect: call microflow MyFirstModule.ACT_Detected())
  }
  datagrid dgChores (DataSource: database from MyFirstModule.Chore) {
    column (Attribute: Title, Caption: 'Title') {
      textfilter tfTitle (OnChange: call microflow MyFirstModule.ACT_Changed())
    }
    column (Attribute: Points, Caption: 'Points') {
      numberfilter nfPoints (OnChange: call microflow MyFirstModule.ACT_Changed())
    }
    column (Attribute: Due, Caption: 'Due') {
      datefilter dfDue (OnChange: call microflow MyFirstModule.ACT_Changed())
    }
    column (Attribute: Colour, Caption: 'Colour') {
      dropdownfilter ddfColour (OnChange: call microflow MyFirstModule.ACT_Changed())
    }
  }
};`

// issue854Slots lists, per widget, the slot and the microflow it must call.
var issue854Slots = []struct{ widget, key, flow string }{
	{"cbColour", "onEnterEvent", "MyFirstModule.ACT_Enter"},
	{"cbColour", "onLeaveEvent", "MyFirstModule.ACT_Leave"},
	{"cbColour", "onChangeFilterInputEvent", "MyFirstModule.ACT_Typed"},
	{"bsTitle", "onDetect", "MyFirstModule.ACT_Detected"},
	{"tfTitle", "onChange", "MyFirstModule.ACT_Changed"},
	{"nfPoints", "onChange", "MyFirstModule.ACT_Changed"},
	{"dfDue", "onChange", "MyFirstModule.ACT_Changed"},
	{"ddfColour", "onChange", "MyFirstModule.ACT_Changed"},
}

func TestIssue854_UnmappedActionSlotsAreWritten(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue854SlotsSetup)
	run(t, exec, issue854SlotsPage)
	doc := storedUnitDoc(t, exec, unitIDByName(t, exec, "page", "Slots"))
	for _, s := range issue854Slots {
		act := pluggableSlotAction(t, namedDoc(t, doc, s.widget), s.key)
		if got := bsonnav.DGetString(act, "$Type"); got != "Forms$MicroflowAction" {
			t.Errorf("%s.%s holds %s, want the microflow call", s.widget, s.key, got)
			continue
		}
		settings := bsonnav.DGetDoc(act, "MicroflowSettings")
		if got := bsonnav.DGetString(settings, "Microflow"); got != s.flow {
			t.Errorf("%s.%s calls %q, want %q", s.widget, s.key, got, s.flow)
		}
	}
	// Control: the combo box's change slot, which no statement here names,
	// stays empty — a named slot must not answer for another.
	if got := bsonnav.DGetString(pluggableSlotAction(t, namedDoc(t, doc, "cbColour"), "onChangeEvent"), "$Type"); got != "Forms$NoAction" {
		t.Errorf("cbColour.onChangeEvent holds %s, want Forms$NoAction", got)
	}
}

// Describe prints every slot the statement wrote, so describe → exec keeps the
// page as it is.
func TestIssue854_DescribedSlotsRoundTrip(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854SlotsSetup)
	run(t, exec, issue854SlotsPage)

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Slots")
	described := out.String()
	for _, want := range []string{
		"onEnterEvent: call microflow MyFirstModule.ACT_Enter",
		"onLeaveEvent: call microflow MyFirstModule.ACT_Leave",
		"onChangeFilterInputEvent: call microflow MyFirstModule.ACT_Typed",
		"onDetect: call microflow MyFirstModule.ACT_Detected",
	} {
		if !strings.Contains(described, want) {
			t.Errorf("describe lacks %q:\n%s", want, described)
		}
	}
	for _, filter := range []string{"textfilter tfTitle", "numberfilter nfPoints", "datefilter dfDue", "dropdownfilter ddfColour"} {
		line := lineContaining(described, filter)
		if !strings.Contains(line, "OnChange: call microflow MyFirstModule.ACT_Changed") {
			t.Errorf("%s is described without its OnChange: %q", filter, line)
		}
	}

	out.Reset()
	run(t, exec, described)
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-running describe rewrote the page:\n%s", out.String())
	}
}

// A database-mode combo box keeps its change action in onChangeDatabaseEvent,
// which describe prints as `OnChange:` (mendixlabs/mxcli#1583). It must not
// also print it under its own key: that spelling is not authorable, and the
// slot would be described twice.
func TestIssue854_DatabaseComboBoxChangeSlotDescribedOnce(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854SlotsSetup)
	run(t, exec, `create microflow MyFirstModule.DS_Chores () returns List of MyFirstModule.Chore begin
  retrieve $Chores from MyFirstModule.Chore;
  return $Chores;
end;`)
	run(t, exec, `create page MyFirstModule.Db (Title: 'Db', Layout: Atlas_Core.Atlas_Default, Params: ($Chore: MyFirstModule.Chore)) {
  dataview dvChore (DataSource: $Chore) {
    combobox cbTitle (optionsSourceDatabaseDataSource: microflow MyFirstModule.DS_Chores, CaptionAttribute: Title, optionsSourceDatabaseValueAttribute: Title, Attribute: Title, OnChange: call microflow MyFirstModule.ACT_Changed(), onEnterEvent: call microflow MyFirstModule.ACT_Enter())
  }
};`)
	doc := storedUnitDoc(t, exec, unitIDByName(t, exec, "page", "Db"))
	cb := namedDoc(t, doc, "cbTitle")
	for key, want := range map[string]string{"onChangeDatabaseEvent": "Forms$MicroflowAction", "onEnterEvent": "Forms$MicroflowAction", "onChangeEvent": "Forms$NoAction"} {
		if got := bsonnav.DGetString(pluggableSlotAction(t, cb, key), "$Type"); got != want {
			t.Errorf("cbTitle.%s holds %s, want %s", key, got, want)
		}
	}

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Db")
	described := out.String()
	if strings.Contains(described, "onChangeDatabaseEvent") {
		t.Errorf("the database-mode change slot is printed under its own key:\n%s", described)
	}
	if strings.Count(described, "MyFirstModule.ACT_Changed") != 1 {
		t.Errorf("the change action is not printed exactly once:\n%s", described)
	}
	if !strings.Contains(described, "onEnterEvent: call microflow MyFirstModule.ACT_Enter") {
		t.Errorf("describe lacks the onEnterEvent:\n%s", described)
	}
	out.Reset()
	run(t, exec, described)
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-running describe rewrote the page:\n%s", out.String())
	}
}

func lineContaining(text, needle string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

// The validator no longer refuses the slots: `OnChange:` on a filter is routed,
// and a named slot is a property the definition maps.
func TestIssue854_SlotsPassValidation(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	act := &ast.ActionV3{Type: "microflow", Target: "M.Handler"}
	cases := []struct {
		widgetType, key string
	}{
		{"textfilter", "OnChange"},
		{"numberfilter", "OnChange"},
		{"datefilter", "OnChange"},
		{"dropdownfilter", "OnChange"},
		{"barcodescanner", "onDetect"},
		{"combobox", "onEnterEvent"},
		{"combobox", "onLeaveEvent"},
		{"combobox", "onChangeFilterInputEvent"},
	}
	for _, c := range cases {
		t.Run(c.widgetType+" "+c.key, func(t *testing.T) {
			w := &ast.WidgetV3{Type: c.widgetType, Name: "w1", Properties: map[string]any{c.key: act}}
			for _, v := range validatePluggableWidgetProperties(w, reg, "page M.P") {
				if strings.Contains(v.Message, c.key) || v.RuleID == actionSlotRefused.Code {
					t.Errorf("%s: %s", v.RuleID, v.Message)
				}
			}
		})
	}
}
