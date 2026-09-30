// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go.mongodb.org/mongo-driver/bson"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// guardGraph is the graph `if $N < 0 then return 0; end if; return $N;`
// builds, with element IDs taken from prefix so two builds differ in every ID.
func guardGraph(prefix string) *microflows.MicroflowObjectCollection {
	id := func(s string) model.ID { return model.ID(prefix + s) }
	obj := func(s string, x, y int) microflows.BaseMicroflowObject {
		return microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: id(s)}, Position: model.Point{X: x, Y: y}}
	}
	flow := func(s, from, to string, cv microflows.CaseValue) *microflows.SequenceFlow {
		return &microflows.SequenceFlow{BaseElement: model.BaseElement{ID: id(s)}, OriginID: id(from), DestinationID: id(to), CaseValue: cv}
	}
	return &microflows.MicroflowObjectCollection{
		BaseElement: model.BaseElement{ID: id("oc")},
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{BaseMicroflowObject: obj("start", 100, 200)},
			&microflows.ExclusiveSplit{BaseMicroflowObject: obj("split", 360, 200),
				SplitCondition: &microflows.ExpressionSplitCondition{BaseElement: model.BaseElement{ID: id("cond")}, Expression: "$N < 0"}},
			&microflows.EndEvent{BaseMicroflowObject: obj("end0", 530, 300), ReturnValue: "0"},
			&microflows.EndEvent{BaseMicroflowObject: obj("endN", 530, 200), ReturnValue: "$N"},
		},
		Flows: []*microflows.SequenceFlow{
			flow("f1", "start", "split", nil),
			flow("f2", "split", "end0", &microflows.ExpressionCase{BaseElement: model.BaseElement{ID: id("c1")}, Expression: "true"}),
			flow("f3", "split", "endN", &microflows.ExpressionCase{BaseElement: model.BaseElement{ID: id("c2")}, Expression: "false"}),
		},
	}
}

// Two builds of one definition are the same graph, whatever IDs each minted
// and in whatever order each lists its objects and flows (ako/mxcli#859).
func TestSameBuiltFlow_IDsAndOrderAreNotADifference(t *testing.T) {
	built, stored := guardGraph("b-"), guardGraph("s-")
	stored.Objects[2], stored.Objects[3] = stored.Objects[3], stored.Objects[2]
	stored.Flows[1], stored.Flows[2] = stored.Flows[2], stored.Flows[1]
	if same, why := sameBuiltFlow(built, stored); !same {
		t.Fatalf("two builds of the same graph differ: %s", why)
	}
}

// Controls: every kind of change a script can make to the graph is a
// difference, so the comparison never reports a changed flow as unchanged.
func TestSameBuiltFlow_ChangesAreDifferences(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(oc *microflows.MicroflowObjectCollection)
		want string
	}{
		{"a changed return value", func(oc *microflows.MicroflowObjectCollection) {
			oc.Objects[3].(*microflows.EndEvent).ReturnValue = "$N + 1"
		}, "ReturnValue"},
		{"a changed condition", func(oc *microflows.MicroflowObjectCollection) {
			oc.Objects[1].(*microflows.ExclusiveSplit).SplitCondition.(*microflows.ExpressionSplitCondition).Expression = "$N <= 0"
		}, "Expression"},
		{"a moved node", func(oc *microflows.MicroflowObjectCollection) {
			oc.Objects[2].(*microflows.EndEvent).Position = model.Point{X: 530, Y: 400}
		}, "nothing stored"},
		{"swapped branches", func(oc *microflows.MicroflowObjectCollection) {
			oc.Flows[1].CaseValue, oc.Flows[2].CaseValue = oc.Flows[2].CaseValue, oc.Flows[1].CaseValue
		}, "CaseValue"},
		{"a redirected flow", func(oc *microflows.MicroflowObjectCollection) {
			oc.Flows[2].DestinationID = oc.Objects[2].GetID()
		}, "no stored flow"},
		{"a redrawn connector", func(oc *microflows.MicroflowObjectCollection) {
			oc.Flows[0].OriginConnectionIndex = 3
		}, "OriginConnectionIndex"},
		{"an extra node", func(oc *microflows.MicroflowObjectCollection) {
			oc.Objects = append(oc.Objects, &microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "extra"}, Position: model.Point{X: 700, Y: 200}}})
		}, "objects"},
	} {
		t.Run(c.name, func(t *testing.T) {
			built, stored := guardGraph("b-"), guardGraph("s-")
			c.edit(built)
			same, why := sameBuiltFlow(built, stored)
			if same {
				t.Fatal("a changed graph compared as the stored one")
			}
			if !strings.Contains(why, c.want) {
				t.Errorf("why = %q, want it to mention %q", why, c.want)
			}
		})
	}
}

// Two nodes drawn at one place cannot be paired by position, so the graph is
// never taken for the stored one: the statement diff decides instead.
func TestSameBuiltFlow_CollidingPositionsNeverMatch(t *testing.T) {
	built, stored := guardGraph("b-"), guardGraph("s-")
	for _, oc := range []*microflows.MicroflowObjectCollection{built, stored} {
		oc.Objects[2].(*microflows.EndEvent).Position = model.Point{X: 530, Y: 200}
	}
	if same, why := sameBuiltFlow(built, stored); same || !strings.Contains(why, "two objects") {
		t.Fatalf("same=%v why=%q, want a refusal to pair colliding nodes", same, why)
	}
}

// soapGraph is guardGraph with a call web service activity whose raw document
// carries the element IDs of its own build, as the reader keeps it whenever the
// call is not one the structured form reproduces.
func soapGraph(t *testing.T, prefix, timeout string) *microflows.MicroflowObjectCollection {
	t.Helper()
	oc := guardGraph(prefix)
	raw, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: prefix + "action"},
		{Key: "$Type", Value: "Microflows$CallWebServiceAction"},
		{Key: "NewResultHandling", Value: bson.D{
			{Key: "$ID", Value: prefix + "rh"},
			{Key: "$Type", Value: "Microflows$ResultHandling"},
			{Key: "VariableType", Value: bson.D{{Key: "$ID", Value: prefix + "vt"}, {Key: "$Type", Value: "DataTypes$VoidType"}}},
		}},
		{Key: "RequestHeaderHandling", Value: bson.D{
			{Key: "$ID", Value: prefix + "hh"},
			{Key: "ParameterMappings", Value: bson.A{int32(2), bson.D{{Key: "$ID", Value: prefix + "pm"}, {Key: "Expression", Value: "1"}}}},
		}},
		{Key: "TimeOutExpression", Value: timeout},
	})
	if err != nil {
		t.Fatal(err)
	}
	oc.Objects = append(oc.Objects, &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(prefix + "act")}, Position: model.Point{X: 700, Y: 200}}},
		Action: &microflows.WebServiceCallAction{BaseElement: model.BaseElement{ID: model.ID(prefix + "action")},
			TimeoutExpression: timeout, RawBSON: raw},
	})
	return oc
}

// A call web service activity's raw document holds the element IDs its build
// minted, so two builds of one statement never had equal bytes, and every
// re-run re-spliced the call (ako/mxcli#861). The IDs are no difference; the
// document's content is.
func TestSameBuiltFlow_WebServiceRawIDsAreNotADifference(t *testing.T) {
	if same, why := sameBuiltFlow(soapGraph(t, "b-", "300"), soapGraph(t, "s-", "300")); !same {
		t.Fatalf("two builds of the same call web service differ: %s", why)
	}
	// Control: a changed value inside the raw document is a difference.
	built, stored := soapGraph(t, "b-", "300"), soapGraph(t, "s-", "300")
	act := built.Objects[len(built.Objects)-1].(*microflows.ActionActivity).Action.(*microflows.WebServiceCallAction)
	act.RawBSON = soapGraph(t, "b-", "30").Objects[4].(*microflows.ActionActivity).Action.(*microflows.WebServiceCallAction).RawBSON
	if same, why := sameBuiltFlow(built, stored); same || !strings.Contains(why, "RawBSON") {
		t.Fatalf("same=%v why=%q: a changed raw call compared as the stored one", same, why)
	}
}
