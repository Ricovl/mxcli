// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The same loss as ako/mxcli#1743, on the other two event slots every input
// widget stores: Studio Pro's "On enter" (focus) and "On leave" actions,
// Forms$<Input>.OnEnterAction / OnLeaveAction. MDL had no property for them,
// describe never read them, and the writer hard-coded a NoAction into both on
// every input — so describe → exec of a Studio Pro input with one deleted it,
// with mx check clean. Only the change slot (`OnChange:`) round-tripped.

// focusInputKinds are the input widget keywords whose stored type has both slots.
var focusInputKinds = []string{"textbox", "textarea", "datepicker", "dropdown", "checkbox", "radiobuttons"}

func TestInputFocusEvents_CheckAccepts(t *testing.T) {
	var b strings.Builder
	b.WriteString("mdl 1;\ncreate page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n  dataview dv (DataSource: microflow M.DS) {\n")
	for _, k := range focusInputKinds {
		b.WriteString("    " + k + " w_" + k + " (Attribute: A, OnEnter: call microflow M.ACT_Enter(Item = $currentObject), OnLeaveAction: save changes)\n")
	}
	b.WriteString("    actionbutton btn (Caption: 'Go', Action: save changes, OnLeave: save changes)\n")
	b.WriteString("  }\n};\n")
	vs := enterKeyViolations(t, b.String())
	for _, k := range focusInputKinds {
		if got := violationsNaming(vs, "w_"+k); len(got) > 0 {
			t.Errorf("%s: OnEnter / OnLeave reported, but both are written:\n%s", k, violationText(got))
		}
	}
	// Control: a button has no such slot, and is told so.
	got := violationsNaming(vs, "btn")
	if len(got) == 0 {
		t.Fatal("OnLeave on an actionbutton passed check — it would be dropped")
	}
	if !strings.Contains(got[0].Message, "only an input widget has an on-leave action") {
		t.Errorf("the refusal does not say why:\n%s", violationText(got))
	}
}

// focusEventsOf returns an input widget's OnEnterAction and OnLeaveAction.
func focusEventsOf(t *testing.T, w pages.Widget) (pages.ClientAction, pages.ClientAction) {
	t.Helper()
	switch x := w.(type) {
	case *pages.TextBox:
		return x.OnEnterAction, x.OnLeaveAction
	case *pages.TextArea:
		return x.OnEnterAction, x.OnLeaveAction
	case *pages.DatePicker:
		return x.OnEnterAction, x.OnLeaveAction
	case *pages.DropDown:
		return x.OnEnterAction, x.OnLeaveAction
	case *pages.CheckBox:
		return x.OnEnterAction, x.OnLeaveAction
	case *pages.RadioButtons:
		return x.OnEnterAction, x.OnLeaveAction
	}
	t.Fatalf("widget type %T has no focus events", w)
	return nil, nil
}

func TestInputFocusEvents_BuilderWritesBoth(t *testing.T) {
	for _, k := range focusInputKinds {
		t.Run(k, func(t *testing.T) {
			mod := mkModule("Mod")
			h := mkHierarchy(mod)
			withContainer(h, mod.ID, mod.ID)
			pb := newPageBuilder(&mock.MockBackend{}, h, "Mod")
			pb.entityContext = "Mod.Ent"
			built, err := pb.buildWidgetV3(&ast.WidgetV3{Type: k, Name: "w1", Properties: map[string]any{
				"Attribute":     "Value",
				"OnEnter":       &ast.ActionV3{Type: "close"},
				"OnLeaveAction": &ast.ActionV3{Type: "save"},
			}})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			enter, leave := focusEventsOf(t, built)
			if _, ok := enter.(*pages.ClosePageClientAction); !ok {
				t.Errorf("OnEnterAction = %T, want the close-page action written", enter)
			}
			if _, ok := leave.(*pages.SaveChangesClientAction); !ok {
				t.Errorf("OnLeaveAction = %T, want the save-changes action written", leave)
			}

			// Control: an input that sets neither leaves both empty.
			plain, err := pb.buildWidgetV3(&ast.WidgetV3{Type: k, Name: "w2", Properties: map[string]any{"Attribute": "Value"}})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if enter, leave := focusEventsOf(t, plain); enter != nil || leave != nil {
				t.Errorf("unset: OnEnterAction = %T, OnLeaveAction = %T, want nil", enter, leave)
			}
		})
	}
}

// DESCRIBE reads both slots back, on every input widget describe prints.
func TestDescribeInputFocusEvents(t *testing.T) {
	flow := func(name string) map[string]any {
		return map[string]any{
			"$Type": "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{
				"$Type": "Forms$MicroflowSettings", "Microflow": name,
			},
		}
	}
	for _, typ := range []string{"Forms$TextBox", "Forms$TextArea", "Forms$DatePicker", "Forms$CheckBox", "Forms$RadioButtonGroup"} {
		t.Run(typ, func(t *testing.T) {
			ctx, buf := newMockCtx(t)
			raw := map[string]any{
				"$Type":         typ,
				"Name":          "w1",
				"OnEnterAction": flow("M.ACT_Enter"),
				"OnLeaveAction": flow("M.ACT_Leave"),
			}
			outputWidgetMDLV3(ctx, parseRawWidget(ctx, raw)[0], 0)
			out := buf.String()
			for _, want := range []string{"OnEnter: call microflow M.ACT_Enter", "OnLeave: call microflow M.ACT_Leave"} {
				if !strings.Contains(out, want) {
					t.Errorf("describe does not print %q:\n%s", want, out)
				}
			}
			// Control: NoActions print nothing.
			ctx2, buf2 := newMockCtx(t)
			raw["OnEnterAction"] = map[string]any{"$Type": "Forms$NoAction"}
			raw["OnLeaveAction"] = map[string]any{"$Type": "Forms$NoAction"}
			outputWidgetMDLV3(ctx2, parseRawWidget(ctx2, raw)[0], 0)
			if out := buf2.String(); strings.Contains(out, "OnEnter") || strings.Contains(out, "OnLeave") {
				t.Errorf("an empty slot was described as an action:\n%s", out)
			}
		})
	}
}
