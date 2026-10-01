// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// decision is a line whose middle node is an expression decision.
func decision(expr, caption string, rule bool) []bson.D {
	objs := line()
	cond := bson.D{{Key: "$ID", Value: bin("cond")}, {Key: "$Type", Value: "Microflows$ExpressionSplitCondition"}, {Key: "Expression", Value: expr}}
	if rule {
		cond = bson.D{{Key: "$ID", Value: bin("cond")}, {Key: "$Type", Value: "Microflows$RuleSplitCondition"}}
	}
	objs[1] = bson.D{
		{Key: "$ID", Value: bin("a")},
		{Key: "$Type", Value: "Microflows$ExclusiveSplit"},
		{Key: "Caption", Value: caption},
		{Key: "RelativeMiddlePoint", Value: "250;200"},
		{Key: "Size", Value: "90;60"},
		{Key: "SplitCondition", Value: cond},
	}
	return objs
}

// ako/mxcli#888: a changed guard condition is set on the stored decision —
// its expression and caption — and nothing else changes.
func TestSplice_SetConditionEditsTheDecisionOnly(t *testing.T) {
	m, deps := newMutator(t, unit(decision("$N <= 0", "$N <= 0", false), lineFlows()))
	if err := m.SetCondition(model.ID(uid("a")), "$N < 1", "$N < 1"); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	raw, _ := bson.Marshal(unit(decision("$N < 1", "$N < 1", false), lineFlows()))
	if !bytes.Equal(deps.saved, raw) {
		t.Error("setting the condition changed more than the decision's expression and caption")
	}

	if err := m.SetCondition(model.ID(uid("b")), "true", "true"); err == nil || !strings.Contains(err.Error(), "only a decision") {
		t.Errorf("want a refusal for an activity, got %v", err)
	}
	r, _ := newMutator(t, unit(decision("", "Valid?", true), lineFlows()))
	if err := r.SetCondition(model.ID(uid("a")), "true", "Valid?"); err == nil || !strings.Contains(err.Error(), "calls a rule") {
		t.Errorf("want a refusal for a rule decision, got %v", err)
	}
}

// The crossing test behind checkBranches: a segment through the inside of the
// area is a crossing; one along its edge, or past it, is not.
func TestSplice_SegmentCrosses(t *testing.T) {
	r := [2]point{{700, 230}, {860, 310}}
	for _, c := range []struct {
		name string
		a, b point
		want bool
	}{
		{"diagonal through it", point{530, 300}, point{915, 200}, true},
		{"horizontal through it", point{700, 280}, point{1000, 280}, true},
		{"along the main line above it", point{700, 200}, point{1000, 200}, false},
		{"along its top edge", point{700, 230}, point{1000, 230}, false},
		{"ending before it", point{500, 280}, point{690, 280}, false},
		{"vertical beside it", point{870, 100}, point{870, 400}, false},
	} {
		if got := segmentCrosses(c.a, c.b, r); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
