// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1597 (case B). `@annotation('Why this commit is here.')` in
// an `alter microflow … insert` fragment passed check and exec ("Altered
// microflow"), and the note was not written — the visitor cannot read a
// positional parameter, so it drops the whole note and records why. The same
// line in `create or modify microflow` is refused as MDL079. The annotation
// rules (MDL059/MDL060/MDL079/MDL092) were run over create bodies only; an
// alter fragment is a body too, and reached the builder unchecked.

func parseAlterFlow(t *testing.T, src string) (*ast.Program, *ast.AlterFlowStmt) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[len(prog.Statements)-1].(*ast.AlterFlowStmt)
	if !ok {
		t.Fatalf("last statement is %T, want *ast.AlterFlowStmt", prog.Statements[len(prog.Statements)-1])
	}
	return prog, s
}

func alterFlowRuleCounts(t *testing.T, prog *ast.Program) map[string]int {
	t.Helper()
	ids := map[string]int{}
	for _, v := range ValidateProgram(prog, "") {
		ids[v.RuleID]++
	}
	return ids
}

func TestValidateProgram_ChecksAnnotationsInAnAlterFlowFragment(t *testing.T) {
	cases := []struct {
		name, fragment, rule string
	}{
		{"positional note text (the report)", "@annotation('Why this commit is here.')\n    commit $Result;", "MDL079"},
		{"typo'd annotation name", "@postion(10, 20)\n    commit $Result;", "MDL059"},
		{"curve that is not a pair", "@curve(from: (a, b))\n    commit $Result;", "MDL060"},
		{"anchor side that does not exist", "@anchor(from: sideways)\n    commit $Result;", invalidAnchorRule},
		// Nested inside a fragment's own control flow: the walk has to enter it.
		{"inside an if in the fragment", "if $Result != empty then\n      @annotation('nested')\n      commit $Result;\n    end if;", "MDL079"},
	}
	for _, op := range []string{"insert after $Result", "replace $Result with"} {
		for _, c := range cases {
			t.Run(op+"/"+c.name, func(t *testing.T) {
				prog, _ := parseAlterFlow(t, "mdl 1;\nalter microflow Probe.ACT_Do {\n  "+op+
					" begin\n    "+c.fragment+"\n  end;\n};")
				if got := alterFlowRuleCounts(t, prog)[c.rule]; got == 0 {
					t.Errorf("check passed an alter fragment carrying %s — the annotation is dropped by "+
						"the builder, so exec writes the flow without it", c.rule)
				}
			})
		}
	}
}

// The control: the two spellings of a note that DO work in an alter fragment
// must stay clean, or the check would refuse what the reporter's workaround is.
func TestValidateProgram_AlterFlowFragmentValidNotesStayClean(t *testing.T) {
	prog, _ := parseAlterFlow(t, `mdl 1;
alter microflow Probe.ACT_Do {
  insert after $Result begin
    @annotation 'Why this commit is here.'
    @curve(from: (39, 0), to: (0, 61))
    @anchor(from: right, to: left)
    commit $Result;
    @annotation(text: 'And this log.')
    log info node 'n' 'x';
  end;
};`)
	ids := alterFlowRuleCounts(t, prog)
	for _, rule := range []string{"MDL059", "MDL060", "MDL079", invalidAnchorRule} {
		if ids[rule] != 0 {
			t.Errorf("%s reported on a valid alter fragment (%d times)", rule, ids[rule])
		}
	}
}

// exec without the preflight (`-c`, the REPL, --no-check) refuses what create
// refuses: the exec-enforced annotation rules, before anything is loaded.
func TestExecAlterFlow_RefusesADroppedAnnotation(t *testing.T) {
	_, s := parseAlterFlow(t, `mdl 1;
alter microflow Probe.ACT_Do {
  insert after $Result begin
    @postion(10, 20)
    commit $Result;
  end;
};`)
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{IsConnectedFunc: func() bool { return true }}))
	err := execAlterFlow(ctx, s)
	if err == nil || !strings.Contains(err.Error(), "MDL059") {
		t.Fatalf("execAlterFlow = %v, want a refusal naming MDL059", err)
	}
}
