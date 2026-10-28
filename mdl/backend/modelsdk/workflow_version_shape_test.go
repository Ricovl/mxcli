// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#1754: every workflow mxcli wrote below Mendix 11.3.0 was broken
// while `mxcli check` and `exec` passed:
//
//   - 10.24 and 11.0: the project no longer loads, `TypeCacheUnknownTypeException
//     … Workflows$StartWorkflowActivity` — the type is introduced in 11.1.0
//     (mendixmodelsdk 4.117.0);
//   - 11.1 and 11.2: CE6671 "An end activity can only be placed at the end of a
//     path." and CE6689 "The 'Workflow end' cannot be reached …" — those
//     versions keep the main flow's start and end implicit (mx convert of a
//     10.24 workflow), and only 11.3 stores them;
//   - below 11.1 every activity carried RelativeMiddlePoint and Size, which
//     Workflows$WorkflowActivity declares from 11.1.0.

func v10(minor int) *types.ProjectVersion {
	return &types.ProjectVersion{MajorVersion: 10, MinorVersion: minor}
}

// shapeWorkflow is a workflow as the executor builds it: an explicit start
// holding the header's note, a call microflow, a decision whose branch ends the
// workflow, and the main flow's closing end.
func shapeWorkflow() *workflows.Workflow {
	branchEnd := &workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Rejected", Caption: "End"}}
	return &workflows.Workflow{
		Name:      "WF",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{
				Name: "Start", Caption: "Start", Annotation: "Approve orders"}},
			&workflows.CallMicroflowTask{
				BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Step", Caption: "Step"},
				Microflow:            "M.ACT_Step",
			},
			&workflows.ExclusiveSplitActivity{
				BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Check", Caption: "Check"},
				Expression:           "true",
				Outcomes: []workflows.ConditionOutcome{
					&workflows.BooleanConditionOutcome{Value: true},
					&workflows.BooleanConditionOutcome{Value: false,
						Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{branchEnd}}},
				},
			},
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "End", Caption: "End"}},
		}},
	}
}

// mainFlowTypes lists the $Type of each main-flow activity, in order.
func mainFlowTypes(t *testing.T, raw bson.Raw) []string {
	t.Helper()
	vals, err := raw.Lookup("Flow", "Activities").Array().Values()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, v := range vals[1:] { // [0] is the list marker
		out = append(out, v.Document().Lookup("$Type").StringValue())
	}
	return out
}

func TestWorkflowEncoder_StartAndEndAreImplicitBelowMendix113(t *testing.T) {
	for _, pv := range []*types.ProjectVersion{v10(24), v11(0), v11(1), v11(2)} {
		raw := encodeWorkflowFor(t, shapeWorkflow(), pv)
		types := mainFlowTypes(t, raw)
		if types[0] == "Workflows$StartWorkflowActivity" {
			t.Errorf("%d.%d: the main flow starts with a StartWorkflowActivity, which this version keeps implicit: %v",
				pv.MajorVersion, pv.MinorVersion, types)
		}
		if types[len(types)-1] == "Workflows$EndWorkflowActivity" {
			t.Errorf("%d.%d: the main flow ends with an EndWorkflowActivity, which this version keeps implicit (CE6671): %v",
				pv.MajorVersion, pv.MinorVersion, types)
		}
		// The start's note is the workflow's own on these versions.
		if got, _ := raw.Lookup("Annotation", "Description").StringValueOK(); got != "Approve orders" {
			t.Errorf("%d.%d: the header note is %q on the workflow, want the start's note", pv.MajorVersion, pv.MinorVersion, got)
		}
		// A branch that ends the workflow keeps its End: EndWorkflowActivity
		// exists from 9.0.2, and it is what ends the path.
		if !strings.Contains(raw.String(), `"Rejected"`) {
			t.Errorf("%d.%d: the branch's End was dropped", pv.MajorVersion, pv.MinorVersion)
		}
	}

	// Control: 11.3 and later store the start and the end, and the note on the
	// start (ako/mxcli#1717); so does an unread version.
	for _, pv := range []*types.ProjectVersion{v11(3), v11(14), nil} {
		raw := encodeWorkflowFor(t, shapeWorkflow(), pv)
		types := mainFlowTypes(t, raw)
		if types[0] != "Workflows$StartWorkflowActivity" || types[len(types)-1] != "Workflows$EndWorkflowActivity" {
			t.Errorf("%v: main flow %v, want it to begin with the start and end with the end", pv, types)
		}
		if _, err := raw.LookupErr("Annotation", "Description"); err == nil {
			t.Errorf("%v: the header note is on the workflow; 11.3 stores it on the start", pv)
		}
	}
}

// hasGeometry reports whether any Workflows$ element in d carries
// RelativeMiddlePoint or Size.
func hasGeometry(v any) bool {
	switch x := v.(type) {
	case bson.D:
		typ := ""
		for _, e := range x {
			if e.Key == "$Type" {
				typ, _ = e.Value.(string)
			}
		}
		for _, e := range x {
			if strings.HasPrefix(typ, "Workflows$") && (e.Key == "RelativeMiddlePoint" || e.Key == "Size") {
				return true
			}
			if hasGeometry(e.Value) {
				return true
			}
		}
	case bson.A:
		for _, el := range x {
			if hasGeometry(el) {
				return true
			}
		}
	}
	return false
}

func TestWorkflowEncoder_NoActivityGeometryBelowMendix111(t *testing.T) {
	for _, pv := range []*types.ProjectVersion{v10(24), v11(0)} {
		var d bson.D
		if err := bson.Unmarshal(encodeWorkflowFor(t, shapeWorkflow(), pv), &d); err != nil {
			t.Fatal(err)
		}
		if hasGeometry(d) {
			t.Errorf("%d.%d: an activity carries RelativeMiddlePoint/Size, which Workflows$WorkflowActivity declares from 11.1.0",
				pv.MajorVersion, pv.MinorVersion)
		}
	}
	// Control: 11.1 declares them, and Studio Pro stores them on every activity.
	var d bson.D
	if err := bson.Unmarshal(encodeWorkflowFor(t, shapeWorkflow(), v11(1)), &d); err != nil {
		t.Fatal(err)
	}
	if !hasGeometry(d) {
		t.Error("11.1: no activity carries RelativeMiddlePoint/Size")
	}
}

// `alter workflow … insert` serializes the new activity on its own, outside
// encodeWorkflow; it carried the geometry below 11.1 too.
func TestWorkflowMutatorActivity_NoGeometryBelowMendix111(t *testing.T) {
	act := &workflows.CallMicroflowTask{
		BaseWorkflowActivity: workflows.BaseWorkflowActivity{Name: "Inserted", Caption: "Inserted"},
		Microflow:            "M.ACT_Step",
	}
	if hasGeometry(serializeWorkflowActivityToBSON(act, v10(24))) {
		t.Error("10.24: an inserted activity carries RelativeMiddlePoint/Size")
	}
	if !hasGeometry(serializeWorkflowActivityToBSON(act, v11(1))) {
		t.Error("control, 11.1: an inserted activity carries no RelativeMiddlePoint/Size")
	}
}
