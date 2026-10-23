// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1543: "LOG cannot set 'Include latest stack trace'" —
// `LOG ERROR NODE 'X' 'failed' WITH STACKTRACE;` was a parse error, so every
// log written through MDL stored IncludeLatestStackTrace = false.
func buildLogAction(t *testing.T, body string) *microflows.LogMessageAction {
	t.Helper()
	prog, errs := visitor.Build("create or modify microflow M.F () begin " + body + " end;")
	for _, e := range errs {
		t.Fatalf("parse error: %v", e)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
	}
	fb.buildFlowGraph(mf.Body, nil)
	var logs []*microflows.LogMessageAction
	for _, obj := range fb.objects {
		if a, ok := obj.(*microflows.ActionActivity); ok {
			if l, ok := a.Action.(*microflows.LogMessageAction); ok {
				logs = append(logs, l)
			}
		}
	}
	if len(logs) != 1 {
		t.Fatalf("want 1 log activity, got %d", len(logs))
	}
	return logs[0]
}

func TestLogWithStackTrace_Issue1543(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"reported spelling", "log error node 'X' 'failed' with stacktrace; return;", true},
		{"after template params", "log error 'failed {1}' with ({1} = 'a') with stacktrace; return;", true},
		{"before on error", "log error 'failed' with stacktrace on error continue; return;", true},
		{"absent", "log error node 'X' 'failed'; return;", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildLogAction(t, tc.body).IncludeLastStackTrace; got != tc.want {
				t.Errorf("IncludeLatestStackTrace = %v, want %v", got, tc.want)
			}
		})
	}

	// The template parameters and the stack trace are two clauses that both
	// open with `with`; neither may swallow the other.
	act := buildLogAction(t, "log error 'failed {1}' with ({1} = 'a') with stacktrace; return;")
	if len(act.TemplateParameters) != 1 {
		t.Errorf("template parameters lost next to `with stacktrace`: %v", act.TemplateParameters)
	}
}

// DESCRIBE has to print the flag, or a Studio Pro-authored log with the box
// ticked loses it on describe → exec. The rendered form must parse back to
// the same flag.
func TestDescribeLogWithStackTrace_Issue1543(t *testing.T) {
	for _, include := range []bool{true, false} {
		action := &microflows.LogMessageAction{
			LogLevel:              microflows.LogLevelError,
			LogNodeName:           "'X'",
			IncludeLastStackTrace: include,
			TemplateParameters:    []string{"$Order/Number"},
		}
		rendered := formatAction(nil, action, nil, nil)
		if has := strings.Contains(rendered, "with stacktrace"); has != include {
			t.Fatalf("IncludeLatestStackTrace=%v rendered as %q", include, rendered)
		}
		prog, errs := visitor.Build("create microflow M.A ($Order: M.Order) begin " + rendered + " end;")
		if len(errs) > 0 {
			t.Fatalf("DESCRIBE emitted MDL the parser rejects.\n  rendered: %s\n  errors:   %v", rendered, errs)
		}
		log := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.LogStmt)
		if log.IncludeLatestStackTrace != include {
			t.Errorf("round trip changed the flag: %v -> %v (rendered %q)", include, log.IncludeLatestStackTrace, rendered)
		}
	}
}

// `stacktrace` becoming a keyword must not take the System.Error attribute
// of the same name away from expressions: `$latestError/Stacktrace` is the
// usual thing to log from an error handler.
func TestLatestErrorStacktraceStillParses_Issue1543(t *testing.T) {
	act := buildLogAction(t, "log error 'failed: {1}' with ({1} = $latestError/Stacktrace); return;")
	if len(act.TemplateParameters) != 1 || !strings.Contains(act.TemplateParameters[0], "Stacktrace") {
		t.Errorf("template parameter = %v, want $latestError/Stacktrace", act.TemplateParameters)
	}
}
