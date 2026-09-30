// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// FlowReturnTypes answers, for `mxcli fmt --upgrade -p app.mpr`, whether a
// call to a project's microflow or nanoflow yields a String — the question
// that decides whether mdl 0 builds `find(…)` / `contains(…)` over the call's
// result as the string function or the List operation (ako/mxcli#860).
//
// It asks the flow builder's own lookup and typing (lookupMicroflowReturnType,
// registerResultVariableType) rather than a copy of them, so the upgrade reads
// the call the way exec builds it: one question, one answer.
type FlowReturnTypes struct {
	fb *flowBuilder
}

// NewFlowReturnTypes reads flow return types from a connected backend.
func NewFlowReturnTypes(b backend.FullBackend) *FlowReturnTypes {
	fb := &flowBuilder{backend: b, varTypes: map[string]string{}, declaredVars: map[string]string{}}
	if h, err := NewContainerHierarchyFromBackend(b); err == nil {
		fb.hierarchy = h
	}
	return &FlowReturnTypes{fb: fb}
}

// ReturnsString reports whether the result of calling the microflow (the
// nanoflow when nanoflow is set) named qualifiedName is a String to the flow
// builder. found is false when the project has no such flow.
func (r *FlowReturnTypes) ReturnsString(nanoflow bool, qualifiedName string) (isString, found bool) {
	var dt microflows.DataType
	if nanoflow {
		dt = r.fb.lookupNanoflowReturnType(qualifiedName)
	} else {
		dt = r.fb.lookupMicroflowReturnType(qualifiedName)
	}
	if dt == nil {
		return false, false
	}
	const probe = "result"
	delete(r.fb.varTypes, probe)
	delete(r.fb.declaredVars, probe)
	r.fb.registerResultVariableType(probe, dt)
	// The condition addListOperationAction tests.
	return r.fb.declaredVars[probe] == "String", true
}
