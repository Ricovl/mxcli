// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func projectSecurityWithRoles(t *testing.T, roles ...bson.D) []byte {
	t.Helper()
	list := bson.A{int32(2)}
	for _, r := range roles {
		list = append(list, r)
	}
	return mustMarshal(t, bson.D{
		{Key: "$ID", Value: freshID()},
		{Key: "$Type", Value: "Security$ProjectSecurity"},
		{Key: "UserRoles", Value: list},
	})
}

func userRole(name string, id bson.Binary, guid *bson.Binary) bson.D {
	d := bson.D{{Key: "$ID", Value: id}, {Key: "$Type", Value: "Security$UserRole"}, {Key: "Name", Value: name}}
	if guid != nil {
		d = append(d, bson.E{Key: "GUID", Value: *guid})
	}
	return d
}

func roleGUIDs(t *testing.T, raw []byte) map[string]*bson.Binary {
	t.Helper()
	out := map[string]*bson.Binary{}
	vals, _ := bson.Raw(raw).Lookup("UserRoles").Array().Values()
	for _, v := range vals {
		doc, ok := v.DocumentOK()
		if !ok {
			continue
		}
		name := doc.Lookup("Name").StringValue()
		if g, err := doc.LookupErr("GUID"); err == nil {
			st, data := g.Binary()
			out[name] = &bson.Binary{Subtype: st, Data: data}
		} else {
			out[name] = nil
		}
	}
	return out
}

// A GUID-less user role gets GUID = its own $ID — what the create path mints
// (codec EmitGUID) — and a role that HAS a GUID keeps it, whatever it is: the
// runtime matches system$userrole rows by that GUID first.
func TestFillMissingUserRoleGUIDs(t *testing.T) {
	bare, kept, other := freshID(), freshID(), freshID()
	in := projectSecurityWithRoles(t, userRole("Bare", bare, nil), userRole("Kept", kept, &other))

	out := FillMissingUserRoleGUIDs(in)
	got := roleGUIDs(t, out)
	if g := got["Bare"]; g == nil || g.Subtype != 0 || !bytes.Equal(g.Data, bare.Data) {
		t.Errorf("GUID-less role: GUID = %v, want its $ID %x as subtype 0", g, bare.Data)
	}
	if g := got["Kept"]; g == nil || !bytes.Equal(g.Data, other.Data) {
		t.Errorf("a role's existing GUID was changed: %v, want %x", g, other.Data)
	}

	// Nothing to fill returns the same bytes, not a re-marshalled copy.
	again := FillMissingUserRoleGUIDs(out)
	if &again[0] != &out[0] {
		t.Error("a unit with nothing to fill was re-marshalled")
	}
}
