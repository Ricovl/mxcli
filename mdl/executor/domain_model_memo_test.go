// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"testing"
)

// TestGetDomainModel_SeesAWrite: the per-module decode memo is dropped when the
// domain model is written, in the same process.
func TestGetDomainModel_SeesAWrite(t *testing.T) {
	exec := typeCheckFixture(t)
	ctx := exec.newExecContext(context.Background())
	mod, err := ctx.Backend.GetModuleByName("Administration")
	if err != nil || mod == nil {
		t.Fatal("no Administration module")
	}
	has := func() bool {
		dm, err := ctx.Backend.GetDomainModel(mod.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range dm.Entities {
			for _, a := range e.Attributes {
				if e.Name == "Account" && a.Name == "MemoProbe" {
					return true
				}
			}
		}
		return false
	}
	if has() {
		t.Fatal("control: the attribute exists before the write")
	}
	run(t, exec, `alter entity Administration.Account add attribute MemoProbe: String;`)
	if !has() {
		t.Error("GetDomainModel still returns the domain model from before the write")
	}
}
