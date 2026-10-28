// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// An input widget's event actions beside `OnChange:` — Studio Pro's Events
// section (ako/mxcli#1743, mendixlabs/mxcli#1662).
//
// A text box's "On enter key press" is stored as
// Forms$TextBox.OnEnterKeyPressAction. MDL had no property for it: every
// spelling was refused as a key mxcli does not write, the writer hard-coded a
// NoAction into the slot, and describe printed nothing — so a search box that
// searches on Enter could not be generated, and describe → exec of a Studio
// Pro one removed the action with mx check clean.
//
//	textbox tbSearch (Attribute: SearchText,
//	  OnEnterKeyPress: call microflow M.ACT_Search(Query = $currentObject))
//
// "On enter" and "On leave" (focus in, focus out) were in the same state on
// every input widget — OnEnterAction / OnLeaveAction, written as NoActions and
// never read — so they are spelled the same way: `OnEnter:` and `OnLeave:`.
// "On enter" is the focus event, not the Enter key.
//
// The MDL name is Studio Pro's label in the shape of `OnChange:`; the stored
// name (`OnEnterKeyPressAction:`) is accepted as the same property, and both
// at once is refused — choosing one would drop the other. Each takes the
// action forms `OnChange:` takes.
//
// Which widgets have a slot is the metamodel's answer, not Studio Pro's
// labels': gen gives OnEnterKeyPressAction to Forms$TextBox alone, a text area
// included in the widgets without it. On a widget without the slot the key is
// refused with that reason (inputEventHint) rather than dropped.
//
// There is no grammar keyword: the key reaches the widget through the generic
// `key: action` branch of widgetPropertyV3, where a flow call written without
// `call` (or a `$name`) parses as a data source — namedActionSlotValue's case,
// converted the same way. The keys are kept out of staticWidgetKnownProps
// (the union every widget type shares) and out of the cmd_pages_builder*
// files, as the submit keys in input_submit_width.go are; the builders name
// them by literal at each call, which is what the per-kind source scan reads.

// inputEventAction is one event action an input widget stores.
type inputEventAction struct {
	key    string          // the MDL name: Studio Pro's label
	stored string          // the stored property name, accepted as the same property
	what   string          // for messages: "an on-enter-key-press action"
	kinds  map[string]bool // the widget keywords whose stored type has the slot
}

// focusEventKinds are the input widgets that store OnEnterAction and
// OnLeaveAction: every attribute-bound input mxcli writes.
var focusEventKinds = map[string]bool{
	"textbox": true, "textarea": true, "datepicker": true, "dropdown": true, "checkbox": true, "radiobuttons": true,
}

// inputEventActions are Studio Pro's order: On enter, On leave, On enter key
// press. "On enter" is focus entering the widget, not the Enter key.
var inputEventActions = []inputEventAction{
	{key: "OnEnter", stored: "OnEnterAction", what: "an on-enter action", kinds: focusEventKinds},
	{key: "OnLeave", stored: "OnLeaveAction", what: "an on-leave action", kinds: focusEventKinds},
	{key: "OnEnterKeyPress", stored: "OnEnterKeyPressAction", what: "an on-enter-key-press action",
		kinds: map[string]bool{"textbox": true}},
}

// isInputEventStoredName reports whether name is the stored-name spelling of
// an event action, which messages leave out in favour of the MDL name.
func isInputEventStoredName(name string) bool {
	for _, ev := range inputEventActions {
		if strings.EqualFold(name, ev.stored) {
			return true
		}
	}
	return false
}

// inputEventPick resolves the two spellings of one event action to the key it
// was written under and its value; ("", nil, nil) when neither is set.
func inputEventPick(key string, canon any, hasCanon bool, stored string, alias any, hasAlias bool) (string, any, error) {
	switch {
	case hasCanon && hasAlias:
		return "", nil, fmt.Errorf("both `%s:` and `%s:` are set — they are one property; "+
			"write it once, as `%s:`", key, stored, key)
	case hasAlias:
		return stored, alias, nil
	case hasCanon:
		return key, canon, nil
	}
	return "", nil, nil
}

// inputEventValue converts the value written under key to an action: an
// action as parsed, or a flow call the grammar read as a data source.
func inputEventValue(key string, raw any) (*ast.ActionV3, error) {
	switch v := raw.(type) {
	case *ast.ActionV3:
		return v, nil
	case *ast.DataSourceV3:
		return actionFromFlowDataSource(v, key)
	}
	return nil, fmt.Errorf("`%s: %s` is not an action — the widget would be written with no "+
		"action in that slot at all", key, renderActionSlotValue(raw))
}

// buildInputEventAction reads one event action of w, by its MDL name and its
// stored name, and builds it through the builder every page action goes
// through. nil when w sets none. Callers pass the two names as literals.
func (pb *pageBuilder) buildInputEventAction(w *ast.WidgetV3, key, stored string) (pages.ClientAction, error) {
	canon, hasCanon := lookupPropCI(w, key)
	alias, hasAlias := lookupPropCI(w, stored)
	at, raw, err := inputEventPick(key, canon, hasCanon, stored, alias, hasAlias)
	if err == nil && raw != nil {
		var action *ast.ActionV3
		if action, err = inputEventValue(at, raw); err == nil {
			return pb.buildClientActionV3(action)
		}
	}
	if err != nil {
		return nil, mdlerrors.NewValidationf("%s %s: %s", strings.ToLower(w.Type), w.Name, err.Error())
	}
	return nil, nil
}

// applyInputFocusEventsV3 reads an input widget's "On enter" and "On leave"
// actions into onEnter and onLeave. Every input builder calls it: the slots
// are on every input, and before ako/mxcli#1743 nothing read them, so
// describe → exec of a Studio Pro input deleted a stored one.
func (pb *pageBuilder) applyInputFocusEventsV3(w *ast.WidgetV3, onEnter, onLeave *pages.ClientAction) error {
	var err error
	if *onEnter, err = pb.buildInputEventAction(w, "OnEnter", "OnEnterAction"); err != nil {
		return err
	}
	*onLeave, err = pb.buildInputEventAction(w, "OnLeave", "OnLeaveAction")
	return err
}

// inputEventHint is nativeWidgetPropHint's answer for a key that names an
// event action, or "" for any other key. Where the kind has the action it
// points a near miss (`OnEnterKey:`) at the property; where it has not, it
// says why the property is not there.
func inputEventHint(kind, key string) string {
	l := strings.ToLower(key)
	for _, ev := range inputEventActions {
		if l == strings.ToLower(ev.key) || l == strings.ToLower(ev.stored) {
			if ev.kinds[kind] {
				return ""
			}
			return fmt.Sprintf("only %s has %s: Mendix stores %s on %s and on no other widget, %s included",
				inputEventOwners(ev), ev.what, ev.stored, inputEventStoredTypes(ev), nativeWidgetKinds[kind].stored)
		}
	}
	if (strings.Contains(l, "enterkey") || strings.Contains(l, "keypress")) && kind == "textbox" {
		return "the enter-key action is `OnEnterKeyPress:`"
	}
	return ""
}

// inputEventOwners names the widgets that have ev, for a message.
func inputEventOwners(ev inputEventAction) string {
	if len(ev.kinds) == 1 && ev.kinds["textbox"] {
		return "a text box"
	}
	return "an input widget"
}

// inputEventStoredTypes lists the stored types that have ev, for a message.
func inputEventStoredTypes(ev inputEventAction) string {
	var types []string
	for kind := range ev.kinds {
		types = append(types, nativeWidgetKinds[kind].stored)
	}
	sort.Strings(types)
	return strings.Join(types, ", ")
}
