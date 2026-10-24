// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// carryNoActionExecution copies DisabledDuringExecution from the stored
// widget's unused action slots onto a pluggable widget the statement changed
// and the engine rebuilt from its template (ako/mxcli#549).
//
// It is the pluggable half of the backend's carryNoActionExecution (#721 L2),
// which pairs a classic widget's slots by the widget's name and cannot reach
// these: a pluggable widget's slots are WidgetValues inside its Object, and
// every one of them holds a Forms$NoAction, action property or not. The
// template writes true on all of them, so where Studio Pro stored false —
// dropDown2's onEnterEvent and onLeaveEvent in ako/TestApp's
// WorkflowCommons.Snip_TaskDashboard_Header — a rewrite that changed the combo
// box turned them true.
//
// Carried, not spelled, for the reason the classic carry gives: a NoAction runs
// nothing, so the flag has no behaviour to ask for. A slot is paired by its
// property key, read through each side's own Type, because the template's
// property type IDs are not the stored ones; only a slot that is a NoAction in
// both is touched, so an action the statement writes keeps what its own
// `with ( … )` says. Object-list items are not paired: they have no name to
// pair on, and a reordered list would carry onto the wrong item.
func (pt *pluggablePassthrough) carryNoActionExecution(def *WidgetDefinition, w *ast.WidgetV3, cw *pages.CustomWidget) {
	if pt == nil || def == nil || w == nil || cw == nil {
		return
	}
	sp, ok := pt.byName[w.Name]
	if !ok || sp.widgetID != def.WidgetID {
		return
	}
	stored := map[string]bool{}
	storedKeys := propertyKeysByTypeID(sp.typ)
	for _, prop := range bsonDocs(bsonValue(sp.obj, "Properties")) {
		action := slotNoAction(prop)
		if v, ok := bsonValue(action, "DisabledDuringExecution").(bool); ok {
			if key := storedKeys[bsonIDKey(bsonValue(prop, "TypePointer"))]; key != "" {
				stored[key] = v
			}
		}
	}
	if len(stored) == 0 {
		return
	}

	builtKeys := propertyKeysByTypeID(cw.RawType)
	for _, prop := range bsonDocs(bsonValue(cw.RawObject, "Properties")) {
		want, ok := stored[builtKeys[bsonIDKey(bsonValue(prop, "TypePointer"))]]
		if !ok {
			continue
		}
		action := slotNoAction(prop)
		if have, ok := bsonValue(action, "DisabledDuringExecution").(bool); ok && have != want {
			bsonnav.DSet(action, "DisabledDuringExecution", want)
		}
	}
}

// propertyKeysByTypeID maps a widget Type's property type IDs to their keys.
func propertyKeysByTypeID(typ bson.D) map[string]string {
	out := map[string]string{}
	for _, pt := range bsonDocs(bsonValue(bsonDocField(typ, "ObjectType"), "PropertyTypes")) {
		out[bsonIDKey(bsonValue(pt, "$ID"))] = bsonString(pt, "PropertyKey")
	}
	return out
}

// slotNoAction is the Forms$NoAction a widget property's value holds, or nil
// when it holds another action or none.
func slotNoAction(prop bson.D) bson.D {
	action := bsonDocField(bsonDocField(prop, "Value"), "Action")
	if bsonString(action, "$Type") != "Forms$NoAction" {
		return nil
	}
	return action
}
