// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// An association's StorageFormat decides the runtime schema: Table is a junction
// table, Column a foreign-key column on the FROM entity's table. Flipping it on a
// deployed app migrates the database.
//
// DESCRIBE used to print `storage column` and nothing for Table (its comment
// called Table "the default"), while CREATE defaults an unstated storage to
// Column. So a description of a Studio Pro table-stored association — measured on
// PedApp's Administration.AccountPasswordData_Account — replayed elsewhere, or as
// plain `create`, produced a column association.
//
// #704 made OR MODIFY carry an unstated storage, which closes the same-project
// replay by accident of the carry; describe now omits only the create default
// (R12, #748). The assertion is therefore on the MDL: the replayed statement,
// read with CREATE's defaults, must mean the stored storage — not merely end up
// there because OR MODIFY carried it.
func TestDescribeAssociationStorageSurvivesReplay(t *testing.T) {
	for _, stored := range []domainmodel.AssociationStorageFormat{
		domainmodel.StorageFormatTable, domainmodel.StorageFormatColumn,
	} {
		t.Run(string(stored), func(t *testing.T) {
			ctx, assoc := assocFixture(t)
			assoc.StorageFormat = stored

			var buf bytes.Buffer
			ctx.Output = &buf
			assertNoError(t, describeAssociation(ctx, ast.QualifiedName{Module: "M", Name: "Child_Parent"}))

			prog, errs := visitor.Build(buf.String())
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE emitted MDL the parser rejects: %v\n%s", errs, buf.String())
			}
			stmt := prog.Statements[0].(*ast.CreateAssociationStmt)
			// What plain `create` of this text would store: a stated storage, or
			// CREATE's default (Column).
			asCreated, stated := statedStorageFormat(stmt.Storage)
			if !stated {
				asCreated = domainmodel.StorageFormatColumn
			}
			if asCreated != stored {
				t.Errorf("DESCRIBE of storage %s reads back as %s under CREATE's defaults\n%s", stored, asCreated, buf.String())
			}
			stmt.CreateOrModify = true
			assertNoError(t, execCreateAssociation(ctx, stmt))

			if assoc.StorageFormat != stored {
				t.Errorf("replaying DESCRIBE changed storage %s → %s\n--- describe ---\n%s",
					stored, assoc.StorageFormat, buf.String())
			}
		})
	}
}

// `mxcli diff` must show a storage change, and must not show one for a statement
// that leaves storage unstated (exec carries the stored value in that case).
func TestDiffAssociationComparesStorage(t *testing.T) {
	base := func(s ast.StorageType) *ast.CreateAssociationStmt {
		return &ast.CreateAssociationStmt{
			Name:           ast.QualifiedName{Module: "M", Name: "Child_Parent"},
			Parent:         ast.QualifiedName{Module: "M", Name: "Child"},
			Child:          ast.QualifiedName{Module: "M", Name: "Parent"},
			Type:           ast.AssocReference,
			DeleteBehavior: ast.DeleteCascade,
			Storage:        s,
		}
	}

	ctx, assoc := assocFixture(t)
	assoc.StorageFormat = domainmodel.StorageFormatTable

	r, err := diffAssociation(ctx, base(ast.StorageColumn))
	assertNoError(t, err)
	if r.Proposed == r.Current {
		t.Errorf("Table → Column reported as no change\n--- current ---\n%s\n--- proposed ---\n%s", r.Current, r.Proposed)
	}

	for _, s := range []ast.StorageType{ast.StorageDefault, ast.StorageTable} {
		r, err = diffAssociation(ctx, base(s))
		assertNoError(t, err)
		if r.Proposed != r.Current {
			t.Errorf("storage %v against stored Table reported as a change\n--- current ---\n%s\n--- proposed ---\n%s",
				s, r.Current, r.Proposed)
		}
	}
}
