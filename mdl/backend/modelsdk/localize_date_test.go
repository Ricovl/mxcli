// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genRest "github.com/mendixlabs/mxcli/modelsdk/gen/rest"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A domain-model write re-serializes every entity in the unit, and the writer
// emitted DateTimeAttributeType without LocalizeDate while the reader never read
// it. On the Studio Pro-authored ako/TestApp every DateTime attribute of a
// rewritten domain model lost its LocalizeDate (#743).

func TestAttributeTypeToGen_WritesLocalizeDate(t *testing.T) {
	for _, want := range []bool{true, false} {
		g, ok := attributeTypeToGen(&domainmodel.DateTimeAttributeType{LocalizeDate: want}).(*genDm.DateTimeAttributeType)
		if !ok {
			t.Fatalf("DateTime maps to %T", g)
		}
		if g.LocalizeDate() != want {
			t.Errorf("LocalizeDate written = %v, want %v", g.LocalizeDate(), want)
		}
		if !g.IsDirty() {
			t.Errorf("LocalizeDate=%v was never set, so the property is left out of the BSON", want)
		}
	}
}

func dateTimeTypeFromRaw(t *testing.T, doc bson.D) *genDm.DateTimeAttributeType {
	t.Helper()
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	g := genDm.NewDateTimeAttributeType()
	g.SetRaw(raw)
	g.InitFromRaw(raw)
	return g
}

func TestAttributeTypeFromGen_ReadsLocalizeDate(t *testing.T) {
	for _, stored := range []bool{true, false} {
		g := dateTimeTypeFromRaw(t, bson.D{{Key: "$Type", Value: "DomainModels$DateTimeAttributeType"}, {Key: "LocalizeDate", Value: stored}})
		dt, ok := attributeTypeFromGen(g).(*domainmodel.DateTimeAttributeType)
		if !ok {
			t.Fatalf("read as %T", attributeTypeFromGen(g))
		}
		if dt.LocalizeDate != stored {
			t.Errorf("LocalizeDate read = %v, stored %v", dt.LocalizeDate, stored)
		}
	}
}

// mxcli wrote DateTime attributes without the property until #743, and an absent
// LocalizeDate is Mendix's default, true. Reading it as false would turn every
// such attribute unlocalized on the next write of its domain model.
func TestAttributeTypeFromGen_AbsentLocalizeDateIsTrue(t *testing.T) {
	g := dateTimeTypeFromRaw(t, bson.D{{Key: "$Type", Value: "DomainModels$DateTimeAttributeType"}})
	dt, ok := attributeTypeFromGen(g).(*domainmodel.DateTimeAttributeType)
	if !ok || !dt.LocalizeDate {
		t.Errorf("absent LocalizeDate read as %+v, want true", attributeTypeFromGen(g))
	}
}

// The mapped value's design-time default is read back, or a domain-model write
// empties it on every external attribute (#743).
func TestAttributeFromGen_ODataMappedDefault(t *testing.T) {
	g := genDm.NewAttribute()
	g.SetName("Online")
	mv := genRest.NewODataMappedValue()
	mv.SetRemoteName("Online")
	mv.SetDefaultValueDesignTime("false")
	g.SetValue(mv)
	attr := attributeFromGen(g)
	if attr.Value == nil || attr.Value.DefaultValue != "false" {
		t.Errorf("default = %+v, want \"false\"", attr.Value)
	}
	if out, ok := attributeToGen(attr, true).Value().(*genRest.ODataMappedValue); !ok || out.DefaultValueDesignTime() != "false" {
		t.Errorf("rewritten value = %+v", attributeToGen(attr, true).Value())
	}
}
