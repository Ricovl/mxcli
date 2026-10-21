// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1599. On 10+ a gallery's "Database" source reached over an
// association is a CustomWidgets$CustomWidgetXPathSource whose EntityRef is an
// IndirectEntityRef of the hops, with its sort bar beside it — the shape the
// report's bson dump shows. The writer stored only a DirectEntityRef, so the
// hops (and with them the context restriction) could not be written at all.
func TestCustomWidgetXPathSourceToGen_OverAssociation(t *testing.T) {
	el, err := customWidgetDataSourceToGen(&pages.DatabaseSource{
		EntityName: "Probe.Item",
		EntitySteps: []pages.AttributeRefStep{{
			Association: "Probe.Item_Parent", DestinationEntity: "Probe.Item",
		}},
		ContextVariable: "Parent",
		Sorting: []*pages.GridSort{{
			AttributePath: "Probe.Item.Code", Direction: "Ascending",
		}},
	})
	if err != nil {
		t.Fatalf("customWidgetDataSourceToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := lookup(d, "$Type"); got != "CustomWidgets$CustomWidgetXPathSource" {
		t.Fatalf("$Type = %v, want CustomWidgets$CustomWidgetXPathSource", got)
	}
	ref, _ := lookup(d, "EntityRef").(bson.D)
	if lookup(ref, "$Type") != "DomainModels$IndirectEntityRef" {
		t.Fatalf("EntityRef = %v, want an IndirectEntityRef", ref)
	}
	steps, _ := lookup(ref, "Steps").(bson.A)
	if len(steps) != 2 {
		t.Fatalf("Steps = %v, want one hop", steps)
	}
	step, _ := steps[1].(bson.D)
	if lookup(step, "Association") != "Probe.Item_Parent" || lookup(step, "DestinationEntity") != "Probe.Item" {
		t.Errorf("step = %v, want Probe.Item_Parent → Probe.Item", step)
	}
	sv, _ := lookup(d, "SourceVariable").(bson.D)
	if lookup(sv, "PageParameter") != "Parent" {
		t.Errorf("SourceVariable = %v, want page parameter Parent", sv)
	}
	bar, _ := lookup(d, "SortBar").(bson.D)
	if items, _ := lookup(bar, "SortItems").(bson.A); len(items) != 2 {
		t.Errorf("SortBar.SortItems = %v, want one sort item", items)
	}
}
