// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `rename entity BIA.VW_StatusTotals to StatusTotals` renamed the entity and its
// references but not its DomainModels$ViewEntitySourceDocument, and the persist
// put the entity's stale SourceDocument reference back over the sweep's
// rewrite: CE6784 "View Entity name is out of sync with the OQL query name".
// Recreating the view under the new name then orphaned the old document
// (CE6786), which no MDL statement can remove.
func TestRename_ViewEntity_RenamesItsSourceDocument(t *testing.T) {
	for _, tc := range []struct {
		name       string
		source     string
		wantRename bool
	}{
		{"view entity", "DomainModels$OqlViewEntitySource", true},
		{"persistent entity (control)", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := mkModule("BIA")
			ent := mkEntity(mod.ID, "VW_StatusTotals")
			ent.Source = tc.source
			if tc.source != "" {
				ent.SourceDocumentRef = "BIA.VW_StatusTotals"
			}
			dm := mkDomainModel(mod.ID, ent)
			var renamed []string
			var persisted string
			mb := &mock.MockBackend{
				IsConnectedFunc:    func() bool { return true },
				ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
				RenameReferencesFunc: func(string, string, bool) ([]types.RenameHit, error) {
					return nil, nil
				},
				UpdateDomainModelFunc: func(d *domainmodel.DomainModel) error {
					persisted = d.Entities[0].SourceDocumentRef
					return nil
				},
				RenameViewEntitySourceDocumentFunc: func(module, oldName, newName string) error {
					renamed = []string{module, oldName, newName}
					return nil
				},
			}
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
			assertNoError(t, execRename(ctx, &ast.RenameStmt{
				ObjectType: "entity",
				Name:       ast.QualifiedName{Module: "BIA", Name: "VW_StatusTotals"},
				NewName:    "StatusTotals",
			}))
			if !tc.wantRename {
				if renamed != nil {
					t.Fatalf("renamed a source document %v for an entity that has none", renamed)
				}
				return
			}
			if len(renamed) != 3 || renamed[0] != "BIA" || renamed[1] != "VW_StatusTotals" || renamed[2] != "StatusTotals" {
				t.Fatalf("source document rename %v, want [BIA VW_StatusTotals StatusTotals]", renamed)
			}
			if persisted != "BIA.StatusTotals" {
				t.Fatalf("persisted SourceDocument %q, want BIA.StatusTotals", persisted)
			}
		})
	}
}
