// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// countingMicroflowDecl is the decl `create or modify` makes of a described
// PedApp microflow, edited by edit, with its build counted.
func countingMicroflowDecl(t *testing.T, exec *Executor, name string, edit func(string) string) (*flowDecl, *int) {
	t.Helper()
	var buf bytes.Buffer
	exec.output = &buf
	if err := afRun(t, exec, "describe microflow "+name+";"); err != nil {
		t.Fatal(err)
	}
	src := edit(buf.String())
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("the description does not parse: %v\n%s", errs[0], src)
	}
	var s *ast.CreateMicroflowStmt
	for _, st := range prog.Statements {
		if c, ok := st.(*ast.CreateMicroflowStmt); ok {
			s = c
		}
	}
	if s == nil {
		t.Fatalf("no create microflow in the description:\n%s", src)
	}
	d := microflowDecl(s)
	n := new(int)
	build := d.build
	d.build = func(ctx *ExecContext) (any, []*microflows.MicroflowParameter, map[string]string, error) {
		*n++
		return build(ctx)
	}
	return d, n
}

// A `create or modify` that states what describe prints for the stored flow
// has nothing to patch and does not build the declared flow to find that out;
// one that changes the body builds it once, however many steps need it
// (ako/mxcli#870: the round-trip suite built every flow up to three times).
func TestPlanFlowModify_BuildsAtMostOnce(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	const name = "FeedbackModule.VAL_Feedback"

	t.Run("unchanged", func(t *testing.T) {
		d, builds := countingMicroflowDecl(t, exec, name, func(s string) string { return s })
		p, err := planFlowModify(exec.newExecContext(context.Background()), d)
		if err != nil || p == nil {
			t.Fatalf("plan: %v %v", p, err)
		}
		if len(p.ops) != 0 || len(p.moves) != 0 || p.mut != nil {
			t.Fatalf("the unchanged description patches the flow: %d ops, %d moves", len(p.ops), len(p.moves))
		}
		if *builds != 0 {
			t.Errorf("the unchanged description built the declared flow %d time(s), want 0", *builds)
		}
	})

	// The control: a changed body is patched, so the plan above was not empty
	// because planning finds nothing.
	t.Run("changed", func(t *testing.T) {
		d, builds := countingMicroflowDecl(t, exec, name, func(s string) string {
			const msg = "'Email is required'"
			if !strings.Contains(s, msg) {
				t.Fatalf("the description has no %s:\n%s", msg, s)
			}
			return strings.Replace(s, msg, "'An email address is required'", 1)
		})
		p, err := planFlowModify(exec.newExecContext(context.Background()), d)
		if err != nil || p == nil {
			t.Fatalf("plan: %v %v", p, err)
		}
		if len(p.ops) == 0 {
			t.Fatal("the changed message is not patched in")
		}
		if *builds != 1 {
			t.Errorf("the changed body built the declared flow %d time(s), want 1", *builds)
		}
	})
}
