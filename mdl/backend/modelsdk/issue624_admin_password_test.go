// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// mendixlabs/mxcli#624 (ako/mxcli#1259): CE0129 "Administrator password has
// not been set" could only be resolved in Studio Pro. AdminPassword is written
// on Security$ProjectSecurity and nothing else about the administrator changes.
func TestSetProjectAdminPassword(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ps, err := b.GetProjectSecurity()
	if err != nil {
		t.Fatalf("GetProjectSecurity: %v", err)
	}
	const pw = "N3w-Admin-Passw0rd"
	// Control: the fixture does not already hold the value written below.
	if before := storedString(t, b, ps.ID, "AdminPassword"); before == pw {
		t.Fatalf("fixture AdminPassword is already %q; the control does not hold", before)
	}
	nameBefore := storedString(t, b, ps.ID, "AdminUserName")

	if err := b.SetProjectAdminPassword(ps.ID, pw); err != nil {
		t.Fatalf("SetProjectAdminPassword: %v", err)
	}
	_ = b.Disconnect()

	b = New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	if got := storedString(t, b, ps.ID, "AdminPassword"); got != pw {
		t.Errorf("stored AdminPassword = %q, want %q", got, pw)
	}
	if got := storedString(t, b, ps.ID, "AdminUserName"); got != nameBefore {
		t.Errorf("AdminUserName changed: %q -> %q", nameBefore, got)
	}
	got, err := b.GetProjectSecurity()
	if err != nil || got.AdminPassword != pw {
		t.Errorf("read back AdminPassword %q (%v), want %q", got.AdminPassword, err, pw)
	}
}
