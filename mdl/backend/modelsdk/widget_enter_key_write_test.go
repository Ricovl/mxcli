// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1743: the writer hard-coded a NoAction into every text box's
// OnEnterKeyPressAction, so an enter-key action could not be written and a
// Studio Pro one was replaced by nothing on describe → exec. The control is
// a text box that sets none, which keeps the empty NoAction Studio Pro stores.
func TestWidgetToGen_TextBoxWritesOnEnterKeyPressAction(t *testing.T) {
	el, err := widgetToGen(&pages.TextBox{
		BaseWidget: pages.BaseWidget{Name: "tbSearch"},
		OnEnterKeyPressAction: &pages.MicroflowClientAction{
			BaseElement:   model.BaseElement{ID: "a"},
			MicroflowName: "ModK.ACT_Search",
		},
	})
	if err != nil {
		t.Fatalf("widgetToGen: %v", err)
	}
	g := el.(*genPg.TextBox)
	act, ok := g.OnEnterKeyPressAction().(*genPg.MicroflowClientAction)
	if !ok {
		t.Fatalf("OnEnterKeyPressAction = %T, want the microflow call", g.OnEnterKeyPressAction())
	}
	if mf := act.MicroflowSettings().(*genPg.MicroflowSettings).MicroflowQualifiedName(); mf != "ModK.ACT_Search" {
		t.Errorf("microflow = %q, want ModK.ACT_Search", mf)
	}
	if _, ok := g.OnChangeAction().(*genPg.NoClientAction); !ok {
		t.Errorf("OnChangeAction = %T; only the enter-key slot was set", g.OnChangeAction())
	}

	// Control: unset is the empty slot, as before.
	el, err = widgetToGen(&pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tbPlain"}})
	if err != nil {
		t.Fatalf("widgetToGen: %v", err)
	}
	if _, ok := el.(*genPg.TextBox).OnEnterKeyPressAction().(*genPg.NoClientAction); !ok {
		t.Errorf("unset OnEnterKeyPressAction = %T, want the NoAction Studio Pro stores", el.(*genPg.TextBox).OnEnterKeyPressAction())
	}
}
