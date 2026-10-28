// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#1454: `set (showFooter: true, clearable: false) on cbColor`
// stored PrimitiveValue "yes" / "no". Studio Pro and the widget template store a
// Boolean property as "true" / "false", and the compiled client reads anything
// but "true" as false — so a bare `true` silently did the opposite.
func TestSetPluggableBooleanWritesTrueFalse_1454(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{{true, "true"}, {false, "false"}} {
		w := kindedPluggableWidget("showFooter", "Boolean", "", false)
		if err := setPluggableWidgetPropertyMut(w, "showFooter", tc.value); err != nil {
			t.Fatalf("set %v: %v", tc.value, err)
		}
		if got := bsoncompat.DMap(kindedValue(t, w))["PrimitiveValue"]; got != tc.want {
			t.Errorf("set showFooter = %v: PrimitiveValue = %q, want %q", tc.value, got, tc.want)
		}
	}
}

// withEnumKeys gives the kinded widget's single PropertyType enumeration keys.
func withEnumKeys(w bson.D, keys ...string) bson.D {
	vals := bson.A{int32(2)}
	for _, k := range keys {
		vals = append(vals, bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetEnumerationValue"},
			{Key: "Caption", Value: k},
			{Key: "_Key", Value: k},
		})
	}
	pts := kindedPropertyTypes(w)
	pt := pts[0].(bson.D)
	for i := range pt {
		if pt[i].Key == "ValueType" {
			pt[i].Value = append(pt[i].Value.(bson.D), bson.E{Key: "EnumerationValues", Value: vals})
		}
	}
	return w
}

// An Enumeration whose keys are literally yes/no (1,298 "yes" values in the
// reporter's production scan) keeps taking a bare boolean as those keys.
func TestSetPluggableEnumYesNoTakesBoolean_1454(t *testing.T) {
	w := withEnumKeys(kindedPluggableWidget("mode", "Enumeration", "", false), "yes", "no")
	if err := setPluggableWidgetPropertyMut(w, "mode", true); err != nil {
		t.Fatal(err)
	}
	if got := bsoncompat.DMap(kindedValue(t, w))["PrimitiveValue"]; got != "yes" {
		t.Errorf("PrimitiveValue = %q, want yes", got)
	}
}

// Any other enumeration has no key a boolean can mean, so it is refused rather
// than written as a key the widget does not declare.
func TestSetPluggableEnumRefusesBoolean_1454(t *testing.T) {
	w := withEnumKeys(kindedPluggableWidget("source", "Enumeration", "", false), "context", "database")
	err := setPluggableWidgetPropertyMut(w, "source", true)
	if err == nil {
		t.Fatalf("expected a refusal, wrote %q", bsoncompat.DMap(kindedValue(t, w))["PrimitiveValue"])
	}
	if !strings.Contains(err.Error(), "context") || !strings.Contains(err.Error(), "database") {
		t.Errorf("refusal should name the declared keys: %v", err)
	}
}

func kindedPropertyTypes(w bson.D) []any {
	objType := bsonnav.DGetDoc(bsonnav.DGetDoc(w, "Type"), "ObjectType")
	for _, e := range objType {
		if e.Key == "PropertyTypes" {
			return e.Value.(bson.A)[1:]
		}
	}
	return nil
}
