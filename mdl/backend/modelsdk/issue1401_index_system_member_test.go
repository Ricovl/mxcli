// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// An index segment on a system date member (changedDate / createdDate) is
// stored as IndexedAttribute.Type = "ChangedDate" with no attribute behind its
// AttributePointer (mendixlabs/mxcli#1401). The reader used to drop Type and
// the writer to hard-code "Normal", so any rewrite of such an entity turned the
// index into an attribute segment pointing at nothing. This pins both halves:
// the encoded segment keeps its Type, and reading it back yields it again.
func TestIssue1401_IndexSegmentTypeRoundTrips(t *testing.T) {
	seg := &domainmodel.IndexAttribute{
		AttributeID: "00000000-0000-0000-0000-000000000000",
		Ascending:   false,
		Type:        "ChangedDate",
	}
	seg.ID = model.ID("3f2a7c8e-1b4d-4e6f-9a0b-1c2d3e4f5a6b")
	idx := &domainmodel.Index{Attributes: []*domainmodel.IndexAttribute{seg}}

	g := indexToGen(idx)
	doc := encodeElement(t, g)
	var segs bson.A
	for _, e := range doc {
		if e.Key == "Attributes" {
			segs, _ = e.Value.(bson.A)
		}
	}
	if len(segs) != 2 {
		t.Fatalf("Attributes = %v, want [marker, one segment]", segs)
	}
	segDoc, _ := segs[1].(bson.D)
	got := map[string]any{}
	for _, e := range segDoc {
		got[e.Key] = e.Value
	}
	if got["Type"] != "ChangedDate" {
		t.Errorf("encoded segment Type = %v, want ChangedDate", got["Type"])
	}
	ptr, ok := got["AttributePointer"].(bson.Binary)
	if !ok || !bytes.Equal(ptr.Data, make([]byte, 16)) {
		t.Errorf("encoded AttributePointer = %#v, want the all-zero GUID", got["AttributePointer"])
	}

	back := indexFromGen(g)
	if len(back.Attributes) != 1 {
		t.Fatalf("read back %d segments, want 1", len(back.Attributes))
	}
	if b := back.Attributes[0]; b.Type != "ChangedDate" || b.Ascending {
		t.Errorf("read back {Type:%q Ascending:%v}, want {ChangedDate false}", b.Type, b.Ascending)
	}
}

// An ordinary attribute segment still writes Type "Normal" — the control that
// the Type now carried is not a constant.
func TestIssue1401_IndexSegmentDefaultsToNormal(t *testing.T) {
	seg := &domainmodel.IndexAttribute{AttributeID: "8a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d", Ascending: true}
	g := indexToGen(&domainmodel.Index{Attributes: []*domainmodel.IndexAttribute{seg}})
	back := indexFromGen(g)
	if back.Attributes[0].Type != "Normal" {
		t.Errorf("attribute segment Type = %q, want Normal", back.Attributes[0].Type)
	}
}
