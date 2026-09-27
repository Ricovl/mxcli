// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// The generic ALTER (ADR-0012 decision 2, ako/mxcli#712): one grammar rule
// `alter <type> Module.Name { set / insert / replace / drop }` for every
// document type, with the target written in one address syntax and resolved by
// the document type. These tests pin the CANONICAL spelling and that the old
// page spellings still parse to the same AST, flagged as the alias they are.

func buildAlterPage(t *testing.T, input string) *ast.AlterPageStmt {
	t.Helper()
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("want 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.AlterPageStmt)
	if !ok {
		t.Fatalf("want *ast.AlterPageStmt, got %T", prog.Statements[0])
	}
	return stmt
}

func TestGenericAlter_CanonicalSetIsParenthesisedAndColon(t *testing.T) {
	stmt := buildAlterPage(t, `alter page Module.Page {
		set (Caption: 'Save', ButtonStyle: Success) on btnSave;
		set (Title: 'Edit order');
	};`)
	if len(stmt.Operations) != 2 {
		t.Fatalf("want 2 operations, got %d", len(stmt.Operations))
	}
	onWidget := stmt.Operations[0].(*ast.SetPropertyOp)
	if onWidget.Target.Widget != "btnSave" {
		t.Errorf("target: got %q", onWidget.Target.Widget)
	}
	if onWidget.Properties["Caption"] != "Save" || onWidget.Properties["ButtonStyle"] != "Success" {
		t.Errorf("properties: got %v", onWidget.Properties)
	}
	if onWidget.Legacy != "" {
		t.Errorf("canonical set must not be flagged as an alias, got %q", onWidget.Legacy)
	}
	pageLevel := stmt.Operations[1].(*ast.SetPropertyOp)
	if pageLevel.Target.Widget != "" || pageLevel.Properties["Title"] != "Edit order" {
		t.Errorf("page-level set: target %q, properties %v", pageLevel.Target.Widget, pageLevel.Properties)
	}
	if pageLevel.Legacy != "" {
		t.Errorf("canonical page-level set flagged as alias: %q", pageLevel.Legacy)
	}
}

func TestGenericAlter_CanonicalDropNamesTargetsWithoutKeyword(t *testing.T) {
	stmt := buildAlterPage(t, `alter snippet Module.Snip {
		drop txtOld, dgOrders.Total;
	};`)
	if stmt.ContainerType != "SNIPPET" {
		t.Errorf("container type: got %q", stmt.ContainerType)
	}
	drop := stmt.Operations[0].(*ast.DropWidgetOp)
	if len(drop.Targets) != 2 || drop.Targets[0].Widget != "txtOld" ||
		drop.Targets[1].Widget != "dgOrders" || drop.Targets[1].Column != "Total" {
		t.Errorf("targets: got %+v", drop.Targets)
	}
	if drop.Legacy != "" {
		t.Errorf("canonical drop flagged as alias: %q", drop.Legacy)
	}
}

// A widget may be NAMED like a keyword the old forms use; the canonical drop
// of it must still parse as a drop of that name.
func TestGenericAlter_DropOfWidgetNamedLikeAKeyword(t *testing.T) {
	stmt := buildAlterPage(t, `alter page Module.Page { drop widget; };`)
	drop := stmt.Operations[0].(*ast.DropWidgetOp)
	if len(drop.Targets) != 1 || drop.Targets[0].Widget != "widget" || drop.Legacy != "" {
		t.Errorf("got %+v legacy=%q", drop.Targets, drop.Legacy)
	}
}

func TestGenericAlter_TargetAddressForms(t *testing.T) {
	stmt := buildAlterPage(t, `alter layout Module.Lay {
		insert into layoutContainer.top { snippetcall bar (Snippet: Module.Bar) }
		insert after 'Approve order'@2 { textbox t1 (Label: 'x') }
		replace hdr@1 with { container c1 }
	};`)
	if stmt.ContainerType != "LAYOUT" {
		t.Errorf("container type: got %q", stmt.ContainerType)
	}
	into := stmt.Operations[0].(*ast.InsertWidgetOp)
	if into.Position != "INTO" || into.Target.Widget != "layoutContainer" || into.Target.Column != "top" {
		t.Errorf("into: %+v", into)
	}
	byCaption := stmt.Operations[1].(*ast.InsertWidgetOp)
	if byCaption.Target.Caption != "Approve order" || byCaption.Target.Ordinal != 2 || byCaption.Target.Widget != "" {
		t.Errorf("caption target: %+v", byCaption.Target)
	}
	repl := stmt.Operations[2].(*ast.ReplaceWidgetOp)
	if repl.Target.Widget != "hdr" || repl.Target.Ordinal != 1 {
		t.Errorf("ordinal target: %+v", repl.Target)
	}
}

// The old spellings are aliases: they must parse to the same operations as the
// canonical form, and record which alias was used so check/exec can warn.
func TestGenericAlter_OldSpellingsAreFlaggedAliases(t *testing.T) {
	cases := []struct {
		name, op string
		legacy   string
	}{
		{"set without parentheses", `set Caption = 'Save' on btnSave`, ast.AlterAliasSetEquals},
		{"page-level set without parentheses", `set Title = 'Edit'`, ast.AlterAliasSetEquals},
		{"parenthesised set with =", `set (Caption = 'Save', ButtonStyle = Success) on btnSave`, ast.AlterAliasSetEquals},
		{"set without parentheses, with colon", `set Caption: 'Save' on btnSave`, ast.AlterAliasSetUnparenthesised},
		{"drop widget", `drop widget txtOld, txtUnused`, ast.AlterAliasDropWidget},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stmt := buildAlterPage(t, "alter page Module.Page { "+c.op+"; };")
			var got string
			switch o := stmt.Operations[0].(type) {
			case *ast.SetPropertyOp:
				got = o.Legacy
			case *ast.DropWidgetOp:
				got = o.Legacy
			default:
				t.Fatalf("unexpected op %T", o)
			}
			if got != c.legacy {
				t.Errorf("legacy spelling: got %q, want %q", got, c.legacy)
			}
		})
	}
}

// The document-specific operations with no generic spelling yet keep their own
// form inside the generic block and are NOT aliases.
func TestGenericAlter_DocumentSpecificOperationsStillParse(t *testing.T) {
	stmt := buildAlterPage(t, `alter page Module.Page {
		set layout = Atlas_Core.TopBar map (Main as Content);
		drop template for Module.Special in lvItems;
		add variables $show: Boolean = 'true';
		drop variables $show;
	};`)
	if len(stmt.Operations) != 4 {
		t.Fatalf("want 4 operations, got %d", len(stmt.Operations))
	}
	if _, ok := stmt.Operations[0].(*ast.SetLayoutOp); !ok {
		t.Errorf("op 0: %T", stmt.Operations[0])
	}
	if _, ok := stmt.Operations[1].(*ast.DropListViewTemplateOp); !ok {
		t.Errorf("op 1: %T", stmt.Operations[1])
	}
	if _, ok := stmt.Operations[2].(*ast.AddVariableOp); !ok {
		t.Errorf("op 2: %T", stmt.Operations[2])
	}
	if _, ok := stmt.Operations[3].(*ast.DropVariableOp); !ok {
		t.Errorf("op 3: %T", stmt.Operations[3])
	}
}

// @0 would read as "no ordinal" and address whatever a bare name addresses.
func TestGenericAlter_OrdinalZeroIsRefused(t *testing.T) {
	_, errs := Build(`alter page Module.Page { drop txtName@0; };`)
	if len(errs) == 0 {
		t.Fatal("want an error for @0, got none")
	}
}
