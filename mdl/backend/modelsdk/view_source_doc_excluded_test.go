// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// ako/mxcli#827 (side item, from #821): the update path of a view entity's OQL
// document encoded Excluded=false as a constant, so any CREATE OR MODIFY of the
// view entity re-included a document Studio Pro had excluded. Exclusion is
// model state, not script state (#914): the rewrite keeps the stored value.
func TestWriteViewEntitySourceDocument_KeepsStoredExcluded(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	const oql = "from MyFirstModule.Thing as t select t.Name as Name"
	id, err := b.WriteViewEntitySourceDocument(mod.ID, "MyFirstModule", "ZzExcluded", oql, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	excluded := func() bool {
		t.Helper()
		raw, err := b.reader.GetRawUnitBytes(string(id))
		if err != nil {
			t.Fatal(err)
		}
		v, ok := bsoncore.Document(raw).Lookup("Excluded").BooleanOK()
		if !ok {
			t.Fatal("stored document has no boolean Excluded")
		}
		return v
	}
	if excluded() {
		t.Fatal("control: a new document is stored excluded")
	}

	// Exclude it, as Studio Pro would.
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatal(err)
	}
	patched, err := withTopLevelBoolean(append([]byte(nil), raw...), "Excluded", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.writer.UpdateRawUnit(string(id), patched); err != nil {
		t.Fatal(err)
	}
	if !excluded() {
		t.Fatal("setup: the document is not excluded")
	}

	// A rewrite with a changed query lands, and keeps the exclusion.
	if _, err := b.WriteViewEntitySourceDocument(mod.ID, "MyFirstModule", "ZzExcluded", oql+" where t.Name != ''", ""); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !excluded() {
		t.Error("rewriting the view entity's OQL document re-included it (Excluded reset to false)")
	}
}
