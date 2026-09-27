// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// Studio Pro writes Security$ProjectSecurity's UserRoles and DemoUsers lists
// with typed-array marker 2 (measured: PedApp 11.13.0, TestApp 11.14.0 and the
// expr-checker fixture all agree). The encoder's default is 3, so rewriting the
// document for `create or modify user role` with an unchanged definition, which
// is what running `describe user role` output does, changed the marker and
// wrote the unit (#731). The control re-reads the untouched document.
func TestProjectSecurity_UserRoleAndDemoUserListMarkers(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	ps, err := b.GetProjectSecurity()
	if err != nil {
		t.Fatalf("GetProjectSecurity: %v", err)
	}
	if len(ps.UserRoles) == 0 {
		t.Fatal("fixture has no user roles")
	}
	markers := func() (int32, int32) {
		raw, err := b.GetRawUnitBytes(ps.ID)
		if err != nil {
			t.Fatalf("GetRawUnitBytes: %v", err)
		}
		var d bson.D
		if err := bson.Unmarshal(raw, &d); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return markerOf(t, d, "UserRoles"), markerOf(t, d, "DemoUsers")
	}

	// Control: the Studio Pro-authored document carries marker 2 for both.
	if u, d := markers(); u != 2 || d != 2 {
		t.Fatalf("fixture markers UserRoles=%d DemoUsers=%d; want 2 and 2 — the control does not hold", u, d)
	}

	ur := ps.UserRoles[0]
	if err := b.AddUserRole(ps.ID, ur.Name, append([]string{}, ur.ModuleRoles...), !ur.ManageAllRoles); err != nil {
		t.Fatalf("AddUserRole: %v", err)
	}
	if u, d := markers(); u != 2 || d != 2 {
		t.Errorf("after rewriting user role %s: markers UserRoles=%d DemoUsers=%d; want 2 and 2", ur.Name, u, d)
	}

	if len(ps.DemoUsers) == 0 {
		t.Fatal("fixture has no demo users")
	}
	du := ps.DemoUsers[0]
	if err := b.AddDemoUser(ps.ID, du.UserName, du.Password+"x", du.Entity, append([]string{}, du.UserRoles...)); err != nil {
		t.Fatalf("AddDemoUser: %v", err)
	}
	if u, d := markers(); u != 2 || d != 2 {
		t.Errorf("after rewriting demo user %s: markers UserRoles=%d DemoUsers=%d; want 2 and 2", du.UserName, u, d)
	}
}
