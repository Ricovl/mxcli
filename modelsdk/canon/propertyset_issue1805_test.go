// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1805: on a Mendix 11.14.0 project mxcli wrote, `fix property-sets`
// reported seven names the elements of a type disagree on, among them
//
//	MIXED  DomainModels$NoGeneralization.HasOwnerAttr: on 81 of 84 element(s); without it: bf8b2a01-… [DomainModels$DomainModel]
//	MIXED  DomainModels$DateTimeAttributeType.LocalizeDate: on 15 of 16 element(s); without it: bf8b2a01-… [DomainModels$DomainModel]
//
// and left them, because the table had no row to fill them from: the writers of
// today emit every one of them, so no measurement of what they write had seen
// one missing. The elements without them were stored by an older mxcli and
// passed through every later rewrite of their unit untouched.
//
// Each value is what `mx convert -p -s` wrote back on 10.24.24, 11.13.0 and
// 11.15.0 after the key was stripped from Studio Pro- and mxcli-authored
// elements of a fresh app (the strip-and-convert method of ako/mxcli#1505), and
// the same everywhere.
func TestCompletePropertySets_Issue1805Rows(t *testing.T) {
	cases := []struct {
		typ, key string
		check    func(any) bool
	}{
		{"DomainModels$NoGeneralization", "HasOwnerAttr", eq(false)},
		{"DomainModels$NoGeneralization", "HasChangedByAttr", eq(false)},
		{"DomainModels$NoGeneralization", "HasCreatedDateAttr", eq(false)},
		{"DomainModels$NoGeneralization", "HasChangedDateAttr", eq(false)},
		{"DomainModels$DateTimeAttributeType", "LocalizeDate", eq(true)},
		{"ODataPublish$EntitySet", "ReadMode", isElement("ODataPublish$ReadSource")},
		{"ODataPublish$EntitySet", "InsertMode", isElement("ODataPublish$ChangeNotSupported")},
		{"ODataPublish$EntitySet", "UpdateMode", isElement("ODataPublish$ChangeNotSupported")},
		{"ODataPublish$EntitySet", "DeleteMode", isElement("ODataPublish$ChangeNotSupported")},
		{"ODataPublish$PublishedMicroflow", "Parameters", isEmptyList(3)},
	}
	// 11.14.0 is the reporter's version: inside the measured range.
	for _, ver := range []string{"10.24.24", "11.13.0", "11.14.0", "11.15.0"} {
		for _, c := range cases {
			t.Run(ver+"/"+c.typ+"."+c.key, func(t *testing.T) {
				in := mustMarshal(t, bson.D{
					{Key: "$ID", Value: freshID()},
					{Key: "$Type", Value: "Test$Holder"},
					{Key: "El", Value: bson.D{{Key: "$ID", Value: freshID()}, {Key: "$Type", Value: c.typ}}},
				})
				got, ok := keysOf(t, CompletePropertySets(in, v(ver)), "El")[c.key]
				if !ok {
					t.Fatalf("a Mendix %s project kept %s without %s; Studio Pro writes it on every element", ver, c.typ, c.key)
				}
				if !c.check(got) {
					t.Fatalf("%s.%s = %#v on %s, not the value mx convert wrote", c.typ, c.key, got, ver)
				}
			})
		}
	}
}

// The floor is the lowest version measured, and a stated value always wins:
// below it nothing is added, and an element that has the key keeps its value.
func TestCompletePropertySets_Issue1805RowsKeepTheirFloorAndTheStoredValue(t *testing.T) {
	in := mustMarshal(t, bson.D{
		{Key: "$ID", Value: freshID()},
		{Key: "$Type", Value: "Test$Holder"},
		{Key: "El", Value: bson.D{{Key: "$ID", Value: freshID()}, {Key: "$Type", Value: "DomainModels$NoGeneralization"}}},
	})
	if got := keysOf(t, CompletePropertySets(in, v("10.24.23")), "El"); len(got) != 2 {
		t.Errorf("10.24.23, below every measurement, got %v; want nothing added", got)
	}

	owned := mustMarshal(t, bson.D{
		{Key: "$ID", Value: freshID()},
		{Key: "$Type", Value: "Test$Holder"},
		{Key: "El", Value: bson.D{
			{Key: "$ID", Value: freshID()},
			{Key: "$Type", Value: "DomainModels$NoGeneralization"},
			{Key: "HasOwnerAttr", Value: true},
		}},
	})
	if got := keysOf(t, CompletePropertySets(owned, v("11.14.0")), "El")["HasOwnerAttr"]; got != true {
		t.Errorf("a stored HasOwnerAttr true became %#v", got)
	}
}

func isEmptyList(marker int32) func(any) bool {
	return func(got any) bool {
		a, ok := got.(bson.A)
		return ok && len(a) == 1 && a[0] == marker
	}
}
