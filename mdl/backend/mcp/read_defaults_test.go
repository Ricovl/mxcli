// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Studio Pro 11.15's ped_read_document leaves out every property whose value
// equals the element's schema default ("If the property's value is equal to the
// element's schema default, it will be excluded"). Up to 11.14 they were
// present. A reader that maps an absent property to Go's zero value therefore
// reads a DIFFERENT value whenever the schema default is not the zero value.
// Measured live on 11.15.0-rc.4 against the same Administration module:
//
//	/entities/0/attributes/0/type   11.14 {"$Type":"…StringAttributeType","length":200}
//	                                11.15 {"$Type":"…StringAttributeType"}
//	/associations/0                 11.14 …"type":"Reference","owner":"Default"…
//	                                11.15 (both absent)

func TestAttributeTypeFromPED_AbsentLengthIsTheDefault200(t *testing.T) {
	got, ok := attributeTypeFromPED(json.RawMessage(`{"$Type":"DomainModels$StringAttributeType"}`)).(*domainmodel.StringAttributeType)
	if !ok || got.Length != 200 {
		t.Fatalf("absent length read as %+v; 11.15 omits the schema default 200, and 0 means unlimited", got)
	}
	unl, _ := attributeTypeFromPED(json.RawMessage(`{"$Type":"DomainModels$StringAttributeType","length":0}`)).(*domainmodel.StringAttributeType)
	if unl == nil || unl.Length != 0 {
		t.Errorf("an explicit length 0 (unlimited) must stay 0, got %+v", unl)
	}
}

func TestReconstructAssociations_AbsentTypeAndOwnerAreTheDefaults(t *testing.T) {
	b := &Backend{}
	raw := json.RawMessage(`[{"$Type":"DomainModels$Association","name":"A_B","parent":"$id(/entities/0)","child":"$id(/entities/1)"}]`)
	assocs, err := b.reconstructAssociations("M", raw, []string{"e0", "e1"})
	if err != nil || len(assocs) != 1 {
		t.Fatalf("reconstructAssociations: %v %v", assocs, err)
	}
	if assocs[0].Type != domainmodel.AssociationTypeReference || assocs[0].Owner != domainmodel.AssociationOwnerDefault {
		t.Errorf("type/owner = %q/%q, want the schema defaults Reference/Default", assocs[0].Type, assocs[0].Owner)
	}
}
