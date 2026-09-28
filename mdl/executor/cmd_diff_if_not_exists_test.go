// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// diffEnumScript diffs src against a project holding enumeration M.E (Red).
func diffEnumScript(t *testing.T, src string) string {
	t.Helper()
	mod := mkModule("M")
	enum := mkEnumeration(mod.ID, "E", "Red")
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}
	ctx, out := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if err := diffProgram(ctx, prog, DiffOptions{}); err != nil {
		t.Fatalf("diff: %v", err)
	}
	return out.String()
}

// diff is the pre-apply view of exec, so a guarded create of an element that
// already exists — which exec skips and leaves untouched (ako/mxcli#731) — is
// unchanged, not modified.
func TestDiff_CreateIfNotExistsOnExistingElementIsUnchanged(t *testing.T) {
	out := diffEnumScript(t, "create enumeration if not exists M.E (Blue 'Blue');")
	if !strings.Contains(out, "Summary: 0 new, 0 modified, 1 unchanged") {
		t.Errorf("a guarded create of an existing element must diff as unchanged; got:\n%s", out)
	}
	if strings.Contains(out, "Blue") {
		t.Errorf("diff shows the script's definition, which exec will not apply:\n%s", out)
	}
}

// CONTROL: without the guard the same statement is a modification.
func TestDiff_UnguardedCreateOnExistingElementIsModified(t *testing.T) {
	out := diffEnumScript(t, "create or modify enumeration M.E (Blue 'Blue');")
	if !strings.Contains(out, "Summary: 0 new, 1 modified, 0 unchanged") {
		t.Errorf("control: an unguarded create of a changed element must diff as modified; got:\n%s", out)
	}
}

// CONTROL: a guarded create of an absent element is new.
func TestDiff_CreateIfNotExistsOnAbsentElementIsNew(t *testing.T) {
	out := diffEnumScript(t, "create enumeration if not exists M.F (Blue 'Blue');")
	if !strings.Contains(out, "Summary: 1 new, 0 modified, 0 unchanged") {
		t.Errorf("a guarded create of an absent element must diff as new; got:\n%s", out)
	}
}
