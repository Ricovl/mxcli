// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func callActivity(eh microflows.ErrorHandlingType) *microflows.ActionActivity {
	a := &microflows.ActionActivity{Action: &microflows.MicroflowCallAction{
		ErrorHandlingType: eh,
		MicroflowCall:     &microflows.MicroflowCall{Microflow: "M.SUB"},
	}}
	a.ID = "act-1"
	return a
}

// mendixlabs/mxcli#698. The MCP mapper built the action without its
// errorHandlingType, so `call microflow … on error rollback|continue` authored
// over --mcp landed with PED's default — the clause silently lost. The PED
// schema (ped_get_schema, Studio Pro 11.14) declares errorHandlingType on every
// action element: 'Rollback' | 'Custom' | 'CustomWithoutRollBack' | 'Continue'
// | 'Abort'.
func TestMapObjectTree_CarriesErrorHandlingType(t *testing.T) {
	b := &Backend{}
	for _, eh := range []microflows.ErrorHandlingType{microflows.ErrorHandlingTypeRollback, microflows.ErrorHandlingTypeContinue} {
		m, err := b.mapObjectTree(callActivity(eh), "/objects/0", map[model.ID]string{})
		if err != nil {
			t.Fatalf("%s: %v", eh, err)
		}
		action := m["action"].(map[string]any)
		if action["errorHandlingType"] != string(eh) {
			t.Errorf("%s: action errorHandlingType = %v", eh, action["errorHandlingType"])
		}
	}
	// No clause: nothing written, PED's default applies — as before.
	m, err := b.mapObjectTree(callActivity(""), "/objects/0", map[model.ID]string{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["action"].(map[string]any)["errorHandlingType"]; ok {
		t.Error("an action with no error handling wrote one")
	}
}

// A custom handler needs an error-handler flow, and PED's SequenceFlow has no
// property for one (ped_get_schema: originId, destinationId, sides, caseValue).
// Written anyway, the handler's error edge became an ordinary flow. Refused
// rather than dropped.
func TestMapObjectTree_RefusesACustomErrorHandler(t *testing.T) {
	b := &Backend{}
	for _, eh := range []microflows.ErrorHandlingType{microflows.ErrorHandlingTypeCustom, microflows.ErrorHandlingTypeCustomWithoutRollback} {
		if _, err := b.mapObjectTree(callActivity(eh), "/objects/0", map[model.ID]string{}); err == nil || !strings.Contains(err.Error(), "error handler") {
			t.Errorf("%s: accepted (%v)", eh, err)
		}
	}
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{callActivity(microflows.ErrorHandlingTypeRollback), func() microflows.MicroflowObject {
			e := &microflows.EndEvent{}
			e.ID = "end-1"
			return e
		}()},
		Flows: []*microflows.SequenceFlow{{OriginID: "act-1", DestinationID: "end-1", IsErrorHandler: true}},
	}
	if _, err := b.buildFlowDocContent("microflow", "MF", nil, oc, &microflows.VoidType{}); err == nil || !strings.Contains(err.Error(), "error handler") {
		t.Errorf("an error-handler flow was written as an ordinary one (%v)", err)
	}
}
