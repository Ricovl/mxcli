// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#888: an end event in a fragment that no `return` drew is one the
// builder added to end an error handler, and the splice refuses it, naming the
// handler. (A handler that states its own return draws an end event the cut
// keeps since ako/mxcli#905: TestCutFragment_HandlerReturnIsANewEndEvent.)
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
	if strings.Contains(err.Error(), "not spliced yet") {
		t.Errorf("the refusal says a handler's return is not spliced, which it is: %v", err)
	}
}
