// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"testing"

	"github.com/mendixlabs/mxcli/generated/metamodel"
)

// The modes a scroll container's LayoutMode takes are the metamodel's, spelled
// as stored (ako/mxcli#1703): a value Studio Pro does not know leaves a
// document it cannot open, and a member missing here refuses a valid one.
func TestScrollContainerLayoutModes_MatchTheMetamodel(t *testing.T) {
	want := []string{string(metamodel.PagesLayoutModeHeadline), string(metamodel.PagesLayoutModeSidebar)}
	if len(ScrollContainerLayoutModes) != len(want) {
		t.Fatalf("ScrollContainerLayoutModes = %v, the metamodel's PagesLayoutMode is %v", ScrollContainerLayoutModes, want)
	}
	for i, m := range want {
		if ScrollContainerLayoutModes[i] != m {
			t.Errorf("ScrollContainerLayoutModes[%d] = %q, metamodel %q", i, ScrollContainerLayoutModes[i], m)
		}
		if got, ok := CanonicalScrollContainerLayoutMode(m); !ok || got != m {
			t.Errorf("CanonicalScrollContainerLayoutMode(%q) = %q, %v", m, got, ok)
		}
	}
	if DefaultScrollContainerLayoutMode != string(metamodel.PagesLayoutModeHeadline) {
		t.Errorf("default %q is not a member", DefaultScrollContainerLayoutMode)
	}
	if got, ok := CanonicalScrollContainerLayoutMode("sidebar"); !ok || got != "Sidebar" {
		t.Errorf("CanonicalScrollContainerLayoutMode(sidebar) = %q, %v — want the stored spelling", got, ok)
	}
	if _, ok := CanonicalScrollContainerLayoutMode("Topbar"); ok {
		t.Error("Topbar is not a LayoutMode")
	}
}
