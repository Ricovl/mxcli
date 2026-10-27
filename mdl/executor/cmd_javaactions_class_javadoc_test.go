// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// ako/mxcli#1700 (mendixlabs/mxcli#1653): mxbuild writes a Java action's
// documentation as the class javadoc, and the .java file mxcli wrote had none —
// a new action got no block, and `create or modify` removed the one a build had
// written, so every rewrite was a pair of unrelated diffs. The generator now
// writes it (sdk/javaactions, measured against mxbuild); this is the executor
// handing it the documentation the model ends up with.

// writtenJavaDoc runs s against a backend holding stored (nil: none) and
// returns the documentation WriteJavaSourceFile was given.
func writtenJavaDoc(t *testing.T, s *ast.CreateJavaActionStmt, stored *javaactions.JavaAction) string {
	t.Helper()
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)
	var light []*types.JavaAction
	if stored != nil {
		stored.ContainerID = mod.ID
		withContainer(h, stored.ContainerID, mod.ID)
		light = []*types.JavaAction{{
			BaseElement:   model.BaseElement{ID: stored.ID},
			ContainerID:   stored.ContainerID,
			Name:          stored.Name,
			Documentation: stored.Documentation,
		}}
	}
	written, called := "", false
	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListModulesFunc:          func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaActionsFunc:      func() ([]*types.JavaAction, error) { return light, nil },
		ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) { return stored, nil },
		CreateJavaActionFunc:     func(*javaactions.JavaAction) error { return nil },
		UpdateJavaActionFunc:     func(*javaactions.JavaAction) error { return nil },
		WriteJavaSourceFileFunc: func(_, _, _ string, _ []*javaactions.JavaActionParameter,
			_ javaactions.CodeActionReturnType, _ []string, _ *string, documentation string) error {
			written, called = documentation, true
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateJavaAction(ctx, s))
	if !called {
		t.Fatal("WriteJavaSourceFile was never called")
	}
	return written
}

func TestCreateJavaAction_WritesTheDocumentationIntoTheSource(t *testing.T) {
	s := parseOne[*ast.CreateJavaActionStmt](t, `/**
 * Formats an amount as text with two decimals.
 *
 * Second paragraph.
 */
create java action MyModule.FormatAmount(Amount: Decimal not null) returns String
as $$
return String.format("%.2f", Amount);
$$;`)
	if got, want := writtenJavaDoc(t, s, nil), "Formats an amount as text with two decimals.\n\nSecond paragraph."; got != want {
		t.Errorf("documentation written = %q, want %q", got, want)
	}
}

// A rewrite without a doc comment keeps the stored documentation (#1018), so
// the file keeps the block too — the report's `create or modify` removed it.
func TestCreateOrModifyJavaAction_CarriesTheDocumentationIntoTheSource(t *testing.T) {
	stored := &javaactions.JavaAction{Name: "JA_Ping", Documentation: "Stored in Studio Pro."}
	stored.ID = nextID("ja")
	if got := writtenJavaDoc(t, rewriteStmt(false), stored); got != "Stored in Studio Pro." {
		t.Errorf("documentation written = %q, want the stored one", got)
	}

	// Control: a stated doc comment replaces it, in the model and the file.
	s := rewriteStmt(false)
	s.Documentation, s.DocumentationSet = "Stated.", true
	stored = &javaactions.JavaAction{Name: "JA_Ping", Documentation: "Stored in Studio Pro."}
	stored.ID = nextID("ja")
	if got := writtenJavaDoc(t, s, stored); got != "Stated." {
		t.Errorf("documentation written = %q, want the stated one", got)
	}
}
