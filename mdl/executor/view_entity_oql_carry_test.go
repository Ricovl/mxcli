// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// storeViewOQL overwrites a view entity's stored OQL — both the source document
// and the entity's copy — the way a Studio Pro edit leaves it, so a test can
// start from text mxcli would never have written.
func storeViewOQL(t *testing.T, exec *Executor, module, name, oql string) {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	mod, err := findModule(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	dm, err := ctx.Backend.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	ent := dm.FindEntityByName(name)
	if ent == nil {
		t.Fatalf("entity %s.%s not found", module, name)
	}
	if _, err := ctx.Backend.WriteViewEntitySourceDocument(mod.ID, module, name, oql, ""); err != nil {
		t.Fatal(err)
	}
	ent.OqlQuery = oql
	if err := ctx.Backend.UpdateEntity(dm.ID, ent); err != nil {
		t.Fatal(err)
	}
}

// storedViewOQL reads a view entity's OQL back from the project.
func storedViewOQL(t *testing.T, exec *Executor, module, name string) string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	mod, err := findModule(ctx, module)
	if err != nil {
		t.Fatal(err)
	}
	dm, err := ctx.Backend.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	ent := dm.FindEntityByName(name)
	if ent == nil {
		t.Fatalf("entity %s.%s not found", module, name)
	}
	return ent.OqlQuery
}

// mendixlabs/mxcli#1627, second symptom: the view entity round trip re-indented
// the OQL. The script's query is the text between its first and last token,
// dedented — so describe → exec cannot tell a stored indentation from the
// script's own, and drops whitespace and comments outside the tokens. Each
// shape here is stored text that describe → `create or modify` rewrote.
//
// The fix keeps the stored bytes when the statement's query is exactly what the
// stored query describes to; the control is a real edit, which must still land.
func TestCreateOrModifyViewEntity_KeepsStoredOQLItDescribes(t *testing.T) {
	shapes := map[string]string{
		"indented first line": "  SELECT t.ID AS VE_Target, t.Code AS Code\n  FROM VEA.Target AS t",
		"tab indented":        "\tSELECT t.ID AS VE_Target, t.Code AS Code\n\tFROM VEA.Target AS t",
		"leading comment":     "-- view over targets\nSELECT t.ID AS VE_Target, t.Code AS Code\nFROM VEA.Target AS t",
		"trailing newline":    "SELECT t.ID AS VE_Target, t.Code AS Code\r\nFROM VEA.Target AS t\r\n",
		"leading blank line":  "\nSELECT t.ID AS VE_Target, t.Code AS Code\nFROM VEA.Target AS t",
	}
	for name, stored := range shapes {
		t.Run(name, func(t *testing.T) {
			exec, out, _ := openPedAppCopy(t)
			if err := agreeExec(t, exec, viewAssociationSetup); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			storeViewOQL(t, exec, "VEA", "VE", stored)

			out.Reset()
			if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeEntity, Name: ast.QualifiedName{Module: "VEA", Name: "VE"}}); err != nil {
				t.Fatalf("describe: %v", err)
			}
			described := out.String()
			if err := agreeExec(t, exec, described); err != nil {
				t.Fatalf("exec of the description: %v\n%s", err, described)
			}
			if got := storedViewOQL(t, exec, "VEA", "VE"); got != stored {
				t.Errorf("describe → exec rewrote the stored OQL\nwant %q\n got %q\ndescribed:\n%s", stored, got, described)
			}

			// Control: an edit to the query is not a re-description, and lands.
			edited := strings.Replace(described, "t.Code AS Code", "t.Code AS Code -- edited", 1)
			if edited == described {
				t.Fatalf("control could not edit the description:\n%s", described)
			}
			if err := agreeExec(t, exec, edited); err != nil {
				t.Fatalf("exec of the edited description: %v", err)
			}
			if got := storedViewOQL(t, exec, "VEA", "VE"); !strings.Contains(got, "-- edited") {
				t.Errorf("control: an edited query must be written, stored %q", got)
			}
		})
	}
}
