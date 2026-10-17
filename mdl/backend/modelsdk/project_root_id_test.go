// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"
)

// mendixlabs/mxcli#1526: the project id (the Projects$Project root unit's $ID,
// which Business Events uses as the app id) was readable by no command, because
// the backend's GetProjectRootID was the generated "unimplemented" stub.
func TestGetProjectRootID_IsTheModulesContainer(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	root, err := b.GetProjectRootID()
	if err != nil {
		t.Fatalf("GetProjectRootID: %v", err)
	}
	if len(root) != 36 {
		t.Fatalf("project id = %q, want a 36-char UUID", root)
	}
	units, err := b.ListUnits()
	if err != nil {
		t.Fatalf("ListUnits: %v", err)
	}
	modules := 0
	for _, u := range units {
		if u.Type != "Projects$ModuleImpl" {
			continue
		}
		modules++
		if string(u.ContainerID) != root {
			t.Errorf("module unit %s has ContainerID %s, want the project id %s", u.ID, u.ContainerID, root)
		}
	}
	if modules == 0 {
		t.Fatal("fixture listed no module units; the control has nothing to compare")
	}
}

func TestGetProjectRootID_NotConnected(t *testing.T) {
	if _, err := New().GetProjectRootID(); err == nil {
		t.Error("want an error when not connected")
	}
}
