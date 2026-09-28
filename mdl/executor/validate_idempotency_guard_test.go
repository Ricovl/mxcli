// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestMDL067RejectsContradictoryGuards covers the one combination the grammar
// allows and nobody can mean: OR MODIFY rebuilds an existing element from the
// statement, IF NOT EXISTS leaves it untouched. Written together one is
// silently ignored, and which one is not readable from the statement.
func TestMDL067RejectsContradictoryGuards(t *testing.T) {
	for _, src := range []string{
		`create or modify entity if not exists M."Game" ("Level": string(20));`,
		`create or modify association if not exists M.Move_Game from M.Move to M.Game;`,
		// Every other document kind carries the same guard (ako/mxcli#731).
		`create or modify page if not exists M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dynamictext t (Content: 'x') };`,
		`create or modify microflow if not exists M.MF () begin return; end;`,
		`create or modify enumeration if not exists M.E (A 'A');`,
		`create or replace constant if not exists M.C type String default 'a';`,
		`create or modify module role if not exists M.User;`,
		`create or modify configuration if not exists 'Default';`,
		`create or modify view entity if not exists M.V (Name: String(100)) as (select c.Name as Name from M.C as c);`,
	} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("%s\n  parse errors: %v", src, errs)
		}
		var got []string
		for _, v := range ValidateProgram(prog, "") {
			if v.RuleID == "MDL067" {
				got = append(got, v.Message+" / "+v.Suggestion)
			}
		}
		if len(got) != 1 {
			t.Fatalf("%s\n  MDL067 fired %d times, want 1", src, len(got))
		}
		if !strings.Contains(got[0], "if not exists") || !strings.Contains(got[0], "or modify") {
			t.Errorf("the message should name both halves, got: %s", got[0])
		}
	}
}

// TestMDL067LeavesEitherGuardAlone is the control. Each spelling on its own is
// the whole point of the feature, so a rule that fires on them would be worse
// than no rule.
func TestMDL067LeavesEitherGuardAlone(t *testing.T) {
	for _, src := range []string{
		`create entity if not exists M."Game" ("Level": string(20));`,
		`create or modify entity M."Game" ("Level": string(20));`,
		`create association if not exists M.Move_Game from M.Move to M.Game;`,
		`create or modify association M.Move_Game from M.Move to M.Game;`,
		`create entity M."Game" ("Level": string(20));`,
		`create page if not exists M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dynamictext t (Content: 'x') };`,
		`create or modify page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dynamictext t (Content: 'x') };`,
		`create microflow if not exists M.MF () begin return; end;`,
		`create module role if not exists M.User;`,
	} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("%s\n  parse errors: %v", src, errs)
		}
		for _, v := range ValidateProgram(prog, "") {
			if v.RuleID == "MDL067" {
				t.Errorf("MDL067 fired on a valid statement:\n  %s\n  %s", src, v.Message)
			}
		}
	}
}

// TestCreateEntityIfNotExistsSkipsWithoutTouching pins the semantic difference
// from CREATE OR MODIFY, which is the whole reason the guard was added: the
// stored definition is left exactly as it is, so a partial statement cannot
// drop the attributes it omits (sudoku finding #24).
func TestCreateEntityIfNotExistsSkipsWithoutTouching(t *testing.T) {
	ctx, game, updated := indexTestCtx(t)
	before := len(game.Attributes)
	err := execCreateEntity(ctx, &ast.CreateEntityStmt{
		Name:        ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Kind:        ast.EntityPersistent,
		Attributes:  []ast.Attribute{{Name: "OnlyThisOne", Type: ast.DataType{Kind: ast.TypeInteger}}},
		CreateGuard: ast.CreateGuard{IfNotExists: true},
	})
	assertNoError(t, err)
	if *updated {
		t.Error("expected no write for an entity that already exists")
	}
	if len(game.Attributes) != before {
		t.Errorf("the stored entity went from %d attributes to %d — IF NOT EXISTS must not touch it",
			before, len(game.Attributes))
	}
	if out := ctxOutput(ctx); !strings.Contains(out, "skipped") {
		t.Errorf("expected a skip notice, got: %q", out)
	}
}
