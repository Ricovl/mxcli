// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

type stubDataSourceSerializer struct{}

func (stubDataSourceSerializer) SerializeWidget(pages.Widget) bson.D             { return nil }
func (stubDataSourceSerializer) SerializeClientAction(pages.ClientAction) bson.D { return nil }
func (stubDataSourceSerializer) SerializeCustomWidgetDataSource(pages.DataSource) bson.D {
	return bson.D{{Key: "$Type", Value: "stub"}}
}

func fieldOf(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// mendixlabs/mxcli#1399. Before Mendix 10 a pluggable widget's database source
// declares no SourceVariable; Studio Pro stores the parameter an association
// path starts from on the WidgetValue beside it (FSM template, 9.24.2:
// repeater1's value holds SnippetParameter "NativeInstanceForm"). Written
// without it, the path starts from the enclosing object instead.
func TestSetDataSourceStoresTheStartParameterOnTheWidgetValue(t *testing.T) {
	old := pkgChildSer
	pkgChildSer = stubDataSourceSerializer{}
	defer func() { pkgChildSer = old }()

	val := bson.D{{Key: "DataSource", Value: nil}, {Key: "SourceVariable", Value: nil}}
	got := setDataSource(val, &pages.DatabaseSource{
		UseConstraints: true, ContextVariable: "NativeInstanceForm", IsSnippetParameter: true,
		EntitySteps: []pages.AttributeRefStep{{Association: "M.A_B", DestinationEntity: "M.A"}},
	})
	sv, ok := fieldOf(got, "SourceVariable").(bson.D)
	if !ok {
		t.Fatalf("SourceVariable = %#v, want a Forms$PageVariable", fieldOf(got, "SourceVariable"))
	}
	if fieldOf(sv, "SnippetParameter") != "NativeInstanceForm" || fieldOf(sv, "PageParameter") != "" {
		t.Errorf("SourceVariable = %v, want SnippetParameter NativeInstanceForm", sv)
	}

	// From the enclosing object: no SourceVariable, as stored for repeater3.
	got = setDataSource(val, &pages.DatabaseSource{
		UseConstraints: true,
		EntitySteps:    []pages.AttributeRefStep{{Association: "M.A_B", DestinationEntity: "M.A"}},
	})
	if sv := fieldOf(got, "SourceVariable"); sv != nil {
		t.Errorf("SourceVariable = %v, want null", sv)
	}
}
