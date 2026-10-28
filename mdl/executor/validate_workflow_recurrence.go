// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// MDL-WF22: a timer recurrence Studio Pro refuses (ako/mxcli#1716).
// `repeat every <n> <unit> at most <m> times` repeats a non-interrupting timer
// — a boundary event or an event sub-process's start. Studio Pro 11.15's model
// validation (the workflow schema shipped with mxbuild) allows:
//
//	unit        minute, hour, day, week, month   (MW0005; second and year are in
//	                                              the enumeration but refused)
//	interval    at least 1
//	max         at least 2, counting the first firing (MW0004)
//
// and only a non-interrupting timer has a Recurrence. mx check enforces none of
// these — it accepted IntervalType 'Bogus' — so they are checked here.
const wfRecurrenceRule = "MDL-WF22"

// wfRecurrenceSite is one `repeat every …` a workflow statement writes.
type wfRecurrenceSite struct {
	where          string
	r              *ast.WorkflowRecurrenceNode
	interrupting   bool
	interruptLabel string // "an interrupting timer" or "a timer that names no kind"
}

func checkWorkflowRecurrences(sites []wfRecurrenceSite, loc linter.Location) []linter.Violation {
	var out []linter.Violation
	report := func(msg, suggestion string) {
		out = append(out, linter.Violation{RuleID: wfRecurrenceRule, Severity: linter.SeverityError, Location: loc, Message: msg, Suggestion: suggestion})
	}
	for _, s := range sites {
		if s.interrupting {
			report(fmt.Sprintf("%s repeats, but it is %s — only a non-interrupting timer has a recurrence; an interrupting one ends its activity the first time it fires", s.where, s.interruptLabel),
				"Make it `non interrupting timer … repeat every …`, or drop the `repeat every` clause.")
			continue
		}
		unit, known := workflows.RecurrenceIntervalType(s.r.Unit)
		switch {
		case !known || unit == "Second" || unit == "Year":
			report(fmt.Sprintf("%s repeats every %d %s — a recurrence repeats every minute, hour, day, week or month (Studio Pro MW0005; mx check does not catch it)", s.where, s.r.Interval, s.r.Unit),
				"Write the unit as minute(s), hour(s), day(s), week(s) or month(s).")
		case s.r.Interval < 1:
			report(fmt.Sprintf("%s repeats every %d %s — the interval is at least 1", s.where, s.r.Interval, s.r.Unit), "Repeat every 1 or more.")
		}
		if s.r.MaxExecutions < 2 {
			report(fmt.Sprintf("%s repeats at most %d time(s) — the count includes the first firing, so it is at least 2 (Studio Pro MW0004; mx check does not catch it)", s.where, s.r.MaxExecutions),
				"`at most 2 times` is the first firing and one repeat.")
		}
	}
	return out
}

// boundaryRecurrenceSites collects the recurrences of the boundary events on
// the activities given, at any depth.
func boundaryRecurrenceSites(acts []ast.WorkflowActivityNode) []wfRecurrenceSite {
	var sites []wfRecurrenceSite
	walkWorkflowActivities(acts, func(a ast.WorkflowActivityNode) {
		for _, be := range workflowActivityBoundaryEvents(a) {
			if be.Recurrence != nil {
				sites = append(sites, boundaryRecurrenceSite("a timer boundary event on "+boundaryHostLabel(a), be.EventType, be.Recurrence))
			}
		}
	})
	return sites
}

func boundaryRecurrenceSite(where, eventType string, r *ast.WorkflowRecurrenceNode) wfRecurrenceSite {
	s := wfRecurrenceSite{where: where, r: r}
	switch eventType {
	case "InterruptingTimer":
		s.interrupting, s.interruptLabel = true, "an interrupting timer"
	case "Timer":
		s.interrupting, s.interruptLabel = true, "a timer that names no kind (written as interrupting)"
	}
	return s
}

// workflowRecurrenceSites collects the recurrences a CREATE WORKFLOW writes.
func workflowRecurrenceSites(stmt *ast.CreateWorkflowStmt) []wfRecurrenceSite {
	sites := boundaryRecurrenceSites(workflowStatementActivities(stmt))
	for _, esp := range stmt.EventSubProcesses {
		if esp.Recurrence == nil {
			continue
		}
		s := wfRecurrenceSite{where: "the timer start of event sub-process " + esp.Name, r: esp.Recurrence}
		if esp.Interrupting {
			s.interrupting, s.interruptLabel = true, "an interrupting start"
		}
		sites = append(sites, s)
	}
	return sites
}

// alterWorkflowRecurrenceSites collects the recurrences an ALTER WORKFLOW writes.
func alterWorkflowRecurrenceSites(stmt *ast.AlterWorkflowStmt) []wfRecurrenceSite {
	sites := boundaryRecurrenceSites(alterWorkflowAddedActivities(stmt))
	for _, op := range stmt.Operations {
		if o, ok := op.(*ast.InsertBoundaryEventOp); ok && o.Recurrence != nil {
			sites = append(sites, boundaryRecurrenceSite("the timer inserted on "+o.ActivityRef, o.EventType, o.Recurrence))
		}
	}
	return sites
}

// boundaryHostLabel names an activity that carries boundary events.
func boundaryHostLabel(a ast.WorkflowActivityNode) string {
	switch n := a.(type) {
	case *ast.WorkflowCallWorkflowNode:
		return "call workflow " + n.Workflow.String()
	case *ast.WorkflowWaitForNotificationNode:
		return "wait for notification"
	}
	return branchingLabel(a)
}
