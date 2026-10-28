// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/modelsdk/property"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type listProbe struct {
	element.Base
	name    *property.Primitive[string]
	widgets *property.PartList[element.Element]
}

func newListProbe(typeName string) *listProbe {
	o := &listProbe{}
	o.SetTypeName(typeName)
	o.name = property.NewPrimitive[string]("Name", property.DecodeString)
	o.name.Bind(&o.Base, 0)
	o.widgets = property.NewPartList[element.Element]("Widgets")
	o.widgets.Bind(&o.Base, 1)
	o.SetProperties([]element.Property{o.name, o.widgets})
	o.name.Set("probe")
	return o
}

// A new element whose mandatory list is empty writes the key as the bare
// registered marker, in the property's position; an unregistered owner with
// the same empty list still omits it (Studio Pro stores the key on some owners
// and not others, so it is never a property-name rule).
func TestMandatoryPropertyListEmittedWhenEmpty(t *testing.T) {
	const owner = "Probe$MandatoryWidgets"
	RegisterMandatoryPropertyList(owner, "Widgets", 2)

	b, err := (&Encoder{}).Encode(newListProbe(owner))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if len(d) != 4 || d[3].Key != "Widgets" {
		t.Fatalf("keys = %v, want $ID $Type Name Widgets", d)
	}
	if a, ok := d[3].Value.(bson.A); !ok || len(a) != 1 || a[0] != int32(2) {
		t.Errorf("Widgets = %v, want [2]", d[3].Value)
	}

	b, err = (&Encoder{OmitKeys: map[string]map[string]bool{owner: {"Widgets": true}}}).Encode(newListProbe(owner))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bson.Raw(b).LookupErr("Widgets"); err == nil {
		t.Error("mandatory list emitted despite OmitKeys")
	}

	b, err = (&Encoder{}).Encode(newListProbe("Probe$Unregistered"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bson.Raw(b).LookupErr("Widgets"); err == nil {
		t.Error("an unregistered owner gained an empty Widgets key")
	}
}
