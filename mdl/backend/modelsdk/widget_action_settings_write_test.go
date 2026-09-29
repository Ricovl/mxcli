// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#721 L2: the writer hard-coded a client action's settings —
// DisabledDuringExecution true, ProgressBar None, no progress message, no
// confirmation — so a describe → exec of a Studio Pro page removed a button's
// "Are you sure?" confirmation and its progress bar. The settings now come from
// the action; the defaults remain for an action that does not say.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func enText(s string) *model.Text { return &model.Text{Translations: map[string]string{"en_US": s}} }

func genTextValue(t *testing.T, el any) string {
	t.Helper()
	txt, ok := el.(*genTexts.Text)
	if !ok || txt == nil {
		t.Fatalf("text is %T, want *texts.Text", el)
	}
	items := txt.TranslationsItems()
	if len(items) != 1 {
		t.Fatalf("text has %d translations, want 1", len(items))
	}
	return items[0].(*genTexts.Translation).Text()
}

func TestClientActionToGen_MicroflowSettings(t *testing.T) {
	f := false
	el, err := clientActionToGen(&pages.MicroflowClientAction{
		BaseElement:     model.BaseElement{ID: "a"},
		MicroflowName:   "M.Lock",
		ActionExecution: pages.ActionExecution{DisabledDuringExecution: &f},
		FlowCallSettings: pages.FlowCallSettings{
			ProgressBar:     "NonBlocking",
			ProgressMessage: enText("Please wait"),
			Confirmation:    &pages.ConfirmationInfo{Question: enText("Sure?"), ProceedCaption: enText("Yes"), CancelCaption: enText("No")},
			Asynchronous:    true,
			FormValidations: "Widget",
		},
	})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	g := el.(*genPg.MicroflowClientAction)
	if g.DisabledDuringExecution() {
		t.Error("DisabledDuringExecution = true; the action said false")
	}
	s := g.MicroflowSettings().(*genPg.MicroflowSettings)
	if s.ProgressBar() != "NonBlocking" || !s.Asynchronous() || s.FormValidations() != "Widget" {
		t.Errorf("ProgressBar %q Asynchronous %v FormValidations %q", s.ProgressBar(), s.Asynchronous(), s.FormValidations())
	}
	if got := genTextValue(t, s.ProgressMessage()); got != "Please wait" {
		t.Errorf("ProgressMessage = %q", got)
	}
	ci, ok := s.ConfirmationInfo().(*genPg.ConfirmationInfo)
	if !ok {
		t.Fatalf("ConfirmationInfo = %T, want the confirmation", s.ConfirmationInfo())
	}
	if genTextValue(t, ci.Question()) != "Sure?" || genTextValue(t, ci.ProceedButtonCaption()) != "Yes" ||
		genTextValue(t, ci.CancelButtonCaption()) != "No" {
		t.Error("confirmation texts not written as given")
	}
}

// Control: an action that says nothing keeps the writer's defaults.
func TestClientActionToGen_MicroflowSettingsDefaults(t *testing.T) {
	el, err := clientActionToGen(&pages.MicroflowClientAction{BaseElement: model.BaseElement{ID: "a"}, MicroflowName: "M.F"})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	g := el.(*genPg.MicroflowClientAction)
	s := g.MicroflowSettings().(*genPg.MicroflowSettings)
	if !g.DisabledDuringExecution() || s.ProgressBar() != "None" || s.Asynchronous() ||
		s.FormValidations() != "All" || s.ConfirmationInfo() != nil || s.ProgressMessage() != nil {
		t.Errorf("defaults changed: DDE %v ProgressBar %q Async %v FV %q Confirmation %v Message %v",
			g.DisabledDuringExecution(), s.ProgressBar(), s.Asynchronous(), s.FormValidations(),
			s.ConfirmationInfo(), s.ProgressMessage())
	}
}

func TestClientActionToGen_NanoflowSettings(t *testing.T) {
	el, err := clientActionToGen(&pages.NanoflowClientAction{
		BaseElement:  model.BaseElement{ID: "a"},
		NanoflowName: "M.N",
		FlowCallSettings: pages.FlowCallSettings{
			ProgressBar:  "Blocking",
			Confirmation: &pages.ConfirmationInfo{Question: enText("Go?"), ProceedCaption: enText("Proceed"), CancelCaption: enText("Cancel")},
		},
	})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	g := el.(*genPg.CallNanoflowClientAction)
	if g.ProgressBar() != "Blocking" || g.ConfirmationInfo() == nil {
		t.Errorf("ProgressBar %q ConfirmationInfo %v", g.ProgressBar(), g.ConfirmationInfo())
	}
	if !g.DisabledDuringExecution() {
		t.Error("DisabledDuringExecution default changed")
	}
}

// Every other action type honours DisabledDuringExecution: false as well.
func TestClientActionToGen_DisabledDuringExecutionFalse(t *testing.T) {
	f := false
	off := pages.ActionExecution{DisabledDuringExecution: &f}
	for _, a := range []pages.ClientAction{
		&pages.SaveChangesClientAction{ActionExecution: off},
		&pages.CancelChangesClientAction{ActionExecution: off},
		&pages.ClosePageClientAction{ActionExecution: off},
		&pages.DeleteClientAction{ActionExecution: off},
		&pages.PageClientAction{ActionExecution: off, PageName: "M.P"},
	} {
		el, err := clientActionToGen(a)
		if err != nil {
			t.Fatalf("%T: %v", a, err)
		}
		if disabledDuringExecution(t, el) {
			t.Errorf("%T: DisabledDuringExecution = true; the action said false", a)
		}
	}
}

// storedTextBox is a stored widget whose empty event slots store
// DisabledDuringExecution false, as 946 of PedApp's and TestApp's do.
func storedTextBox(name string) bson.D {
	noAction := func(dde bool) bson.D {
		return bson.D{{Key: "$Type", Value: "Forms$NoAction"}, {Key: "DisabledDuringExecution", Value: dde}}
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "Name", Value: name},
		{Key: "OnChangeAction", Value: bson.D{{Key: "$Type", Value: "Forms$MicroflowAction"}, {Key: "DisabledDuringExecution", Value: false}}},
		{Key: "OnEnterAction", Value: noAction(false)},
		{Key: "OnLeaveAction", Value: noAction(false)},
		{Key: "OnEnterKeyPressAction", Value: noAction(false)},
	}
}

func builtTextBox(name string) *genPg.TextBox {
	g := genPg.NewTextBox()
	g.SetName(name)
	g.SetOnChangeAction(noActionGen())
	g.SetOnEnterAction(noActionGen())
	g.SetOnLeaveAction(noActionGen())
	g.SetOnEnterKeyPressAction(noActionGen())
	return g
}

func TestCarryNoActionExecution(t *testing.T) {
	stored := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Widgets", Value: bson.A{int32(2), storedTextBox("tb1")}},
	}
	g := builtTextBox("tb1")
	other := builtTextBox("tbNew")
	root := genPg.NewDataView()
	root.SetName("dv")
	root.AddWidgets(g)
	root.AddWidgets(other)

	carryNoActionExecutionFrom(stored, root)

	for key, slot := range map[string]any{
		"OnEnterAction": g.OnEnterAction(), "OnLeaveAction": g.OnLeaveAction(),
		"OnEnterKeyPressAction": g.OnEnterKeyPressAction(),
	} {
		if slot.(*genPg.NoClientAction).DisabledDuringExecution() {
			t.Errorf("%s: DisabledDuringExecution = true; the stored NoAction holds false", key)
		}
	}
	// The stored OnChange slot is a microflow call, not a NoAction: the
	// script removed that action, and nothing is carried from a different one.
	if !g.OnChangeAction().(*genPg.NoClientAction).DisabledDuringExecution() {
		t.Error("OnChangeAction: carried from a stored action of another type")
	}
	// Control: a widget with no stored counterpart keeps the writer's value.
	if !other.OnEnterAction().(*genPg.NoClientAction).DisabledDuringExecution() {
		t.Error("tbNew: a new widget took a value from nowhere")
	}
}

// Studio Pro stores OnLeaveAction on every check box, date picker and radio
// button group (PedApp and ako/TestApp: 16, 76 and 4 of 4); the writer left it
// out, so a rewrite deleted the slot. The text box is the control: it always
// wrote one.
func TestWidgetToGen_InputWidgetsWriteOnLeaveAction(t *testing.T) {
	type onLeaver interface{ OnLeaveAction() element.Element }
	for _, w := range []pages.Widget{
		&pages.CheckBox{BaseWidget: pages.BaseWidget{Name: "cb"}},
		&pages.DatePicker{BaseWidget: pages.BaseWidget{Name: "dp"}},
		&pages.RadioButtons{BaseWidget: pages.BaseWidget{Name: "rb"}},
		&pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tb"}},
	} {
		el, err := widgetToGen(w)
		if err != nil {
			t.Fatalf("%T: %v", w, err)
		}
		ol, ok := el.(onLeaver)
		if !ok {
			t.Fatalf("%T has no OnLeaveAction", el)
		}
		if _, ok := ol.OnLeaveAction().(*genPg.NoClientAction); !ok {
			t.Errorf("%T: OnLeaveAction = %T, want the empty NoAction Studio Pro stores", w, ol.OnLeaveAction())
		}
	}
}

// A NoAction whose flag already equals the stored one is left alone. Setting
// the equal value marked it dirty, and inside a pluggable widget's kept Object
// that made the encoder rebuild the Object's lists with today's typed-array
// markers where Studio Pro stored older ones, rewriting an unchanged widget
// (#721 L4). The control is a slot whose flag differs, which is still carried.
func TestCarryNoActionExecution_EqualFlagLeavesTheElementClean(t *testing.T) {
	stored := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Widgets", Value: bson.A{int32(2), storedTextBox("tb1")}},
	}
	decode := func(d bson.D) element.Element {
		raw, err := bson.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		return el
	}

	// The stored widget itself, decoded as a kept Object's children are.
	kept := decode(storedTextBox("tb1"))
	carryNoActionExecutionFrom(stored, kept)
	element.Walk(kept, func(e element.Element) bool {
		if e.IsDirty() {
			t.Errorf("%s: dirtied by carrying the value it already holds", e.TypeName())
		}
		return true
	})

	// Control: a slot holding true where the stored one holds false is carried.
	g := builtTextBox("tb1")
	carryNoActionExecutionFrom(stored, g)
	if g.OnEnterAction().(*genPg.NoClientAction).DisabledDuringExecution() {
		t.Error("OnEnterAction: the differing stored value was not carried")
	}
}
