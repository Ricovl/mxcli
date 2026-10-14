// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A Security$UserRole without a GUID gets one invented by mxbuild on every
// build — random, so the runtime's system$userrole.modelguid never matches
// the model and the row is found by name only. Filling GUID = $ID (what the
// create path mints) gives the role a stable identity; an existing GUID is the
// runtime's primary match key and must never change.

const (
	roleBareID   = "aaaaaaa1-0000-4000-8000-000000000001"
	roleKeptID   = "aaaaaaa1-0000-4000-8000-000000000002"
	roleKeptGUID = "aaaaaaa1-0000-4000-8000-000000000003"
)

// projectSecurityRoles builds a ProjectSecurity unit with one GUID-less user
// role and one whose GUID is not its $ID (Studio Pro-style).
// With bareHasGUID the first role is given its GUID too — the baseline the
// repair count is compared with, since the table completes other keys as well.
func projectSecurityRoles(t *testing.T, bareHasGUID bool) []byte {
	t.Helper()
	bin := func(id string) bson.Binary { return bson.Binary{Subtype: 0, Data: uuidToBlob(id)} }
	bare := bson.D{{Key: "$ID", Value: bin(roleBareID)}, {Key: "$Type", Value: "Security$UserRole"}, {Key: "Name", Value: "User"}}
	if bareHasGUID {
		bare = append(bare, bson.E{Key: "GUID", Value: bin(roleBareID)})
	}
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bin(repairUnitID)},
		{Key: "$Type", Value: "Security$ProjectSecurity"},
		{Key: "UserRoles", Value: bson.A{int32(2),
			bare,
			bson.D{{Key: "$ID", Value: bin(roleKeptID)}, {Key: "$Type", Value: "Security$UserRole"}, {Key: "Name", Value: "Administrator"},
				{Key: "GUID", Value: bin(roleKeptGUID)}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// roleGUID returns the GUID bytes of the i-th entry of UserRoles (index 0 is
// the list marker), or nil when it has none.
func roleGUID(t *testing.T, raw []byte, i int) []byte {
	t.Helper()
	g, err := bson.Raw(raw).Lookup("UserRoles").Array().Index(uint(i)).Document().LookupErr("GUID")
	if err != nil {
		return nil
	}
	_, data := g.Binary()
	return data
}

// The repair fills a missing UserRole GUID with the role's $ID, counts it,
// never touches an existing one, and has nothing to do on a second run.
func TestCompleteStoredPropertySets_FillsMissingUserRoleGUID(t *testing.T) {
	baseW, _ := newV2WriterForCommitTest(t, repairUnitID, projectSecurityRoles(t, true))
	baseW.reader.projectVersion = pv(11, 15, 0)
	base, err := baseW.reader.StoredPropertySetGaps(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}

	stored := projectSecurityRoles(t, false)
	w, path := newV2WriterForCommitTest(t, repairUnitID, stored)
	w.reader.projectVersion = pv(11, 15, 0)

	rep, err := w.CompleteStoredPropertySets(repairUnitID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != base.Added+1 {
		t.Fatalf("repair = %+v, want the GUID counted on top of the %d other keys", rep, base.Added)
	}
	after, _ := os.ReadFile(path)
	if got := roleGUID(t, after, 1); !bytes.Equal(got, uuidToBlob(roleBareID)) {
		t.Errorf("GUID-less role: GUID = %x, want its $ID", got)
	}
	if got := roleGUID(t, after, 2); !bytes.Equal(got, uuidToBlob(roleKeptGUID)) {
		t.Errorf("existing role GUID changed to %x", got)
	}

	rep, err = w.CompleteStoredPropertySets(repairUnitID)
	if err != nil || rep.Changed() {
		t.Fatalf("second run = %+v, %v; want nothing", rep, err)
	}
	if again, _ := os.ReadFile(path); !bytes.Equal(again, after) {
		t.Fatal("the second run rewrote the unit")
	}
}

// An ordinary rewrite of the security unit — which carries a stored role's
// bytes through as they are — fills the GUID too, and the storage-GUID guard
// does not refuse it (a GUID on one side only is not a change of identity).
func TestUpdateUnitFillsMissingUserRoleGUID(t *testing.T) {
	stored := projectSecurityRoles(t, false)
	w, path := newV2WriterForCommitTest(t, repairUnitID, stored)
	w.reader.projectVersion = pv(11, 15, 0)

	if err := w.UpdateRawUnit(repairUnitID, stored); err != nil {
		t.Fatalf("rewrite refused: %v", err)
	}
	after, _ := os.ReadFile(path)
	if got := roleGUID(t, after, 1); !bytes.Equal(got, uuidToBlob(roleBareID)) {
		t.Errorf("GUID-less role after rewrite: GUID = %x, want its $ID", got)
	}
	if got := roleGUID(t, after, 2); !bytes.Equal(got, uuidToBlob(roleKeptGUID)) {
		t.Errorf("existing role GUID changed to %x", got)
	}
}
