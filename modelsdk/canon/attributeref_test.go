// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Moved from mdl/backend/modelsdk (page_bare_attributeref_test.go) when the
// guard moved to the write choke point. The shape is Feedback v4.0.2's
// ShareFeedback_Logo: an image URL parameter written as a bare `ImageB64`
// under a data view whose flow the project lacks, which left `mx check` unable
// to LOAD the project (ArgumentNullException setting 'Attribute', 11.13.0).
func TestBareAttributeRefError(t *testing.T) {
	attrRef := func(a string) bson.D {
		return bson.D{{Key: "$Type", Value: "DomainModels$AttributeRef"}, {Key: "Attribute", Value: a}, {Key: "EntityRef", Value: nil}}
	}
	doc := func(a string) []byte {
		b, err := bson.Marshal(bson.D{
			{Key: "$Type", Value: "Forms$Page"},
			{Key: "Widgets", Value: bson.A{int32(2), bson.D{
				{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
				{Key: "Name", Value: "image1"},
				{Key: "Params", Value: bson.A{int32(2), bson.D{{Key: "AttributeRef", Value: attrRef(a)}}}},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, bare := range []string{"ImageB64", "Feedback.ImageB64"} {
		err := BareAttributeRefError("page X", doc(bare))
		if err == nil || !strings.Contains(err.Error(), `"`+bare+`"`) || !strings.Contains(err.Error(), "image1") {
			t.Fatalf("a bare attribute reference must be refused, naming it and its widget; got %v", err)
		}
	}
	for _, ok := range []string{"FeedbackModule.Feedback.ImageB64", ""} {
		if err := BareAttributeRefError("page X", doc(ok)); err != nil {
			t.Errorf("%q must be accepted: %v", ok, err)
		}
	}
	if got := BareAttributeRefs([]byte{1, 2, 3}); got != nil {
		t.Errorf("unreadable bytes must yield nothing, got %v", got)
	}
}

// ako/mxcli#885: a change activity's member is the same kind of identifier. A
// spliced change wrote `Name` for `System.User.Name`, and `mx check` (11.13.0)
// could not load the project ("The text 'Name' is not a valid
// AttributeIdentifier").
func TestBareAttributeRefError_ChangeActionItem(t *testing.T) {
	doc := func(attr, assoc string) []byte {
		b, err := bson.Marshal(bson.D{
			{Key: "$Type", Value: "Microflows$Microflow"},
			{Key: "Name", Value: "Repro_ChangeState"},
			{Key: "ObjectCollection", Value: bson.D{{Key: "Objects", Value: bson.A{int32(2), bson.D{
				{Key: "$Type", Value: "Microflows$ActionActivity"},
				{Key: "Action", Value: bson.D{
					{Key: "$Type", Value: "Microflows$ChangeAction"},
					{Key: "Items", Value: bson.A{int32(2), bson.D{
						{Key: "$Type", Value: "Microflows$ChangeActionItem"},
						{Key: "Attribute", Value: attr},
						{Key: "Association", Value: assoc},
					}}},
				}},
			}}}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	err := BareAttributeRefError("microflow X", doc("Name", ""))
	if err == nil || !strings.Contains(err.Error(), `"Name"`) || !strings.Contains(err.Error(), "Repro_ChangeState") {
		t.Fatalf("a bare change member must be refused, naming it and its flow; got %v", err)
	}
	// Control: a qualified attribute, and an association member (whose
	// Attribute is empty), are written.
	for _, ok := range [][2]string{{"System.User.Name", ""}, {"", "System.UserRoles"}} {
		if err := BareAttributeRefError("microflow X", doc(ok[0], ok[1])); err != nil {
			t.Errorf("%v must be accepted: %v", ok, err)
		}
	}
}
