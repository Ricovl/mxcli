// SPDX-License-Identifier: Apache-2.0

// The layout twin of #541 and #705: layoutToGen writes the canvas as the
// constant 1280 × 800, so `describe layout` → `exec` of a Studio Pro-authored
// layout with any other canvas rewrote the unit where ADR-0008 would elide the
// write, and reported "Replaced" for a statement that changed nothing. Found
// while proving mendixlabs/mxcli#1411 on Conversational UI's Layout_MasterBase.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func storedLayoutCanvas(t *testing.T, b *Backend, id model.ID) (width, height int64) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return bsonInt(bsonnav.DGet(d, "CanvasWidth")), bsonInt(bsonnav.DGet(d, "CanvasHeight"))
}

func setStoredLayoutCanvas(t *testing.T, b *Backend, id model.ID, width, height any) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bsonnav.DSet(d, "CanvasWidth", width)
	bsonnav.DSet(d, "CanvasHeight", height)
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := b.writer.UpdateRawUnit(string(id), out); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
}

func layoutCanvasFixture(t *testing.T) (*Backend, *pages.Layout) {
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
	l := &pages.Layout{
		ContainerID: mod.ID,
		Name:        "ZzCanvasLayout",
		LayoutType:  pages.LayoutTypeResponsive,
		Widgets:     []pages.Widget{&pages.LayoutPlaceholder{BaseWidget: pages.BaseWidget{Name: "Main"}}},
	}
	if err := b.CreateLayout(l); err != nil {
		t.Fatalf("CreateLayout: %v", err)
	}
	return b, l
}

// A rewrite keeps the stored canvas, at either BSON integer width (Studio Pro
// stores int64; the page fix once matched only int32 and carried nothing).
func TestUpdateLayout_CarriesStoredCanvas(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height any
	}{
		{"int64 (what Studio Pro stores)", int64(1198), int64(640)},
		{"int32", int32(1198), int32(640)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, l := layoutCanvasFixture(t)
			setStoredLayoutCanvas(t, b, l.ID, tc.width, tc.height)

			if err := b.UpdateLayout(l); err != nil {
				t.Fatalf("UpdateLayout: %v", err)
			}
			if w, h := storedLayoutCanvas(t, b, l.ID); w != 1198 || h != 640 {
				t.Errorf("canvas = %d × %d, want 1198 × 640 — the stored canvas was not carried", w, h)
			}
		})
	}
}

// The point of carrying: an unchanged rewrite of a layout whose canvas is not
// 1280 × 800 — every one of PedApp's 22 Atlas layouts — leaves the stored unit
// byte-identical, so the write is elided rather than reported as "Replaced".
func TestUpdateLayout_UnchangedRewriteKeepsStoredBytes(t *testing.T) {
	b, l := layoutCanvasFixture(t)
	setStoredLayoutCanvas(t, b, l.ID, int64(1198), int64(600))
	before, err := b.reader.GetRawUnitBytes(string(l.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateLayout(l); err != nil {
		t.Fatalf("UpdateLayout: %v", err)
	}
	after, err := b.reader.GetRawUnitBytes(string(l.ID))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		w, h := storedLayoutCanvas(t, b, l.ID)
		t.Errorf("an unchanged rewrite changed the stored unit (canvas now %d × %d, was 1198 × 600)", w, h)
	}
}

// Control: a new layout still gets the 1280 × 800 Studio Pro writes, so the
// test above is not passing because nothing ever writes a canvas.
func TestCreateLayout_StillWritesDefaultCanvas(t *testing.T) {
	b, l := layoutCanvasFixture(t)
	if w, h := storedLayoutCanvas(t, b, l.ID); w != 1280 || h != 800 {
		t.Errorf("new layout canvas = %d × %d, want 1280 × 800", w, h)
	}
}
