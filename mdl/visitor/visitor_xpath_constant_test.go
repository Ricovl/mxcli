// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1685: an XPath constraint could not name a constant. Mendix XPath
// writes one as `@Module.Constant`, exactly as an expression does, but the
// constraint grammar had no value of that shape: `@` was admitted as a ONE-WORD
// name on its own, so `[Total > @Shop.C_Limit]` failed at `Shop` ("mismatched
// input 'Shop' expecting ']'") and a Studio Pro-authored retrieve or data source
// using a constant could not be described and executed back.
//
// The lenient parse describe and the formatter use was worse than the error: it
// recovered past the module name and handed back the bare `@` as an attribute,
// so anything that re-rendered the constraint dropped the constant.

func TestXPath_ConstantReference_IsAConstantNotAName(t *testing.T) {
	expr, ok := ParseXPathConstraint("[Total > @Shop.C_Limit]")
	if !ok {
		t.Fatal("ParseXPathConstraint did not parse a constant comparand")
	}
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("built %T, want *ast.BinaryExpr", expr)
	}
	c, ok := bin.Right.(*ast.ConstantRefExpr)
	if !ok {
		t.Fatalf("right operand = %T (%#v), want *ast.ConstantRefExpr", bin.Right, bin.Right)
	}
	if got := c.QualifiedName.String(); got != "Shop.C_Limit" {
		t.Errorf("constant = %q, want Shop.C_Limit", got)
	}
}

// Every position a value takes in a constraint: either side of a comparison, a
// function argument, inside a nested predicate, under not(), and next to the
// other operand kinds. The re-render is what describe enrichment and the
// long-constraint formatter write back, so it must give back the input.
func TestXPath_ConstantReference_RoundTrips(t *testing.T) {
	for _, in := range []string{
		"[Total > @Shop.C_Limit]",
		"[@Shop.C_Limit <= Total]",
		"[contains(Name, @Shop.C_Prefix)]",
		"[Shop.Item_Order/Shop.Order[Total >= @Shop.C_Limit]/Name = @Shop.C_Name]",
		"[Total > @Shop.C_Limit and Name != empty]",
		"[not(Total > @Shop.C_Limit)]",
		"[(Total > @Shop.C_Min or Total < @Shop.C_Max) and Name = $Name]",
		"[Total > -@Shop.C_Limit]",
		// A keyword is a name part in XPath; so it is in a constant's name.
		"[Total > @Shop.Limit]",
		"[Total > @Shop.Status]",
	} {
		expr, ok := ParseXPathConstraint(in)
		if !ok {
			t.Errorf("ParseXPathConstraint(%q) did not parse", in)
			continue
		}
		if got := "[" + xpathExprToString(expr) + "]"; got != in {
			t.Errorf("round trip of %q = %q", in, got)
		}
	}
}

// `@` alone is not a name. While xpathWord admitted it, `[Name = @]` parsed as
// a comparison with an attribute called "@".
func TestXPath_ConstantReference_BareAtIsNotAName(t *testing.T) {
	src := "create microflow Shop.F ()\nbegin\n  retrieve $Items from Shop.Item where [Name = @];\nend;\n"
	if _, errs := Build(src); len(errs) == 0 {
		t.Error("`[Name = @]` parsed; want a syntax error")
	}
	// The lenient re-parse hands the visitor a recovered tree with the name
	// missing; it must not dereference what is not there.
	for _, in := range []string{"[Name = @]", "[Name = @ and Total > 1]", "[@]", "[Name = @.]"} {
		_, _ = ParseXPathConstraint(in)
	}
}

// The three writers keep the constant as written: the retrieve and the access
// rule store their source text, the data source re-renders its parse.
func TestXPath_ConstantReference_StoredVerbatimInEverySlot(t *testing.T) {
	t.Run("retrieve", func(t *testing.T) {
		for _, tc := range []struct{ where, want string }{
			{"[Total > @Shop.C_Limit]", "[Total > @Shop.C_Limit]"},
			{"[Total > @Shop.C_Limit][Name != empty]", "[Total > @Shop.C_Limit][Name != empty]"},
		} {
			prog := mustBuild(t, "create microflow Shop.F ()\nbegin\n  retrieve $Items from Shop.Item where "+tc.where+";\nend;\n")
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			r, ok := mf.Body[0].(*ast.RetrieveStmt)
			if !ok {
				t.Fatalf("body[0] = %T, want *ast.RetrieveStmt", mf.Body[0])
			}
			src, ok := r.Where.(*ast.SourceExpr)
			if !ok {
				t.Fatalf("where %s built %T, want *ast.SourceExpr", tc.where, r.Where)
			}
			if got := strings.TrimSpace(src.Source); got != tc.want && "["+got+"]" != tc.want {
				t.Errorf("where %s stored as %q, want %q", tc.where, got, tc.want)
			}
		}
	})

	t.Run("data source", func(t *testing.T) {
		// The unbracketed form goes through the expression grammar, which always
		// had constants — and then through the same renderer, which had no case
		// for one and stored `[Total > ]`.
		for _, where := range []string{
			"[Total > @Shop.C_Limit and Name != empty]",
			"Total > @Shop.C_Limit and Name != empty",
		} {
			prog := mustBuild(t, `create page Shop.P (title: 'T', layout: Atlas_Core.Atlas_Default) {
  gallery g (datasource: database from Shop.Item where `+where+`) {
    dynamictext t (content: 'x')
  }
}`)
			page := prog.Statements[0].(*ast.CreatePageStmtV3)
			ds := findWidget(page.Widgets, "g").Properties["DataSource"].(*ast.DataSourceV3)
			if want := "[Total > @Shop.C_Limit and Name != empty]"; ds.Where != want {
				t.Errorf("where %s: datasource Where = %q, want %q", where, ds.Where, want)
			}
		}
	})

	t.Run("access rule", func(t *testing.T) {
		g, _ := grantStmt(t, `grant read * on entity Shop.Item to Shop.User where [Total > @Shop.C_Limit];`)
		if want := "[Total > @Shop.C_Limit]"; g.XPathConstraint != want {
			t.Errorf("XPathConstraint = %q, want %q", g.XPathConstraint, want)
		}
	})
}

// A constraint too long for one line is laid out from its parse. The layout
// must keep the constant; a recovered parse that read `@` as a name wrote the
// constraint back without it.
func TestXPath_ConstantReference_SurvivesTheLongConstraintLayout(t *testing.T) {
	in := "[Total > @Shop.C_Limit and Name != empty and Description != empty and Category = 'Hardware']"
	got := FormatXPathConstraint(in)
	if got == in {
		t.Fatalf("constraint of %d characters was not laid out; the test needs one that is", len(in))
	}
	if !strings.Contains(got, "Total > @Shop.C_Limit") {
		t.Errorf("layout dropped the constant:\n%s", got)
	}
	if flat := FlattenXPathConstraint(got); flat != in {
		t.Errorf("layout changed the constraint:\n got %s\nwant %s", flat, in)
	}
}
