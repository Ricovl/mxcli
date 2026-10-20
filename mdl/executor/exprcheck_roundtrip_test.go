// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestExprCheckRoundTrip_DescribeProducesZeroHints walks every microflow in the
// test project, regenerates its MDL with DESCRIBE, re-parses each expression
// with the robust expression parser, and asserts that no hint fires.
//
// The source is known-good — it round-trips through mxcli's own describer — so
// any hint is a gap in the expression grammar rather than a defect in the
// model: a missing parsePrimary branch, a missing funcTable entry, or a missing
// slot constraint. Those gaps are invisible to the unit tests, which only feed
// the parser expressions someone thought to write down.
//
// This replaces mdl/exprcheck/roundtrip_test.go, which asserted the same thing
// but could not run. It sat behind `//go:build roundtrip`, a tag no Makefile
// target and no CI job ever passed, and it called three things that have since
// been removed: mdl/backend/mpr (the legacy engine, deleted),
// executor.GetHierarchyForMining and executor.DescribeMicroflowToString. So it
// had stopped compiling under its own tag long before anyone noticed, and
// `go mod tidy` refused to run at all while it referenced the deleted package.
//
// It lives here, in package executor under the `integration` tag, because that
// is where the real-project harness already is (setupTestEnv, describeMDL) and
// because `make test-integration-executor` actually runs it. The import cycle
// the original file's comment worried about does not arise in this direction:
// mdl/exprcheck does not import mdl/executor.
func TestExprCheckRoundTrip_DescribeProducesZeroHints(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	be := env.executor.Backend()
	if be == nil {
		t.Fatal("no backend on the test executor")
	}

	// A microflow's ContainerID is its FOLDER when it sits in one, not its
	// module, so a flat module-id map resolves almost nothing: measured on
	// testdata/expr-checker/minimal.mpr, a map built from ListModules named 1
	// of 16 microflows, while the hierarchy walk named all 16. That is what
	// the removed executor.GetHierarchyForMining did, and dropping it is what
	// made the first run of this test describe a single microflow and find no
	// expressions at all.
	h, err := NewContainerHierarchyFromBackend(be)
	if err != nil {
		t.Fatalf("container hierarchy: %v", err)
	}

	mfs, err := be.ListMicroflows()
	if err != nil {
		t.Fatalf("list microflows: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("the test project has no microflows; this test would pass vacuously")
	}

	parser := exprcheck.NewParser()
	resolver := exprcheck.DefaultSlotResolver()

	var described, slots, hints int
	for _, mf := range mfs {
		modName := h.GetModuleName(h.FindModuleID(mf.ContainerID))
		if modName == "" || mf.Name == "" {
			continue
		}
		qn := ast.QualifiedName{Module: modName, Name: mf.Name}

		mdl, err := env.describeMDL(fmt.Sprintf("describe microflow %s;", qn.String()))
		if err != nil {
			t.Errorf("describe %s: %v", qn.String(), err)
			continue
		}
		described++

		prog, errs := visitor.Build(mdl)
		if len(errs) > 0 {
			t.Errorf("re-parsing the describe of %s failed: %v", qn.String(), errs)
			continue
		}

		for _, st := range prog.Statements {
			mfStmt, ok := st.(*ast.CreateMicroflowStmt)
			if !ok {
				continue
			}
			visitMicroflowExpressions(mfStmt, func(slotPath string, expr ast.Expression) {
				src := expressionSource(expr)
				if src == "" {
					return
				}
				slots++
				_, hs := parser.Parse(src, exprcheck.Context{
					SlotPath:  slotPath,
					Microflow: qn.String(),
					Slots:     resolver,
				})
				for _, h := range hs {
					hints++
					t.Errorf("hint on known-good source %s [%s] %q: code=%s problem=%q",
						qn.String(), slotPath, src, h.Code, h.Problem)
				}
			})
		}
	}

	if described == 0 {
		t.Fatal("no microflow could be described; the walk proved nothing")
	}
	// Not a vacuous pass: the project must actually contain expressions.
	if slots == 0 {
		t.Errorf("described %d microflow(s) but found no expression slots; "+
			"either the test project lost its expressions or the walk below "+
			"no longer matches the AST", described)
	}
	t.Logf("round-trip: %d microflow(s) described, %d expression slot(s), %d hint(s)",
		described, slots, hints)
}

// expressionSource returns the literal source of an expression the parser can
// re-check, or "" for an expression the describer did not emit as text.
func expressionSource(e ast.Expression) string {
	if se, ok := e.(*ast.SourceExpr); ok {
		return se.Source
	}
	return ""
}

// visitMicroflowExpressions calls fn for every expression slot in the body.
//
// It mirrors adapters.walkBody but stays self-contained, so a change to the
// adapter's traversal cannot quietly shrink what this test covers — if the two
// drift, the slot count above is what notices.
func visitMicroflowExpressions(mf *ast.CreateMicroflowStmt, fn func(string, ast.Expression)) {
	var walk func([]ast.MicroflowStatement)
	walk = func(body []ast.MicroflowStatement) {
		for _, s := range body {
			switch x := s.(type) {
			case *ast.IfStmt:
				if x.Condition != nil {
					fn("IfStmt.Condition", x.Condition)
				}
				walk(x.ThenBody)
				walk(x.ElseBody)
			case *ast.WhileStmt:
				if x.Condition != nil {
					fn("WhileStmt.Condition", x.Condition)
				}
				walk(x.Body)
			case *ast.LoopStmt:
				walk(x.Body)
			case *ast.ReturnStmt:
				if x.Value != nil {
					fn("ReturnStmt.Value", x.Value)
				}
			case *ast.DeclareStmt:
				if x.InitialValue != nil {
					fn("DeclareStmt.InitialValue", x.InitialValue)
				}
			case *ast.MfSetStmt:
				if x.Value != nil {
					fn("MfSetStmt.Value", x.Value)
				}
			case *ast.LogStmt:
				if x.Message != nil {
					fn("LogStmt.Message", x.Message)
				}
			case *ast.CreateObjectStmt:
				entityQN := x.EntityType.String()
				for _, ci := range x.Changes {
					if ci.Value != nil {
						fn("CreateItem.Value:"+entityQN+"."+ci.Attribute, ci.Value)
					}
				}
			case *ast.ChangeObjectStmt:
				for _, ci := range x.Changes {
					if ci.Value != nil {
						fn("ChangeItem.Value", ci.Value)
					}
				}
			case *ast.CallMicroflowStmt:
				for _, a := range x.Arguments {
					if a.Value != nil {
						fn("CallArgument.Value", a.Value)
					}
				}
			case *ast.CallNanoflowStmt:
				for _, a := range x.Arguments {
					if a.Value != nil {
						fn("CallArgument.Value", a.Value)
					}
				}
			}
		}
	}
	walk(mf.Body)
}
