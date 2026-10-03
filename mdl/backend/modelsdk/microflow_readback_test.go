// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"
)

// ReadBackMicroflow / ReadBackNanoflow return a flow as the reader returns it
// once stored (ako/mxcli#859): what the writer defaults reads back as it
// does from a project, and the flow's graph — objects, flows, annotation
// flows — comes back whole. Nothing is written: the backend has no project.
func TestReadBack_IsTheStoredForm(t *testing.T) {
	b := New()
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000009")},
			Position:    model.Point{X: 100, Y: 100},
		},
		SplitCondition: &microflows.ExpressionSplitCondition{Expression: "true"},
		// The writer stores it on the split, and it reads back as stored.
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	}
	oc := annotatedCollection()
	oc.Objects = append(oc.Objects, split)

	mf, err := b.ReadBackMicroflow(&microflows.Microflow{Name: "F", ObjectCollection: oc})
	if err != nil {
		t.Fatalf("ReadBackMicroflow: %v", err)
	}
	got := mf.ObjectCollection
	if len(got.Objects) != 4 || len(got.Flows) != 1 || len(got.AnnotationFlows) != 1 {
		t.Fatalf("read back %d objects, %d flows, %d annotation flows; want 4, 1, 1",
			len(got.Objects), len(got.Flows), len(got.AnnotationFlows))
	}
	for _, o := range got.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok && s.ErrorHandlingType != microflows.ErrorHandlingTypeRollback {
			t.Errorf("the split reads back with ErrorHandlingType %q; it was written as Rollback", s.ErrorHandlingType)
		}
	}

	nf, err := b.ReadBackNanoflow(&microflows.Nanoflow{Name: "N", ObjectCollection: annotatedCollection()})
	if err != nil {
		t.Fatalf("ReadBackNanoflow: %v", err)
	}
	if n := nf.ObjectCollection; len(n.Objects) != 3 || len(n.AnnotationFlows) != 1 {
		t.Fatalf("nanoflow read back %d objects, %d annotation flows; want 3, 1", len(n.Objects), len(n.AnnotationFlows))
	}
}

// A read back that loses what the writer wrote is refused, not returned: two
// read-back flows compare equal in whatever the reader drops, so a change to
// it would be matched as no change and never written (ako/mxcli#859 — a
// changed `show page … with title` was reported Unchanged).
//
// This used a split's Documentation as the dropped field until the reader
// learned to read it (mendixlabs/mxcli#1267), and no other written-but-unread
// flow field is left to stand in. So the comparison the guard rests on is
// tested directly, on a written document and a read-back that lost a key.
func TestReadBack_RefusesWhatTheReaderDrops(t *testing.T) {
	written, err := bsonv2.Marshal(bsonv2.D{
		{Key: "$Type", Value: "Microflows$ExclusiveSplit"},
		{Key: "Caption", Value: "Big?"},
		{Key: "Documentation", Value: "not read back"},
	})
	if err != nil {
		t.Fatal(err)
	}
	readBack, err := bsonv2.Marshal(bsonv2.D{
		{Key: "$Type", Value: "Microflows$ExclusiveSplit"},
		{Key: "Caption", Value: "Big?"},
		{Key: "Documentation", Value: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sameWritten(written, readBack); err == nil || !strings.Contains(err.Error(), "Documentation") {
		t.Fatalf("sameWritten = %v; want it refused naming the Documentation the read back lost", err)
	}
	// CONTROL: identical documents compare equal, so the refusal above is
	// about the lost value and not about the comparison failing on anything.
	if err := sameWritten(written, written); err != nil {
		t.Fatalf("sameWritten(x, x) = %v", err)
	}
}

// A split's Documentation is written, and now read: the read back carries it
// rather than being refused (mendixlabs/mxcli#1267).
func TestReadBack_SplitDocumentation(t *testing.T) {
	b := New()
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000009")},
			Position:    model.Point{X: 100, Y: 100},
		},
		SplitCondition:    &microflows.ExpressionSplitCondition{Expression: "true"},
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
		Documentation:     "read back",
	}
	oc := annotatedCollection()
	oc.Objects = append(oc.Objects, split)
	mf, err := b.ReadBackMicroflow(&microflows.Microflow{Name: "F", ObjectCollection: oc})
	if err != nil {
		t.Fatalf("ReadBackMicroflow: %v", err)
	}
	for _, o := range mf.ObjectCollection.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok && s.Documentation != "read back" {
			t.Errorf("split Documentation read back as %q", s.Documentation)
		}
	}
}
