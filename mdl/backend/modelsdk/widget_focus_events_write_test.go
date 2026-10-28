// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The writer hard-coded a NoAction into every input widget's OnEnterAction
// and OnLeaveAction (the text box's OnEnterAction excepted), so Studio Pro's
// "On enter" / "On leave" actions could not be written and describe → exec
// replaced stored ones with nothing (ako/mxcli#1743's sibling slots). The
// control is an input that sets neither, which keeps the empty NoActions.
func TestWidgetToGen_InputWidgetsWriteFocusEvents(t *testing.T) {
	type focusSlots interface {
		OnEnterAction() element.Element
		OnLeaveAction() element.Element
	}
	enter := func() pages.ClientAction {
		return &pages.MicroflowClientAction{BaseElement: model.BaseElement{ID: "e"}, MicroflowName: "M.ACT_Enter"}
	}
	leave := func() pages.ClientAction {
		return &pages.MicroflowClientAction{BaseElement: model.BaseElement{ID: "l"}, MicroflowName: "M.ACT_Leave"}
	}
	cases := []struct {
		set, unset pages.Widget
	}{
		{&pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tb"}, OnEnterAction: enter(), OnLeaveAction: leave()},
			&pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tb"}}},
		{&pages.TextArea{BaseWidget: pages.BaseWidget{Name: "ta"}, OnEnterAction: enter(), OnLeaveAction: leave()},
			&pages.TextArea{BaseWidget: pages.BaseWidget{Name: "ta"}}},
		{&pages.DatePicker{BaseWidget: pages.BaseWidget{Name: "dp"}, OnEnterAction: enter(), OnLeaveAction: leave()},
			&pages.DatePicker{BaseWidget: pages.BaseWidget{Name: "dp"}}},
		{&pages.DropDown{BaseWidget: pages.BaseWidget{Name: "dd"}, OnEnterAction: enter(), OnLeaveAction: leave()},
			&pages.DropDown{BaseWidget: pages.BaseWidget{Name: "dd"}}},
		{&pages.CheckBox{BaseWidget: pages.BaseWidget{Name: "cb"}, OnEnterAction: enter(), OnLeaveAction: leave()},
			&pages.CheckBox{BaseWidget: pages.BaseWidget{Name: "cb"}}},
		{&pages.RadioButtons{BaseWidget: pages.BaseWidget{Name: "rb"}, OnEnterAction: enter(), OnLeaveAction: leave()},
			&pages.RadioButtons{BaseWidget: pages.BaseWidget{Name: "rb"}}},
	}
	flowOf := func(t *testing.T, el element.Element) string {
		t.Helper()
		a, ok := el.(*genPg.MicroflowClientAction)
		if !ok {
			return ""
		}
		return a.MicroflowSettings().(*genPg.MicroflowSettings).MicroflowQualifiedName()
	}
	for _, c := range cases {
		el, err := widgetToGen(c.set)
		if err != nil {
			t.Fatalf("%T: %v", c.set, err)
		}
		g := el.(focusSlots)
		if got := flowOf(t, g.OnEnterAction()); got != "M.ACT_Enter" {
			t.Errorf("%T: OnEnterAction = %T %q, want the call to M.ACT_Enter", c.set, g.OnEnterAction(), got)
		}
		if got := flowOf(t, g.OnLeaveAction()); got != "M.ACT_Leave" {
			t.Errorf("%T: OnLeaveAction = %T %q, want the call to M.ACT_Leave", c.set, g.OnLeaveAction(), got)
		}

		el, err = widgetToGen(c.unset)
		if err != nil {
			t.Fatalf("%T: %v", c.unset, err)
		}
		g = el.(focusSlots)
		if _, ok := g.OnEnterAction().(*genPg.NoClientAction); !ok {
			t.Errorf("%T unset: OnEnterAction = %T, want the NoAction Studio Pro stores", c.unset, g.OnEnterAction())
		}
		if _, ok := g.OnLeaveAction().(*genPg.NoClientAction); !ok {
			t.Errorf("%T unset: OnLeaveAction = %T, want the NoAction Studio Pro stores", c.unset, g.OnLeaveAction())
		}
	}
}
