// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#953: a layout's platform is what decides whether the React-client
// rules apply to a page at all — a native page builds a static image and a
// classic drop-down clean (measured, mxbuild 11.13.0). LayoutType cannot tell:
// native layouts store "Default" / "Popup", and "Popup" reads as a web value.
func TestLayoutPlatformColumn_Issue953(t *testing.T) {
	const modID = model.ID("mod-l")
	newLayout := func(id, name string, lt pages.LayoutType, native bool) *pages.Layout {
		l := &pages.Layout{Name: name, LayoutType: lt, Native: native}
		l.ID = model.ID(id)
		l.ContainerID = modID
		return l
	}
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog: cat,
		reader: &mock.MockBackend{
			ListLayoutsFunc: func() ([]*pages.Layout, error) {
				return []*pages.Layout{
					newLayout("l-web", "Atlas_Default", pages.LayoutTypeResponsive, false),
					newLayout("l-native", "NativePhone_Default", pages.LayoutTypeDefault, true),
					newLayout("l-native-popup", "NativePhone_PopOver", pages.LayoutTypePopup, true),
				}, nil
			},
		},
		snapshot: &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{modID: true},
			moduleNames:     map[model.ID]string{modID: "L"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx: tx,
	}
	if err := b.buildLayouts(); err != nil {
		t.Fatalf("buildLayouts: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got := queryStrings(t, cat, `SELECT QualifiedName, Platform FROM layouts_data`)
	want := map[string]string{
		"L.Atlas_Default":       "Web",
		"L.NativePhone_Default": "Native",
		"L.NativePhone_PopOver": "Native",
	}
	for qn, w := range want {
		if got[qn] != w {
			t.Errorf("%s Platform = %q, want %q", qn, got[qn], w)
		}
	}
}
