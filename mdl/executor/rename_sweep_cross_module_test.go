// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#817: RENAME ENTITY read the domain model, ran the project-wide
// RenameReferences sweep over the raw units, and then persisted the semantic model
// it had read BEFORE the sweep. Every edit the sweep made inside the renamed
// entity's own domain model was put back. repointEntitySelfRefs patched the
// entity's own member references, but nothing else in the unit: a same-module
// entity that generalizes the renamed one kept `extends Module.Old` — a dangling
// generalization, after a rename that reported the reference as updated.
//
// Run on the Studio Pro-authored PedApp so the renamed entity has GUID != $ID: a
// re-read-and-persist that re-minted identities would be visible here and
// invisible on an mxcli-created entity (CLAUDE.md, "A GUID Is the Database's
// Identity").
func TestRenameEntity_SameModuleSweepSurvivesThePersist(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := afRun(t, exec, `create persistent entity Administration.Sub817 extends Administration.Account (Note: String(50));`); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	ctx := exec.newExecContext(context.Background())
	mod, err := ctx.Backend.GetModuleByName("Administration")
	if err != nil || mod == nil {
		t.Fatalf("module Administration: %v", err)
	}
	dm := mustDomainModel(t, ctx.Backend, mod.ID)
	if got := entityNamed(t, dm, "Sub817").GeneralizationRef; got != "Administration.Account" {
		t.Fatalf("precondition: Sub817 extends %q, want Administration.Account", got)
	}
	account := entityNamed(t, dm, "Account")
	before := unitGUIDs(t, ctx.Backend, dm.ID)
	if g := before[string(account.ID)]; g == "" || g == string(account.ID) {
		t.Fatalf("precondition: Administration.Account must be Studio Pro-authored (GUID != $ID), got GUID %q", g)
	}

	out.Reset()
	if err := afRun(t, exec, `rename entity Administration.Account to Account817;`); err != nil {
		t.Fatalf("rename: %v\n%s", err, out.String())
	}

	dm = mustDomainModel(t, ctx.Backend, mod.ID)
	// The rename itself landed (a fix that stopped writing would pass the rest).
	if renamed := entityByID(dm, account.ID); renamed == nil || renamed.Name != "Account817" {
		t.Fatalf("entity not renamed: %+v", renamed)
	}
	// The symptom: the sweep rewrote Sub817's generalization, the persist undid it.
	if got := entityNamed(t, dm, "Sub817").GeneralizationRef; got != "Administration.Account817" {
		t.Errorf("Sub817 extends %q after the rename, want Administration.Account817 — "+
			"the domain model persisted after the sweep was read before it (#817)", got)
	}

	assertGUIDsKept(t, before, unitGUIDs(t, ctx.Backend, dm.ID))
}

// ako/mxcli#803, rename half: RENAME ASSOCIATION looked only at dm.Associations,
// so a cross-module association — stored in the FROM module's CrossAssociations —
// was reported "association not found".
//
// TestApp's ViewAssociations.persistent_order is a Studio Pro-authored
// cross-module association (GUID != $ID), so a rename that rebuilt it with a fresh
// GUID would show here.
func TestRenameAssociation_CrossModule(t *testing.T) {
	exec, out := openTestAppCopy(t)
	ctx := exec.newExecContext(context.Background())
	mod, err := ctx.Backend.GetModuleByName("ViewAssociations")
	if err != nil || mod == nil {
		t.Fatalf("module ViewAssociations: %v", err)
	}
	dm := mustDomainModel(t, ctx.Backend, mod.ID)
	var ca *domainmodel.CrossModuleAssociation
	for _, c := range dm.CrossAssociations {
		if c.Name == "persistent_order" {
			ca = c
		}
	}
	if ca == nil {
		t.Fatal("precondition: ViewAssociations.persistent_order is not a cross-module association in the fixture")
	}
	before := unitGUIDs(t, ctx.Backend, dm.ID)
	if g := before[string(ca.ID)]; g == "" || g == string(ca.ID) {
		t.Fatalf("precondition: persistent_order must be Studio Pro-authored (GUID != $ID), got GUID %q", g)
	}

	if err := afRun(t, exec, `rename association ViewAssociations.persistent_order to persistent_order803;`); err != nil {
		t.Fatalf("rename: %v\n%s", err, out.String())
	}

	dm = mustDomainModel(t, ctx.Backend, mod.ID)
	var renamed *domainmodel.CrossModuleAssociation
	for _, c := range dm.CrossAssociations {
		if c.ID == ca.ID {
			renamed = c
		}
	}
	if renamed == nil || renamed.Name != "persistent_order803" {
		t.Fatalf("cross-module association not renamed: %+v", renamed)
	}
	assertGUIDsKept(t, before, unitGUIDs(t, ctx.Backend, dm.ID))
}

// openTestAppCopy opens a private copy of the TestApp submodule fixture.
func openTestAppCopy(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "testapp", "TestApp")
	if _, err := os.Stat(filepath.Join(src, "TestApp.mpr")); err != nil {
		t.Skipf("TestApp submodule not initialised (git submodule update --init testdata/testapp): %v", err)
	}
	dir := t.TempDir()
	if err := copyPedAppFile(filepath.Join(src, "TestApp.mpr"), filepath.Join(dir, "TestApp.mpr")); err != nil {
		t.Fatal(err)
	}
	if err := copyPedAppTree(filepath.Join(src, "mprcontents"), filepath.Join(dir, "mprcontents")); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, "TestApp.mpr")}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	return exec, out
}

func mustDomainModel(t *testing.T, b backend.FullBackend, moduleID model.ID) *domainmodel.DomainModel {
	t.Helper()
	dm, err := b.GetDomainModel(moduleID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	return dm
}

func entityNamed(t *testing.T, dm *domainmodel.DomainModel, name string) *domainmodel.Entity {
	t.Helper()
	for _, e := range dm.Entities {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("entity %s not found", name)
	return nil
}

func entityByID(dm *domainmodel.DomainModel, id model.ID) *domainmodel.Entity {
	for _, e := range dm.Entities {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// unitGUIDs maps every element $ID in a stored unit that carries a GUID to it.
func unitGUIDs(t *testing.T, b backend.FullBackend, unitID model.ID) map[string]string {
	t.Helper()
	raw, err := b.GetRawUnitBytes(unitID)
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			var id, guid string
			for _, e := range x {
				switch e.Key {
				case "$ID":
					id = bsonIDString(e.Value)
				case "GUID":
					guid = bsonIDString(e.Value)
				}
				walk(e.Value)
			}
			if id != "" && guid != "" {
				out[id] = guid
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

// bsonIDString renders a stored $ID/GUID (binary UUID or string) the way the
// semantic model spells an ID, so the two can be compared.
func bsonIDString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bson.Binary:
		return mmpr.BlobToUUID(x.Data)
	}
	return fmt.Sprint(v)
}

func assertGUIDsKept(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) == 0 {
		t.Fatal("no GUID-bearing elements found; the GUID check proves nothing")
	}
	for id, g := range before {
		if a, ok := after[id]; !ok {
			t.Errorf("element %s with GUID %s is gone after the rename", id, g)
		} else if a != g {
			t.Errorf("element %s: storage GUID changed %s -> %s", id, g, a)
		}
	}
}
