// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1434: every version gate an exec handler applies is
// reported by CheckVersionGates — the pass `mxcli check -p` and exec's
// pre-flight run — on a project below the gate's floor, and none on one at or
// above it. The handlers refuse through checkVersionGates(ctx, stmt), so a
// statement in this table is refused by exec exactly when check reports it.
func TestCheckVersionGates_ReportsEveryHandlerGate(t *testing.T) {
	url := "item/{Id}"
	cases := []struct {
		name  string
		stmt  ast.Statement
		below [3]int // a version under the gate's floor
		want  string // the statement the refusal names
	}{
		{"rest client", &ast.CreateRestClientStmt{}, [3]int{10, 0, 0}, "create rest client"},
		{"data transformer", &ast.CreateDataTransformerStmt{}, [3]int{11, 8, 0}, "create data transformer"},
		{"published rest", &ast.CreatePublishedRestServiceStmt{}, [3]int{9, 24, 0}, "create published rest service"},
		{"alter published rest", &ast.AlterPublishedRestServiceStmt{}, [3]int{9, 24, 0}, "alter published rest service"},
		{"grant published rest", &ast.GrantPublishedRestServiceAccessStmt{}, [3]int{9, 24, 0}, "grant access on published rest service"},
		{"odata graphql", &ast.CreateODataServiceStmt{SupportsGraphQL: true}, [3]int{10, 13, 0}, "SupportsGraphQL"},
		{"message definition document", &ast.CreateMessageDefinitionStmt{}, [3]int{11, 14, 0}, "create message definition"},
		{"workflow group", &ast.AlterSettingsStmt{Section: "WORKFLOWS", AddGroup: true, GroupName: "G"}, [3]int{11, 1, 0}, "add group"},
		{"view entity", &ast.CreateViewEntityStmt{}, [3]int{10, 17, 0}, "create view entity"},
		{"agent model", &ast.CreateModelStmt{}, [3]int{11, 8, 0}, "create model"},
		{"consumed mcp", &ast.CreateConsumedMCPServiceStmt{}, [3]int{11, 8, 0}, "create consumed mcp service"},
		{"knowledge base", &ast.CreateKnowledgeBaseStmt{}, [3]int{11, 8, 0}, "create knowledge base"},
		{"agent", &ast.CreateAgentStmt{}, [3]int{11, 8, 0}, "create agent"},
		{"page parameters", &ast.CreatePageStmtV3{Parameters: []ast.PageParameter{{Name: "P"}}}, [3]int{9, 3, 0}, "create page with parameters"},
		{"send email", &ast.CreateMicroflowStmt{Body: []ast.MicroflowStatement{
			&ast.IfStmt{ThenBody: []ast.MicroflowStatement{&ast.SendEmailStmt{}}}}}, [3]int{11, 12, 0}, "send email"},
		{"deep link", &ast.CreateMicroflowStmt{URL: &url}, [3]int{10, 5, 0}, "a microflow URL"},
		{"synchronize unsynchronized", &ast.CreateNanoflowStmt{Body: []ast.MicroflowStatement{
			&ast.SynchronizeStmt{SyncType: "Unsynchronized"}}}, [3]int{9, 3, 0}, "synchronize unsynchronized"},
		{"workflow event handlers", &ast.CreateWorkflowStmt{EventHandlers: []ast.WorkflowEventHandlerNode{{AnyEvent: true}}}, [3]int{10, 6, 0}, "on workflow events"},
		{"workflow agent task", &ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{
			&ast.WorkflowCallMicroflowNode{Agent: true}}}, [3]int{11, 8, 0}, "call agent microflow"},
		{"event subprocess", &ast.CreateWorkflowStmt{EventSubProcesses: []ast.WorkflowEventSubProcessNode{{Name: "E"}}}, [3]int{11, 7, 0}, "event subprocess"},
		{"timer event subprocess", &ast.CreateWorkflowStmt{EventSubProcesses: []ast.WorkflowEventSubProcessNode{{Name: "E", Timer: true}}}, [3]int{11, 12, 0}, "on … timer"},
		{"notification", &ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{&ast.WorkflowNotificationNode{}}}, [3]int{11, 10, 0}, "notification activities"},
		{"alter workflow", &ast.AlterWorkflowStmt{}, [3]int{8, 18, 0}, "alter workflow"},
		{"alter workflow agent task", &ast.AlterWorkflowStmt{Operations: []ast.AlterWorkflowOp{
			&ast.InsertAfterOp{NewActivities: []ast.WorkflowActivityNode{&ast.WorkflowCallMicroflowNode{Agent: true}}}}}, [3]int{11, 8, 0}, "call agent microflow"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, c.stmt}}

			vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog)
			if len(vs) != 1 {
				t.Fatalf("want one MDL-VERSION01 below the floor, got %d: %+v", len(vs), vs)
			}
			if v := vs[0]; v.RuleID != "MDL-VERSION01" || !strings.HasPrefix(v.Message, "statement 2: ") ||
				!strings.Contains(v.Message, c.want) || v.Suggestion == "" {
				t.Errorf("violation = %+v, want statement 2 naming %q with a hint", v, c.want)
			}
			// The handler's own refusal is the same text: exec and check agree.
			if err := checkVersionGates(versionCtx(t, c.below[0], c.below[1], c.below[2]), c.stmt); err == nil ||
				!strings.Contains(vs[0].Message, strings.SplitN(err.Error(), "\n", 2)[0]) {
				t.Errorf("exec's refusal %v differs from check's %q", err, vs[0].Message)
			}

			// Control: a version with every feature passes.
			if vs := versionGateViolations(versionCtx(t, 11, 99, 0), prog); len(vs) != 0 {
				t.Errorf("control: reported at 11.99: %+v", vs)
			}
		})
	}
}

// Statements with no gate, or a gated statement without the gated clause, are
// not reported — `synchronize all` on 9.0, a page without parameters, OData
// without GraphQL.
func TestCheckVersionGates_UngatedFormsPass(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateModuleStmt{Name: "M"},
		&ast.CreateMicroflowStmt{},
		&ast.CreateNanoflowStmt{Body: []ast.MicroflowStatement{&ast.SynchronizeStmt{SyncType: "All"}}},
		&ast.CreatePageStmtV3{},
		&ast.CreateODataServiceStmt{},
		&ast.AlterSettingsStmt{Section: "MODEL"},
	}}
	if vs := versionGateViolations(versionCtx(t, 9, 0, 0), prog); len(vs) != 0 {
		t.Errorf("ungated forms reported on 9.0: %+v", vs)
	}
}

// One source of truth: a handler that called checkFeature with its own feature
// key would be a gate check -p cannot see. Every gate goes through
// statementVersionGates (or, for a gate that depends on stored state, a helper
// in version_gates.go).
func TestCheckFeatureIsOnlyCalledFromTheGateTable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`\bcheckFeature\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "version_gates.go" || f == "cmd_features.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if call.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s:%d calls checkFeature directly; add the gate to statementVersionGates so `check -p` applies it too", f, i+1)
			}
		}
	}
}
