// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// RenameDocument renames the unit it is given — and only a document, and only
// one still carrying the name the caller resolved. Its predecessor took a bare
// name and rewrote the first unit in the module carrying it, which for a page in
// a folder of its own name was the folder (ako/mxcli#1443). These are the two
// refusals that keep a wrong ID from doing the same damage, and the control that
// the right one still renames.
func TestRenameDocument_RefusesContainersAndAStaleName(t *testing.T) {
	b := New()
	if err := b.Connect(copyPedAppFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	folders, err := b.ListFolders()
	if err != nil {
		t.Fatal(err)
	}
	var folderID model.ID
	var folderName string
	for _, f := range folders {
		if f.Name != "" {
			folderID, folderName = f.ID, f.Name
			break
		}
	}
	if folderID == "" {
		t.Fatal("precondition: PedApp has no named folder")
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("module MyFirstModule: %v", err)
	}
	pgs, err := b.ListPages()
	if err != nil || len(pgs) == 0 {
		t.Fatalf("precondition: PedApp has no page (%v)", err)
	}
	pg := pgs[0]

	pageName := func() string {
		t.Helper()
		all, err := b.ListPages()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range all {
			if p.ID == pg.ID {
				return p.Name
			}
		}
		t.Fatalf("page %s is gone", pg.ID)
		return ""
	}

	// A folder is a container, not a document.
	err = b.RenameDocument(folderID, folderName, "Renamed1443")
	if err == nil || !strings.Contains(err.Error(), "not a document") {
		t.Errorf("RenameDocument on folder %q: err %v, want a refusal", folderName, err)
	}
	after, err := b.ListFolders()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range after {
		if f.ID == folderID && f.Name != folderName {
			t.Errorf("folder %q was renamed to %q", folderName, f.Name)
		}
	}

	// So is a module; RENAME MODULE has its own path.
	if err := b.RenameDocument(mod.ID, mod.Name, "Renamed1443"); err == nil || !strings.Contains(err.Error(), "not a document") {
		t.Errorf("RenameDocument on module %s: err %v, want a refusal", mod.Name, err)
	}

	// The stored name must be the one the caller resolved.
	if err := b.RenameDocument(pg.ID, pg.Name+"_Stale", "Renamed1443"); err == nil || !strings.Contains(err.Error(), "is named") {
		t.Errorf("RenameDocument with a stale name: err %v, want a refusal", err)
	}
	if got := pageName(); got != pg.Name {
		t.Errorf("page renamed to %q by a refused call", got)
	}

	// Control: the right unit and name are renamed.
	if err := b.RenameDocument(pg.ID, pg.Name, pg.Name+"_1443"); err != nil {
		t.Fatalf("RenameDocument: %v", err)
	}
	if got := pageName(); got != pg.Name+"_1443" {
		t.Errorf("page is named %q, want %q", got, pg.Name+"_1443")
	}
}
