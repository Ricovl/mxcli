// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#705 item 4 — the snippet twin of #541. snippetToGen wrote Type,
// ExportLevel and the canvas as constants, and the Type constant was "", which
// is not even a member of PagesType (Native | Web): describe → exec of the Blank
// template's FeedbackModule._ReadMe turned Type "Web" into "". None of the four
// has an MDL spelling, so a rewrite carries the stored value and a new snippet
// gets what Studio Pro writes (Web, Hidden, 800 × 600 on all four snippets in
// that template).
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

type snippetHeader struct {
	typ, exportLevel string
	width, height    int64
}

func storedSnippetHeader(t *testing.T, b *Backend, id model.ID) snippetHeader {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return snippetHeader{
		typ:         bsonnav.DGetString(d, "Type"),
		exportLevel: bsonnav.DGetString(d, "ExportLevel"),
		width:       bsonInt(bsonnav.DGet(d, "CanvasWidth")),
		height:      bsonInt(bsonnav.DGet(d, "CanvasHeight")),
	}
}

func snippetFixture(t *testing.T) (*Backend, *pages.Snippet) {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	s := &pages.Snippet{ContainerID: mod.ID, Name: "ZzHeaderSnippet"}
	if err := b.CreateSnippet(s); err != nil {
		t.Fatalf("CreateSnippet: %v", err)
	}
	return b, s
}

func TestCreateSnippet_WritesStudioProHeader(t *testing.T) {
	b, s := snippetFixture(t)
	got := storedSnippetHeader(t, b, s.ID)
	want := snippetHeader{"Web", "Hidden", 800, 600}
	if got != want {
		t.Errorf("new snippet header = %+v, want %+v — \"\" is not a PagesType", got, want)
	}
}

func TestUpdateSnippet_CarriesStoredHeader(t *testing.T) {
	b, s := snippetFixture(t)

	// Stand in for a Studio Pro snippet whose values differ from every default,
	// at the width Studio Pro stores the canvas (int64).
	raw, err := b.reader.GetRawUnitBytes(string(s.ID))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bsonnav.DSet(d, "Type", "Native")
	bsonnav.DSet(d, "ExportLevel", "API")
	bsonnav.DSet(d, "CanvasWidth", int64(1198))
	bsonnav.DSet(d, "CanvasHeight", int64(500))
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := b.writer.UpdateRawUnit(string(s.ID), out); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}

	s.Documentation = "rewritten"
	if err := b.UpdateSnippet(s); err != nil {
		t.Fatalf("UpdateSnippet: %v", err)
	}

	got := storedSnippetHeader(t, b, s.ID)
	want := snippetHeader{"Native", "API", 1198, 500}
	if got != want {
		t.Errorf("rewritten snippet header = %+v, want the stored %+v", got, want)
	}
	// Control: the authored change still lands.
	raw, _ = b.reader.GetRawUnitBytes(string(s.ID))
	var after bson.D
	_ = bson.Unmarshal(raw, &after)
	if doc := bsonnav.DGetString(after, "Documentation"); doc != "rewritten" {
		t.Errorf("Documentation = %q, want rewritten — the authored change was lost", doc)
	}
}
