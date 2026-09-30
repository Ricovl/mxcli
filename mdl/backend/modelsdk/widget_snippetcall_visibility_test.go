// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Forms$SnippetCallWidget has no ConditionalVisibilitySettings property — the
// generated type carries none, and all 139 snippet calls Studio Pro stored in
// PedApp and TestApp lack the key. The writer registered it as a null field, so
// every snippet call it wrote added the key: GetPut broke on each snippet call
// once #826 let those documents execute at all.
func TestSnippetCallWidgetHasNoVisibilityKey(t *testing.T) {
	sc := &pages.SnippetCallWidget{SnippetName: "M.Snip"}
	sc.Name = "sc1"
	doc := encodeWidget(t, sc)
	for _, e := range doc {
		if e.Key == "ConditionalVisibilitySettings" {
			t.Errorf("snippet call wrote ConditionalVisibilitySettings = %#v; Studio Pro stores no such key", e.Value)
		}
	}
	// Control: a container has the property and keeps its null.
	lv := encodeWidget(t, &pages.Container{BaseWidget: pages.BaseWidget{Name: "c1"}})
	found := false
	for _, e := range lv {
		found = found || e.Key == "ConditionalVisibilitySettings"
	}
	if !found {
		t.Error("control: a container lost its ConditionalVisibilitySettings key")
	}
}
