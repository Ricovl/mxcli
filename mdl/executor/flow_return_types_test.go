// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// FlowReturnTypes gives `fmt --upgrade -p` the answer the flow builder acts
// on: a call's result is a String exactly when addListOperationAction turns
// `find(…)` / `contains(…)` over it into the string function (ako/mxcli#860).
func TestFlowReturnTypes_AnswersAsTheBuilderBuilds(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-1"}, Name: "M"}
	b := &mock.MockBackend{
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(string) (*model.Module, error) { return mod, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				{ContainerID: mod.ID, Name: "Label", ReturnType: &microflows.StringType{}},
				{ContainerID: mod.ID, Name: "Regions", ReturnType: &microflows.ListType{EntityQualifiedName: "M.E"}},
				{ContainerID: mod.ID, Name: "Count", ReturnType: &microflows.IntegerType{}},
			}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{{ContainerID: mod.ID, Name: "NLabel", ReturnType: &microflows.StringType{}}}, nil
		},
	}
	r := NewFlowReturnTypes(b)
	for _, c := range []struct {
		nanoflow        bool
		name            string
		isString, found bool
	}{
		{false, "M.Label", true, true},
		{false, "M.Regions", false, true},
		{false, "M.Count", false, true},
		{false, "M.Missing", false, false},
		{true, "M.NLabel", true, true},
		{true, "M.Label", false, false}, // a microflow is not a nanoflow
	} {
		isString, found := r.ReturnsString(c.nanoflow, c.name)
		if isString != c.isString || found != c.found {
			t.Errorf("%s (nanoflow %v): got (%v, %v), want (%v, %v)", c.name, c.nanoflow, isString, found, c.isString, c.found)
		}

		// The builder's own reading of the same call, for the String case:
		// the operation it builds is the string function's Change variable.
		if !found {
			continue
		}
		fb := &flowBuilder{varTypes: map[string]string{}, declaredVars: map[string]string{}, backend: b}
		call := ast.QualifiedName{Module: "M", Name: c.name[2:]}
		if c.nanoflow {
			fb.addCallNanoflowAction(&ast.CallNanoflowStmt{OutputVariable: "R", NanoflowName: call})
		} else {
			fb.addCallMicroflowAction(&ast.CallMicroflowStmt{OutputVariable: "R", MicroflowName: call})
		}
		if got := fb.declaredVars["R"] == "String"; got != isString {
			t.Errorf("%s: the builder reads the call's result as String=%v, FlowReturnTypes as %v", c.name, got, isString)
		}
	}
}

// The same questions against Studio Pro-authored flows, through the fast path
// (a stored unit read by name) the mock above does not reach.
func TestFlowReturnTypes_PedApp(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	r := NewFlowReturnTypes(exec.newExecContext(context.Background()).Backend)
	for _, c := range []struct {
		nanoflow        bool
		name            string
		isString, found bool
	}{
		{false, "FeedbackModule.ConvertUUIDToURL", true, true},
		{false, "FeedbackModule.SUB_Feedback_PostToAppInsights", false, true}, // an object
		{false, "FeedbackModule.VAL_Feedback", false, true},                   // a Boolean
		{true, "Atlas_Web_Content.DS_LoginContext", false, true},
		{false, "Atlas_Web_Content.DS_LoginContext", false, false}, // a nanoflow, not a microflow
		{false, "FeedbackModule.NoSuchFlow", false, false},
	} {
		if isString, found := r.ReturnsString(c.nanoflow, c.name); isString != c.isString || found != c.found {
			t.Errorf("%s (nanoflow %v): got (%v, %v), want (%v, %v)", c.name, c.nanoflow, isString, found, c.isString, c.found)
		}
	}
}
