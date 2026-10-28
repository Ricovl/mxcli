// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// MDL-WF20: a user task whose page offers no way to choose one of its
// outcomes. The task page is the only place a user finishes a task, and Mendix
// adds no buttons to it; a page that only shows the task passes check, mx check
// and the build, and every instance stalls at the task. An agent eval generated
// three such pages (ako/mxcli#1709, #1710).
//
// A page can choose an outcome with a `complete task '<Outcome>'` button
// (Forms$SetTaskOutcomeClientAction), or with a button calling a microflow that
// reaches `set task outcome … '<Outcome>'`, directly or through the microflows
// it calls. What the check cannot see is not a finding: a microflow it cannot
// read, a snippet, a page the script alters. It is a warning, not a refusal —
// an outcome can be completed off the page on purpose.
const taskPageOutcomeRule = "MDL-WF20"

// maxTaskOutcomeCallDepth bounds the microflow calls followed from a button.
const maxTaskOutcomeCallDepth = 6

// ValidateTaskPageOutcomes checks the user tasks whose page the script creates.
func ValidateTaskPageOutcomes(prog *ast.Program) []linter.Violation {
	return validateTaskPageOutcomes(prog, nil)
}

// StoredTaskPageOutcomeViolations is the MDL-WF20 warnings that need the
// project: a task page stored in it, or a button calling a stored microflow.
// Only what ValidateTaskPageOutcomes did not already report is returned, so the
// two can be printed together.
func StoredTaskPageOutcomeViolations(prog *ast.Program, b backend.FullBackend) []linter.Violation {
	if prog == nil || b == nil {
		return nil
	}
	base := map[string]bool{}
	for _, v := range validateTaskPageOutcomes(prog, nil) {
		base[v.Location.DocumentName+"\x00"+v.Message] = true
	}
	var out []linter.Violation
	for _, v := range validateTaskPageOutcomes(prog, b) {
		if !base[v.Location.DocumentName+"\x00"+v.Message] {
			out = append(out, v)
		}
	}
	return out
}

func validateTaskPageOutcomes(prog *ast.Program, b backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	r := &taskOutcomeResolver{
		b:      b,
		pages:  map[string]*ast.CreatePageStmtV3{},
		flows:  map[string]*ast.CreateMicroflowStmt{},
		alters: map[string]bool{},
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			r.pages[strings.ToLower(s.Name.String())] = s
		case *ast.CreateMicroflowStmt:
			r.flows[strings.ToLower(s.Name.String())] = s
		case *ast.AlterPageStmt:
			r.alters[strings.ToLower(s.PageName.String())] = true
		}
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateWorkflowStmt)
		if !ok {
			continue
		}
		walkWorkflowActivities(workflowStatementActivities(s), func(act ast.WorkflowActivityNode) {
			n, ok := act.(*ast.WorkflowUserTaskNode)
			if !ok || len(n.Outcomes) == 0 {
				return
			}
			page := n.Page.String()
			if page == "" || page == "." {
				return
			}
			reach, ok := r.pageReach(page)
			if !ok || reach.any {
				return
			}
			var missing []string
			for _, o := range n.Outcomes {
				if !reach.outcomes[o.Caption] {
					missing = append(missing, "'"+o.Caption+"'")
				}
			}
			if len(missing) == 0 {
				return
			}
			out = append(out, taskPageOutcomeViolation(s, n, page, missing))
		})
	}
	return out
}

func taskPageOutcomeViolation(s *ast.CreateWorkflowStmt, n *ast.WorkflowUserTaskNode, page string, missing []string) linter.Violation {
	kind := "user task"
	if n.IsMultiUser {
		kind = "multi user task"
	}
	outcomes := strings.Join(missing, ", ")
	first := strings.Trim(missing[0], "'")
	return linter.Violation{
		RuleID:   taskPageOutcomeRule,
		Severity: linter.SeverityWarning,
		Location: linter.Location{
			Module:       s.Name.Module,
			DocumentType: "workflow",
			DocumentName: s.Name.Name,
		},
		Message: fmt.Sprintf(
			"%s %s in workflow %s: its page %s has no button that completes %s %s — no user can choose %s "+
				"from the task page — mx check and the build pass, and the instance waits at the task",
			kind, workflowUserTaskLabel(n), s.Name.String(), page,
			plural(len(missing), "the outcome", "the outcomes"), outcomes, plural(len(missing), "it", "them")),
		Suggestion: fmt.Sprintf(
			"Add one button per outcome to %s: `actionbutton btn%s (Caption: '%s', Action: complete task '%s')`, "+
				"or a button calling a microflow that runs `set task outcome $WorkflowUserTask '%s'`. "+
				"The user must claim the task first (see the write-workflows skill, Task pages).",
			page, identifierSafe(first), first, first, first),
	}
}

// identifierSafe keeps the letters and digits of s, for a suggested widget name.
func identifierSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Outcome"
	}
	return b.String()
}

// taskReach is the outcomes a page or microflow can complete a task with; any
// is set when something it does could not be followed.
type taskReach struct {
	outcomes map[string]bool
	any      bool
}

func (t *taskReach) merge(o taskReach) {
	if t.outcomes == nil {
		t.outcomes = map[string]bool{}
	}
	for k := range o.outcomes {
		t.outcomes[k] = true
	}
	t.any = t.any || o.any
}

func (t *taskReach) add(outcome string) {
	if t.outcomes == nil {
		t.outcomes = map[string]bool{}
	}
	t.outcomes[outcome] = true
}

// taskOutcomeResolver answers taskReach for pages and microflows from the
// script and, when b is set, the project.
type taskOutcomeResolver struct {
	b      backend.FullBackend // nil: the script only
	pages  map[string]*ast.CreatePageStmtV3
	flows  map[string]*ast.CreateMicroflowStmt
	alters map[string]bool
}

// pageReach is what the named page's buttons can complete; ok is false when the
// page cannot be judged (not created by the script and not readable, or altered).
func (r *taskOutcomeResolver) pageReach(qn string) (taskReach, bool) {
	key := strings.ToLower(qn)
	if r.alters[key] {
		return taskReach{}, false
	}
	if s := r.pages[key]; s != nil {
		var reach taskReach
		forEachWidget(s, func(w *ast.WidgetV3, _ string) {
			if strings.EqualFold(w.Type, "snippetcall") {
				reach.any = true
			}
			for _, v := range w.Properties {
				if a, ok := v.(*ast.ActionV3); ok {
					reach.merge(r.actionReach(a))
				}
			}
		})
		return reach, true
	}
	if r.b == nil {
		return taskReach{}, false
	}
	info, err := r.b.GetRawUnitByName("page", qn)
	if err != nil || info == nil || len(info.Contents) == 0 {
		return taskReach{}, false
	}
	var doc bson.D
	if err := bson.Unmarshal(info.Contents, &doc); err != nil {
		return taskReach{}, false
	}
	var reach taskReach
	r.walkStoredPage(doc, &reach)
	return reach, true
}

func (r *taskOutcomeResolver) actionReach(a *ast.ActionV3) taskReach {
	var reach taskReach
	switch a.Type {
	case "completeTask":
		reach.add(a.OutcomeValue)
	case "microflow":
		reach.merge(r.flowReach(a.Target, 0, map[string]bool{}))
	}
	return reach
}

// walkStoredPage collects the page document's complete-task actions and the
// microflows its actions call; a snippet is not followed.
func (r *taskOutcomeResolver) walkStoredPage(node any, reach *taskReach) {
	switch v := node.(type) {
	case bson.D:
		switch bsonString(v, "$Type") {
		case "Forms$SetTaskOutcomeClientAction":
			reach.add(bsonString(v, "OutcomeValue"))
		case "Forms$MicroflowAction":
			for _, e := range v {
				if s, ok := e.Value.(bson.D); ok && e.Key == "MicroflowSettings" {
					if mf := bsonString(s, "Microflow"); mf != "" {
						reach.merge(r.flowReach(mf, 0, map[string]bool{}))
					}
				}
			}
		case "Forms$SnippetCallWidget":
			reach.any = true
		}
		for _, e := range v {
			r.walkStoredPage(e.Value, reach)
		}
	case bson.A:
		for _, item := range v {
			r.walkStoredPage(item, reach)
		}
	}
}

// flowReach is the outcomes a microflow — and the microflows it calls — can
// complete a task with. One it cannot read could complete any.
func (r *taskOutcomeResolver) flowReach(qn string, depth int, visited map[string]bool) taskReach {
	key := strings.ToLower(qn)
	if depth > maxTaskOutcomeCallDepth {
		return taskReach{any: true}
	}
	if visited[key] {
		return taskReach{}
	}
	visited[key] = true

	var reach taskReach
	if s := r.flows[key]; s != nil {
		walkFlowStatements(s.Body, func(st ast.MicroflowStatement) {
			switch x := st.(type) {
			case *ast.SetTaskOutcomeStmt:
				reach.add(x.OutcomeValue)
			case *ast.CallMicroflowStmt:
				reach.merge(r.flowReach(x.MicroflowName.String(), depth+1, visited))
			}
		})
		return reach
	}
	if r.b == nil {
		return taskReach{any: true}
	}
	objs, found := (&StoredCommitEvents{b: r.b}).flowObjects(false, qn)
	if !found {
		return taskReach{any: true}
	}
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, o := range oc.Objects {
			switch a := o.(type) {
			case *microflows.ActionActivity:
				switch act := a.Action.(type) {
				case *microflows.SetTaskOutcomeAction:
					reach.add(act.OutcomeValue)
				case *microflows.MicroflowCallAction:
					if act.MicroflowCall != nil {
						reach.merge(r.flowReach(act.MicroflowCall.Microflow, depth+1, visited))
					}
				}
			case *microflows.LoopedActivity:
				walk(a.ObjectCollection)
			}
		}
	}
	walk(objs)
	return reach
}
