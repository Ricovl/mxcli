// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
)

// gen exposes Layout.LayoutType(), and it is a phantom: it binds a key a
// Forms$Layout does not carry, so it reads "" for every layout ever written.
// Reading it there made DESCRIBE LAYOUT report all 22 Atlas layouts as
// Responsive, because the describe output defaulted "" to that.
//
// The value lives on the content wrapper. This pins layoutTypeOf to the wrapper
// and covers both platforms.
func TestLayoutTypeOf_ReadsTheContentWrapper(t *testing.T) {
	web := genPg.NewWebLayoutContent()
	web.SetLayoutType("ModalPopup")
	l := genPg.NewLayout()
	l.SetContent(web)
	if got := layoutTypeOf(l); string(got) != "ModalPopup" {
		t.Errorf("web: got %q, want ModalPopup", got)
	}

	native := genPg.NewNativeLayoutContent()
	native.SetLayoutType("Popup")
	l2 := genPg.NewLayout()
	l2.SetContent(native)
	if got := layoutTypeOf(l2); string(got) != "Popup" {
		t.Errorf("native: got %q, want Popup", got)
	}

	// A layout with no content reads empty rather than inventing a default —
	// an empty value means the read failed and must be reported as such.
	if got := layoutTypeOf(genPg.NewLayout()); got != "" {
		t.Errorf("no content: got %q, want empty", got)
	}
}

// ako/mxcli#953: the platform is the content wrapper's TYPE, and the lint rules
// that predict React-client CE0582 need it — a native page builds a static
// image or a classic drop-down clean (measured, 11.13.0). ListLayouts left
// Native false for every layout, so nothing downstream could tell.
func TestLayoutIsNative_ReadsTheContentWrapperType(t *testing.T) {
	l := genPg.NewLayout()
	l.SetContent(genPg.NewNativeLayoutContent())
	if !layoutIsNative(l) {
		t.Error("native content: want native")
	}
	w := genPg.NewLayout()
	w.SetContent(genPg.NewWebLayoutContent())
	if layoutIsNative(w) {
		t.Error("web content: want not native")
	}
	if layoutIsNative(genPg.NewLayout()) {
		t.Error("no content: want not native")
	}
}
