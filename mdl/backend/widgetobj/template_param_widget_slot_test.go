// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#826: a pluggable widget's template parameter read through a data
// view (`$dataView1.Name`) names the data view in the Widget slot, beside the
// data view's own variable — the pair Studio Pro stores.
func TestSerializeColumnClientTemplateParameter_WidgetSlot(t *testing.T) {
	get := func(d bson.D, k string) any {
		for _, e := range d {
			if e.Key == k {
				return e.Value
			}
		}
		return nil
	}
	for _, tc := range []struct {
		name          string
		p             pages.ClientTemplateParameter
		widget, pageP string
	}{
		{"widget", pages.ClientTemplateParameter{AttributeRef: "M.Car.Brand", SourceWidget: "dataView1", SourceVariable: "Car"}, "dataView1", "Car"},
		{"page parameter (control)", pages.ClientTemplateParameter{AttributeRef: "M.Car.Brand", SourceVariable: "Car"}, "", "Car"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sv, ok := get(SerializeColumnClientTemplateParameter(&tc.p), "SourceVariable").(bson.D)
			if !ok {
				t.Fatalf("SourceVariable not a document")
			}
			if got := get(sv, "Widget"); got != tc.widget {
				t.Errorf("Widget = %v, want %q", got, tc.widget)
			}
			if got := get(sv, "PageParameter"); got != tc.pageP {
				t.Errorf("PageParameter = %v, want %q", got, tc.pageP)
			}
		})
	}
}
