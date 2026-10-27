// SPDX-License-Identifier: Apache-2.0

package wfmutator

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// withNullAnnotation adds the `Annotation: null` every workflow and activity
// type carrying a note stores when it has none (11.15.0).
func withNullAnnotation(d bson.D) bson.D {
	return append(d, bson.E{Key: "Annotation", Value: nil})
}

// assertNote checks d's Annotation is a Workflows$Annotation holding text, in
// Studio Pro's exact property set ($ID, $Type, Description).
func assertNote(t *testing.T, d bson.D, text string) bson.D {
	t.Helper()
	note := bsonnav.DGetDoc(d, "Annotation")
	if note == nil {
		t.Fatalf("Annotation = %v, want a note", bsonnav.DGet(d, "Annotation"))
	}
	var keys []string
	for _, e := range note {
		keys = append(keys, e.Key)
	}
	if strings.Join(keys, ",") != "$ID,$Type,Description" {
		t.Errorf("note keys = %v, want $ID,$Type,Description", keys)
	}
	if bsonnav.DGetString(note, "$Type") != "Workflows$Annotation" || bsonnav.DGetString(note, "Description") != text {
		t.Errorf("note = %v, want Workflows$Annotation %q", note, text)
	}
	return note
}

// startDoc is a workflow whose flow starts with a start activity carrying the
// Annotation slot, and whose own Annotation holds legacy, as mxcli wrote the
// header's note before ako/mxcli#1717.
func startDoc(legacy any) bson.D {
	start := withNullAnnotation(makeWfActivity("Workflows$StartWorkflowActivity", "Start", "Start"))
	return append(makeWorkflowDoc(start), bson.E{Key: "Annotation", Value: legacy})
}

func startActivity(t *testing.T, m *Mutator) bson.D {
	t.Helper()
	d, err := m.findActivityByCaption("Start", 0)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// ako/mxcli#1717: `set (Annotation: '…')` writes the note on the workflow's
// start — the start activity's, where Studio Pro shows it, and what CREATE's
// header `annotation '…'` writes — clears the workflow's own, and the empty
// string removes it.
func TestWorkflowMutator_SetProperty_Annotation(t *testing.T) {
	legacy := bson.D{{Key: "$ID", Value: bson.Binary{Data: make([]byte, 16)}},
		{Key: "$Type", Value: "Workflows$Annotation"}, {Key: "Description", Value: "Old"}}
	m := newMutator(startDoc(legacy))
	if err := m.SetProperty("annotation", "Started from the request form"); err != nil {
		t.Fatal(err)
	}
	first := assertNote(t, startActivity(t, m), "Started from the request form")
	if v, present := lookup(m.rawData, "Annotation"); !present || v != nil {
		t.Errorf("the workflow's own Annotation = %v (present %v), want null", v, present)
	}

	// Editing a note keeps its $ID: the unit is rewritten, the element is not.
	if err := m.SetProperty("annotation", "Reworded"); err != nil {
		t.Fatal(err)
	}
	second := assertNote(t, startActivity(t, m), "Reworded")
	id1, _ := bsonnav.DGet(first, "$ID").(bson.Binary)
	id2, _ := bsonnav.DGet(second, "$ID").(bson.Binary)
	if !bytes.Equal(id1.Data, id2.Data) {
		t.Errorf("editing the note changed its $ID")
	}

	if err := m.SetProperty("annotation", ""); err != nil {
		t.Fatal(err)
	}
	if v, present := lookup(startActivity(t, m), "Annotation"); !present || v != nil {
		t.Errorf("after removing: Annotation = %v (present %v), want null", v, present)
	}
}

// `on Start` is the same note, set the same way.
func TestWorkflowMutator_SetActivityProperty_AnnotationOnStart(t *testing.T) {
	m := newMutator(startDoc("ignored"))
	if err := m.SetActivityProperty("Start", 0, "annotation", "From the form"); err != nil {
		t.Fatal(err)
	}
	assertNote(t, startActivity(t, m), "From the form")
	if v, _ := lookup(m.rawData, "Annotation"); v != nil {
		t.Errorf("the workflow's own Annotation = %v, want null", v)
	}
}

// A start activity stored without an Annotation slot (a version without
// workflow notes) is refused, not given a property its version does not
// declare.
func TestWorkflowMutator_SetProperty_AnnotationNeedsTheSlot(t *testing.T) {
	m := newMutator(makeWorkflowDoc(makeWfActivity("Workflows$StartWorkflowActivity", "Start", "Start")))
	if err := m.SetProperty("annotation", "x"); err == nil {
		t.Fatal("a start activity without an Annotation slot must be refused")
	}
	if _, present := lookup(startActivity(t, m), "Annotation"); present {
		t.Error("the refused set wrote an Annotation")
	}
	if err := newMutator(makeWorkflowDoc()).SetProperty("annotation", "x"); err == nil {
		t.Error("a workflow without a start activity must be refused")
	}
}

// `set (Annotation: '…') on X` writes the activity's note, the one
// `@annotation '…'` before it writes.
func TestWorkflowMutator_SetActivityProperty_Annotation(t *testing.T) {
	act := withNullAnnotation(makeWfActivity("Workflows$CallMicroflowActivity", "Log it", "ACT_Log"))
	m := newMutator(makeWorkflowDoc(act))
	if err := m.SetActivityProperty("ACT_Log", 0, "annotation", "Logs it"); err != nil {
		t.Fatal(err)
	}
	actDoc, _ := m.findActivityByCaption("ACT_Log", 0)
	assertNote(t, actDoc, "Logs it")

	if err := m.SetActivityProperty("ACT_Log", 0, "annotation", ""); err != nil {
		t.Fatal(err)
	}
	actDoc, _ = m.findActivityByCaption("ACT_Log", 0)
	if v, present := lookup(actDoc, "Annotation"); !present || v != nil {
		t.Errorf("after removing: Annotation = %v (present %v), want null", v, present)
	}
}

// Studio Pro draws no note on an end, and describe prints none, so a note on
// one is refused; so is an activity stored without the Annotation slot.
func TestWorkflowMutator_SetActivityProperty_AnnotationRefusals(t *testing.T) {
	for _, tc := range []struct{ typeName, name, want string }{
		{"Workflows$EndWorkflowActivity", "End", "end"},
		{"Workflows$EndOfParallelSplitPathActivity", "PathEnd", "end"},
		{"Workflows$CallMicroflowActivity", "NoSlot", "no annotation"},
	} {
		act := makeWfActivity(tc.typeName, "", tc.name)
		if tc.name != "NoSlot" {
			act = withNullAnnotation(act)
		}
		m := newMutator(makeWorkflowDoc(act))
		err := m.SetActivityProperty(tc.name, 0, "annotation", "x")
		if err == nil {
			t.Errorf("%s: accepted", tc.typeName)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q does not mention %q", tc.typeName, err, tc.want)
		}
		actDoc, _ := m.findActivityByCaption(tc.name, 0)
		if v, _ := lookup(actDoc, "Annotation"); v != nil {
			t.Errorf("%s: the refused set wrote %v", tc.typeName, v)
		}
	}
}

// lookup reports a key's value and whether the key is present at all, which
// bsonnav.DGet cannot tell apart for a null.
func lookup(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}
