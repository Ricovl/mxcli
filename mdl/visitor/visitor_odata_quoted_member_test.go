// SPDX-License-Identifier: Apache-2.0

package visitor

import "testing"

// mendixlabs/mxcli#1423: `expose (..., "Description")` stored the double quotes
// as part of the member name, so mx check reported
// CE1613 "The selected attribute 'ModA.EntA."Description"' no longer exists".
// The quotes are identifier-quoting syntax, as in `create persistent entity`.
func TestPublishEntity_QuotedExposeMemberIsUnquoted(t *testing.T) {
	svc := odataActionService(t,
		"  publish entity T.EntA as 'EntAs' (ReadMode: source)\n"+
			"    expose (Name as 'Name' (KEY), \"Description\");")
	if len(svc.Entities) != 1 || len(svc.Entities[0].Members) != 2 {
		t.Fatalf("expected 1 entity with 2 members, got %+v", svc.Entities)
	}
	if got := svc.Entities[0].Members[1].Name; got != "Description" {
		t.Errorf("member name = %q, want %q", got, "Description")
	}
}

// Same grammar rule, published microflow parameters.
func TestPublishMicroflow_QuotedExposeParamIsUnquoted(t *testing.T) {
	svc := odataActionService(t,
		"  publish microflow T.DoThing expose ( \"Description\" as 'description' );")
	if len(svc.Microflows) != 1 || len(svc.Microflows[0].Parameters) != 1 {
		t.Fatalf("expected 1 microflow with 1 parameter, got %+v", svc.Microflows)
	}
	if got := svc.Microflows[0].Parameters[0].Name; got != "Description" {
		t.Errorf("parameter name = %q, want %q", got, "Description")
	}
}
