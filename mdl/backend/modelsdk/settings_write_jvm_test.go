// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"
)

// mendixlabs/mxcli#131: the value `alter settings configuration 'Default'
// (ExtraJvmParameters: '…')` sets has to reach the stored configuration under the
// key Studio Pro reads — "ExtraJvmParameters", as a Studio Pro-authored 11.14
// project stores it — and read back through the backend.
func TestUpdateProjectSettings_ExtraJvmParametersPersist(t *testing.T) {
	const params = "-Dhttps.proxyHost=127.0.0.1 -Dhttps.proxyPort=29758"
	proj := copyFixture(t)
	seeded := seedConfiguration(t, proj)
	if _, ok := seeded["ExtraJvmParameters"]; !ok {
		t.Fatalf("fixture precondition: the Default configuration does not store ExtraJvmParameters: %#v", seeded)
	}

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	ps.Configuration.Configurations[0].ExtraJvmParameters = params
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	after := readConfiguration(t, proj)
	if got := after["ExtraJvmParameters"]; got != params {
		t.Errorf("stored ExtraJvmParameters = %#v, want %q", got, params)
	}
	// The write is an overlay: the seeded data beside it survives.
	if after["OpenAdminPort"] != seeded["OpenAdminPort"] {
		t.Errorf("OpenAdminPort = %#v, want %#v", after["OpenAdminPort"], seeded["OpenAdminPort"])
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	ps2, err := b2.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings(2): %v", err)
	}
	if got := ps2.Configuration.Configurations[0].ExtraJvmParameters; got != params {
		t.Errorf("ExtraJvmParameters read back as %q, want %q", got, params)
	}
}
