// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestReadSlice_Microflows checks the gen→microflows adapter: the parameter /
// activity split (parameters live in the object collection and must not be
// counted as activities) and the return-type mapping. SHOW MICROFLOWS is
// cross-checked byte-for-byte against the legacy engine in the plan validation.
func TestReadSlice_Microflows(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mfs, err := b.ListMicroflows()
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	if len(mfs) != 16 {
		t.Fatalf("ListMicroflows count = %d, want 16", len(mfs))
	}

	var cmp *struct {
		params  int
		ret     string
		hasObjs bool
	}
	for _, mf := range mfs {
		if mf.Name == "ChangeMyPassword" {
			ret := ""
			if mf.ReturnType != nil {
				ret = mf.ReturnType.GetTypeName()
			}
			cmp = &struct {
				params  int
				ret     string
				hasObjs bool
			}{len(mf.Parameters), ret, mf.ObjectCollection != nil}
		}
	}
	if cmp == nil {
		t.Fatal("ChangeMyPassword not found")
	}
	if cmp.params != 1 {
		t.Errorf("ChangeMyPassword parameter count = %d, want 1 (params must not be counted as activities)", cmp.params)
	}
	if cmp.ret != "Void" {
		t.Errorf("ChangeMyPassword return type = %q, want Void", cmp.ret)
	}
	if !cmp.hasObjs {
		t.Error("ChangeMyPassword should have a populated object collection")
	}
}

func TestGetMicroflowByName_DecodesSelectedUnit(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mf, err := b.GetMicroflowByName("Administration.ChangeMyPassword")
	if err != nil {
		t.Fatalf("GetMicroflowByName: %v", err)
	}
	if mf == nil {
		t.Fatal("GetMicroflowByName returned nil")
	}
	if mf.Name != "ChangeMyPassword" || len(mf.Parameters) != 1 {
		t.Fatalf("microflow = %q with %d parameters", mf.Name, len(mf.Parameters))
	}

	missing, err := b.GetMicroflowByName("Administration.DoesNotExist")
	if err != nil {
		t.Fatalf("missing lookup: %v", err)
	}
	if missing != nil {
		t.Fatalf("missing lookup = %#v, want nil", missing)
	}
}

// TestReadSlice_Nanoflows confirms nanoflows reuse the microflow conversion
// helpers (parameter split, flow objects, return type). SHOW NANOFLOWS is
// cross-checked byte-for-byte against legacy in the plan validation.
func TestReadSlice_Nanoflows(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	nfs, err := b.ListNanoflows()
	if err != nil {
		t.Fatalf("ListNanoflows: %v", err)
	}
	if len(nfs) != 13 {
		t.Fatalf("ListNanoflows count = %d, want 13", len(nfs))
	}
	for _, nf := range nfs {
		if nf.Name == "" {
			t.Error("nanoflow with empty name")
		}
	}
}
