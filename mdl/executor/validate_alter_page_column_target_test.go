// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Found reproducing mendixlabs/mxcli#1512: `alter page M.P { replace grid1
// column(C) with { column (…) } }`, where grid1 has no column C, passed
// `check --references` and exec then refused it — "column grid1 column(C) not
// found — a column is addressed by the attribute or the caption describe prints
// in it". Any INSERT or REPLACE on the document waived every column address on
// it, the REPLACE's own target included, because a column the script adds could
// be the one addressed. It can only be one an EARLIER operation adds: exec
// resolves each operation's targets before that operation runs.

// storedColumnsMutator is a stored page holding grid1 with one column, Number.
// Only target resolution and the column-entity lookups are answered.
type storedColumnsMutator struct {
	backend.PageMutator
}

func (storedColumnsMutator) ResolveAlterTarget(t backend.AlterTarget) (backend.AlterTargetMatch, error) {
	switch {
	case len(t.Path) == 1 && t.Path[0] == "grid1":
		return backend.AlterTargetMatch{Kind: "widget", Name: "grid1"}, nil
	case len(t.Path) == 2 && t.Path[0] == "grid1" && t.Path[1] == "column(Number)":
		return backend.AlterTargetMatch{Kind: "column", Name: "grid1 column(Number)"}, nil
	}
	return backend.AlterTargetMatch{}, fmt.Errorf("column %s not found — available: grid1 column(Number)", t)
}
func (storedColumnsMutator) EnclosingEntityForChildren(string) string { return "" }
func (storedColumnsMutator) EnclosingDataSourceFlow(string, bool) (string, string) {
	return "", ""
}

func storedColumnsCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("Shop")
	pg := mkPage(mod.ID, "P_Db")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(model.ID) (backend.PageMutator, error) {
			return storedColumnsMutator{}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func TestAlterPageRefs_ReportsAMissingColumnTheStatementReplaces(t *testing.T) {
	for name, src := range map[string]string{
		"replace":      `alter page Shop.P_Db { replace grid1 column(C) with { column (Attribute: Note, Caption: 'C') } };`,
		"insert after": `alter page Shop.P_Db { insert after grid1 column(C) { column (Attribute: Note, Caption: 'N') } };`,
		"drop before an insert": `alter page Shop.P_Db { drop widget grid1 column(Nope)
			insert after grid1 column(Number) { column (Attribute: Note, Caption: 'N') } };`,
		"caption address": `alter page Shop.P_Db { replace grid1 column('Nope') with { column (Attribute: Note, Caption: 'C') } };`,
		"after an unrelated insert": `alter page Shop.P_Db { replace grid1 column(C) with { column (Attribute: Note, Caption: 'C') } };
			alter page Shop.P_Db { insert after grid1 column(Note) { column (Attribute: Total, Caption: 'T') } };`,
	} {
		t.Run(name, func(t *testing.T) {
			errs := checkAlterPageRefs(t, storedColumnsCtx(t), src)
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), "not found") {
				t.Fatalf("an address no stored column has passed check: %v", errs)
			}
		})
	}
}

// CONTROL: a column an earlier operation adds is not in the stored document,
// and must not be reported — in the same statement or a later one.
func TestAlterPageRefs_AcceptsAColumnAnEarlierOperationAdds(t *testing.T) {
	for name, src := range map[string]string{
		"stored column": `alter page Shop.P_Db { replace grid1 column(Number) with { column (Attribute: Note, Caption: 'C') } };`,
		"same statement": `alter page Shop.P_Db {
			insert after grid1 column(Number) { column (Attribute: Note, Caption: 'N') }
			replace grid1 column(Note) with { column (Attribute: Total, Caption: 'T') } };`,
		"later statement": `alter page Shop.P_Db { replace grid1 column(Number) with { column (Attribute: Note, Caption: 'C') } };
			alter page Shop.P_Db { replace grid1 column(Note) with { column (Attribute: Total, Caption: 'T') } };`,
	} {
		t.Run(name, func(t *testing.T) {
			if errs := checkAlterPageRefs(t, storedColumnsCtx(t), src); len(errs) != 0 {
				t.Errorf("got %v, want none", errs)
			}
		})
	}
}
