// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A static OPEN_LINK address is written with Studio Pro's exact key set:
// $ID, $Type, AttributeRef (null), IsDynamic, Value. Measured 21 of 21 static
// Forms$StaticOrDynamicString across ako/TestApp and PedApp, every one storing
// AttributeRef as null. Dropping the key made a describe → exec of any page with
// a link button rewrite the address (ako/mxcli-ledger FINDINGS #170).
func TestStaticAddress_WritesNullAttributeRef(t *testing.T) {
	el, err := clientActionToGen(&pages.LinkClientAction{
		BaseElement: model.BaseElement{ID: "link-id"},
		LinkType:    pages.LinkTypeWeb,
		Address:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doc := encodeElement(t, el.(*genPg.OpenLinkClientAction).Address())
	want := []string{"$ID", "$Type", "AttributeRef", "IsDynamic", "Value"}
	if got := docKeys(doc); !reflect.DeepEqual(got, want) {
		t.Fatalf("static address keys = %v, want %v", got, want)
	}
	for _, e := range doc {
		if e.Key == "AttributeRef" && e.Value != nil {
			t.Errorf("AttributeRef = %v, want null on a static address", e.Value)
		}
	}
}

// CONTROL: the dynamic address keeps its own AttributeRef document — the
// registered null only fills the key when nothing set it.
func TestDynamicAddress_KeepsAttributeRef(t *testing.T) {
	el, err := clientActionToGen(&pages.LinkClientAction{
		BaseElement:      model.BaseElement{ID: "link-id"},
		LinkType:         pages.LinkTypeWeb,
		AddressAttribute: "FeedbackModule.ResponseHelper.URL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doc := encodeElement(t, el.(*genPg.OpenLinkClientAction).Address())
	want := []string{"$ID", "$Type", "AttributeRef", "IsDynamic", "Value"}
	if got := docKeys(doc); !reflect.DeepEqual(got, want) {
		t.Fatalf("dynamic address keys = %v, want %v", got, want)
	}
	for _, e := range doc {
		if e.Key == "AttributeRef" && e.Value == nil {
			t.Error("AttributeRef is null on a dynamic address — the registered default flattened it")
		}
	}
}
