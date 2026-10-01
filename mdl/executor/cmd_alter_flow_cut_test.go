// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#888: an end event in a fragment that no `return` drew is one the
// builder added to end an error handler, and the splice refuses it. The
// refusal must not tell the user to "end that path with a return": a handler
// that does state one (`on error … { return false; }`) draws exactly this end
// event, so that advice cannot be followed.
func TestCutFragment_HandlerEndRefusalDoesNotAskForAReturn(t *testing.T) {
	start := &microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: "start"}}}
	act := &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: "act"}}}}
	handlerEnd := &microflows.EndEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: "handler"}}, ReturnValue: "false"}
	end := &microflows.EndEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: "end"}}}
	oc := &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
		start, act, handlerEnd, end,
	}}
	_, err := cutFragment(oc, "end", map[model.ID]bool{})
	if err == nil {
		t.Fatal("want the handler's end event refused")
	}
	if !strings.Contains(err.Error(), "error handler") {
		t.Errorf("the refusal does not name the error handler: %v", err)
	}
	if strings.Contains(err.Error(), "end that path with a return") || strings.Contains(err.Error(), "states no return") {
		t.Errorf("the refusal asks for a return, which a handler that returns already states: %v", err)
	}
}
