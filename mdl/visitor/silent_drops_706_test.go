// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#706: five forms that parsed, passed `check`, and were then dropped
// or stored as something else. The three that stored a WRONG MODEL (throw,
// float/currency/date, the parenthesised association) are now refused where
// they are written, with the spelling that does work. The two that only lose
// words (enumeration-value doc comments, index names) write a correct model, so
// they are warnings raised by `check`; here it is pinned that the visitor
// carries the text to it. They never worked, so refusing them changes no
// script's meaning — it only ends the silence (ADR-0011: not header-gated).
//
// Every rejection test has a control beside it: "the form is refused" is also
// satisfied by breaking the working neighbour, which is the mistake a
// token-level rejection invites (refusing the word `Currency` would also refuse
// an enumeration called M.Currency).

// buildOK parses a script that must be accepted, and returns its statements.
func buildOK(t *testing.T, script string) []ast.Statement {
	t.Helper()
	prog, errs := Build(script)
	if len(errs) > 0 {
		t.Fatalf("expected %q to parse, got:\n%s", script, errsText(errs))
	}
	return prog.Statements
}

// wantRejected asserts the script is refused and the message carries every hint.
func wantRejected(t *testing.T, script string, hints ...string) {
	t.Helper()
	got := parseErr(t, script)
	if got == "" {
		t.Fatalf("accepted, but the model cannot store what it says:\n%s", script)
	}
	for _, h := range hints {
		if !strings.Contains(got, h) {
			t.Errorf("error does not mention %q:\n%s", h, got)
		}
	}
}

// --- 1. throw ---------------------------------------------------------------

// `throw <expr>` had a grammar rule and no listener, so the statement vanished
// from the microflow: `check` passed and `exec` wrote a flow without it.
func TestThrowStatementIsRejected(t *testing.T) {
	wantRejected(t, "create microflow M.MF () begin throw 'boom'; end;", "raise error")
	// Nested in a branch, where it is most likely to be written.
	wantRejected(t, "create microflow M.MF ($x: Boolean) begin if $x then throw 'boom'; end if; end;", "raise error")
}

func TestRaiseErrorStillParses(t *testing.T) {
	buildOK(t, "create microflow M.MF () begin raise error; end;")
}

// --- 2. float / currency / date ---------------------------------------------

// `float` and `currency` fell through buildDataType to String(unlimited) on an
// attribute and to Void in a microflow; `date` was written as DateTime.
func TestRemovedPrimitiveTypesAreRejected(t *testing.T) {
	for _, tc := range []struct{ name, script, hint string }{
		{"float attribute", "create persistent entity M.E ( Amount: float );", "Decimal"},
		{"currency attribute", "create persistent entity M.E ( Amount: currency );", "Decimal"},
		{"date attribute", "create persistent entity M.E ( Born: date );", "DateTime"},
		{"alter add attribute", "alter entity M.E add attribute Amount: Float;", "Decimal"},
		{"microflow parameter", "create microflow M.MF ($A: currency) begin log 'x'; end;", "Decimal"},
		{"microflow return", "create microflow M.MF () returns Date begin return empty; end;", "DateTime"},
		{"declare", "create microflow M.MF () begin declare $d Float = 1; end;", "Decimal"},
		{"constant", "create constant M.C type float default 1;", "Decimal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantRejected(t, tc.script, tc.hint)
		})
	}
}

// The control: the neighbouring types, and the same words where they are
// names rather than types.
func TestRemovedPrimitiveTypeWordsStillWorkAsNames(t *testing.T) {
	stmts := buildOK(t, `create persistent entity M.E (
  Amount: decimal,
  Born: datetime,
  "Date": datetime,
  Price: M.Currency,
  Kind: enumeration(M.Float)
);`)
	e := stmts[0].(*ast.CreateEntityStmt)
	want := []ast.DataTypeKind{ast.TypeDecimal, ast.TypeDateTime, ast.TypeDateTime, ast.TypeEnumeration, ast.TypeEnumeration}
	if len(e.Attributes) != len(want) {
		t.Fatalf("got %d attributes, want %d", len(e.Attributes), len(want))
	}
	for i, a := range e.Attributes {
		if a.Type.Kind != want[i] {
			t.Errorf("attribute %s: kind %v, want %v", a.Name, a.Type.Kind, want[i])
		}
	}
	buildOK(t, "create enumeration M.Currency ( USD 'US Dollar' );")
	buildOK(t, "create microflow M.MF ($d: DateTime, $n: Decimal) begin declare $x Decimal = 1; end;")
}

// --- 3. the parenthesised association form ----------------------------------

// `association X (from … to …, type: …, storage: …)` read none of its options:
// a ReferenceSet with table storage was stored as a Reference in a column.
func TestParenthesisedAssociationFormIsRejected(t *testing.T) {
	wantRejected(t,
		"create association M.A_B (from M.A to M.B, type: referenceset, storage: table);",
		"create association M.A_B from M.A to M.B", "type referenceset")
	wantRejected(t, "create association M.A_B (from M.A to M.B);", "from M.A to M.B")
}

// The control: the canonical form carries the same options and they arrive.
func TestCanonicalAssociationFormKeepsItsOptions(t *testing.T) {
	stmts := buildOK(t, "create association M.A_B from M.A to M.B type referenceset storage table;")
	a := stmts[0].(*ast.CreateAssociationStmt)
	if a.Type != ast.AssocReferenceSet || a.Storage != ast.StorageTable {
		t.Errorf("got type %v storage %v, want ReferenceSet/Table", a.Type, a.Storage)
	}
}

// --- 4. doc comments on enumeration values ----------------------------------

// Enumerations$EnumerationValue has no documentation property, so a `/** … */`
// on a value is not stored. It is a warning (MDL-ENUMDOC01, in the executor)
// rather than a refusal — the enumeration is written correctly, only the note
// is lost — so the visitor must carry the text to where the check can see it.
func TestEnumerationValueDocCommentIsCapturedForTheWarning(t *testing.T) {
	stmts := buildOK(t, `/** Order lifecycle */
create enumeration M.Status (
  /** Waiting for approval */
  Pending 'Pending',
  -- an ordinary comment
  Done 'Done'
);`)
	e := stmts[0].(*ast.CreateEnumerationStmt)
	if !strings.Contains(e.Documentation, "Order lifecycle") {
		t.Errorf("enumeration documentation lost: %q", e.Documentation)
	}
	if got := e.Values[0].Documentation; !strings.Contains(got, "Waiting for approval") {
		t.Errorf("value doc comment = %q, want it captured", got)
	}
	if got := e.Values[1].Documentation; got != "" {
		t.Errorf("a -- comment became documentation: %q", got)
	}
}

// --- 5. index names ---------------------------------------------------------

// A Mendix index is anonymous. The name is still accepted — it is the required
// part of `create index X on E (…)` and appears throughout existing scripts —
// but it is now carried to the AST so `check` can say it is not stored
// (MDL-IDX01, in the executor). These pin that the visitor captures it from
// every spelling.
func TestIndexNameIsCapturedForTheWarning(t *testing.T) {
	stmts := buildOK(t, `create persistent entity M.Cell ( "Row": integer, "Col": integer )
  index "IdxRowCol" on ("Row", "Col")
  index ("Col");
alter entity M.Cell add index IdxRow ("Row");
create index IdxCol on M.Cell ("Col" desc);`)

	e := stmts[0].(*ast.CreateEntityStmt)
	if len(e.Indexes) != 2 {
		t.Fatalf("got %d indexes, want 2", len(e.Indexes))
	}
	if got := e.Indexes[0].Name; got != "IdxRowCol" {
		t.Errorf("entity-body index name = %q, want IdxRowCol", got)
	}
	if got := e.Indexes[1].Name; got != "" {
		t.Errorf("anonymous index got a name %q", got)
	}
	if got := stmts[1].(*ast.AlterEntityStmt).Index.Name; got != "IdxRow" {
		t.Errorf("alter add index name = %q, want IdxRow", got)
	}
	if got := stmts[2].(*ast.AlterEntityStmt).Index.Name; got != "IdxCol" {
		t.Errorf("create index name = %q, want IdxCol", got)
	}
}
