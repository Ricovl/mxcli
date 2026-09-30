// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"strconv"
	"strings"
)

// The generic ALTER (ADR-0012 decision 2) is one patch grammar for every
// document type:
//
//	alter <type> Module.Name {
//	  set ( Key: value ) on <target>;
//	  insert before|after|into <target> { <fragment> }
//	  replace <target> with { <fragment> }
//	  drop <target>;
//	}
//
// The grammar and the operations are shared; what differs per document type is
// what a <target> means. A page addresses widgets by name, a microflow has no
// names and addresses activities by content. That is the resolver's job, and
// the only per-type knowledge the generic path needs: one AlterTargetResolver
// per document type, implemented by that type's mutator on every backend.

// AlterTarget is the document-independent address an ALTER operation names, as
// it was written. At most one of Path and Caption is set.
type AlterTarget struct {
	// Path is a name, or a name and one member: ["btnSave"],
	// ["dgOrders", "Total"], ["layoutContainer", "top"].
	Path []string
	// Caption is a quoted content address: 'Approve order'.
	Caption string
	// Ordinal is @n (1-based), choosing one of several matches; 0 when absent.
	Ordinal int
}

// String renders the target as MDL writes it, for messages.
func (t AlterTarget) String() string {
	var s string
	if t.Caption != "" {
		s = "'" + strings.ReplaceAll(t.Caption, "'", "''") + "'"
	} else if len(t.Path) == 2 {
		s = joinColumnAddress(t.Path[0], t.Path[1])
	} else {
		s = strings.Join(t.Path, ".")
	}
	if t.Ordinal > 0 {
		s += "@" + strconv.Itoa(t.Ordinal)
	}
	return s
}

// AlterTargetMatch is one element an AlterTarget resolved to.
type AlterTargetMatch struct {
	// Kind names what the element is, in the document's own terms: "widget",
	// "column", "region", "activity".
	Kind string
	// Name is how describe names it — the handle to write to address it alone.
	Name string
}

// AlterTargetResolver is the per-document-type half of the generic ALTER.
//
// ResolveAlterTarget returns the one element the target addresses. It never
// guesses: an address that matches nothing is an error, and so is one that
// matches more than one element with no @n to choose — that error lists the
// matches (AlterTargetError), so the author can write the address that picks
// one. A form the document type does not support (a caption on a page, whose
// elements have names) is refused with a message saying which form it takes.
//
// Resolving has no side effects; the operation that follows does the change.
type AlterTargetResolver interface {
	ResolveAlterTarget(t AlterTarget) (AlterTargetMatch, error)
}

// AlterTargetError is a target that did not resolve to exactly one element.
// Matches is empty for a miss and holds every candidate for an ambiguity.
type AlterTargetError struct {
	Target  AlterTarget
	Matches []AlterTargetMatch
	// Detail replaces the generic message when the resolver has a better one
	// (a not-found listing what IS there, say). Optional.
	Detail string
}

func (e *AlterTargetError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	if len(e.Matches) == 0 {
		return fmt.Sprintf("alter target %s not found", e.Target)
	}
	parts := make([]string, len(e.Matches))
	for i, m := range e.Matches {
		parts[i] = fmt.Sprintf("@%d %s %s", i+1, m.Kind, m.Name)
	}
	return fmt.Sprintf("alter target %s is ambiguous: it matches %d elements (%s) — add @n to choose one",
		e.Target, len(e.Matches), strings.Join(parts, ", "))
}

// PickAlterTargetMatch applies a target's @n to the candidates a resolver
// found: exactly one candidate and no @n resolves; @n within range picks that
// one; anything else is an AlterTargetError. Resolvers share it so an ordinal
// means the same thing in every document type.
func PickAlterTargetMatch(t AlterTarget, candidates []AlterTargetMatch) (AlterTargetMatch, error) {
	switch {
	case len(candidates) == 0:
		return AlterTargetMatch{}, &AlterTargetError{Target: t}
	case t.Ordinal > len(candidates):
		return AlterTargetMatch{}, &AlterTargetError{Target: t, Matches: candidates,
			Detail: fmt.Sprintf("alter target %s: there are only %d matches", t, len(candidates))}
	case t.Ordinal > 0:
		return candidates[t.Ordinal-1], nil
	case len(candidates) == 1:
		return candidates[0], nil
	default:
		return AlterTargetMatch{}, &AlterTargetError{Target: t, Matches: candidates}
	}
}

// CheckPageAlterTarget refuses the address forms of the generic ALTER that a
// widget tree has no use for. Every page element that can be a target has a
// Name, and names are unique within the document, so a caption would have to
// guess among widgets that share one, and an @n would pick among matches that
// cannot occur. Both are errors rather than being quietly ignored. Shared by
// every backend's page mutator, so the page address syntax has one definition.
func CheckPageAlterTarget(t AlterTarget) error {
	switch {
	case t.Caption != "":
		return &AlterTargetError{Target: t, Detail: fmt.Sprintf(
			"alter target %s: a page, snippet or layout element is addressed by name "+
				"(`btnSave`, `layoutContainer.top`), and a grid column by what it shows "+
				"(`grid column(Attr)`, `grid column('Caption')`), not by a bare caption — "+
				"`describe` prints every widget's name", t)}
	case t.Ordinal > 0:
		return &AlterTargetError{Target: t, Detail: fmt.Sprintf(
			"alter target %s: @%d chooses among several matches, and a widget name is unique "+
				"within its page — drop the @%d; to choose among grid columns that share an address, "+
				"put it on the column: `grid column(Attr)@n`",
			t, t.Ordinal, t.Ordinal)}
	case len(t.Path) == 0 || len(t.Path) > 2:
		return &AlterTargetError{Target: t, Detail: fmt.Sprintf(
			"alter target %q: want a widget name or `widget.member`", t.String())}
	}
	return nil
}

// WorkflowActivityCandidate is one workflow activity a target was compared
// with: its name, its caption and its storage $Type.
type WorkflowActivityCandidate struct {
	Name        string
	Caption     string
	StorageType string
}

// ResolveWorkflowActivityTarget is the workflow's address rule, shared by
// every backend's workflow mutator so an activity address means the same thing
// on each. candidates are the activities whose name or caption equals the
// target's text, in the depth-first order `describe workflow` prints them (an
// activity, then its outcome flows, then its boundary-event flows).
//
// A name is an activity's identity and a caption a label that may repeat
// another activity's name (a jump's caption defaults to its target's name), so
// without @n the activities NAMED so win over those merely captioned so. With
// @n every match counts, so an existing `ACT_Process@2` keeps addressing what
// it always did.
func ResolveWorkflowActivityTarget(t AlterTarget, candidates []WorkflowActivityCandidate) (AlterTargetMatch, error) {
	if err := CheckWorkflowAlterTarget(t); err != nil {
		return AlterTargetMatch{}, err
	}
	ref := WorkflowTargetText(t)
	pool := candidates
	if t.Ordinal == 0 {
		var named []WorkflowActivityCandidate
		for _, c := range candidates {
			if c.Name == ref {
				named = append(named, c)
			}
		}
		if len(named) > 0 {
			pool = named
		}
	}
	matches := make([]AlterTargetMatch, 0, len(pool))
	for _, c := range pool {
		name := c.Name
		if name == "" {
			name = "'" + strings.ReplaceAll(c.Caption, "'", "''") + "'"
		}
		matches = append(matches, AlterTargetMatch{Kind: WorkflowActivityKind(c.StorageType), Name: name})
	}
	return PickAlterTargetMatch(t, matches)
}

// WorkflowTargetText is the text a workflow target compares with an
// activity's name and caption.
func WorkflowTargetText(t AlterTarget) string {
	if t.Caption != "" {
		return t.Caption
	}
	if len(t.Path) > 0 {
		return t.Path[0]
	}
	return ""
}

// CheckWorkflowAlterTarget refuses the address forms a workflow activity has
// no use for: a dotted member path.
func CheckWorkflowAlterTarget(t AlterTarget) error {
	if t.Caption == "" && len(t.Path) != 1 {
		return &AlterTargetError{Target: t, Detail: fmt.Sprintf(
			"alter target %s: a workflow activity is addressed by its name (`ReviewOrder`) or its "+
				"caption (`'Review the order'`), with @n to choose one of several matches", t)}
	}
	return nil
}

// WorkflowActivityKind names a workflow activity's storage $Type the way an
// author would, for the matches an ambiguous target lists.
func WorkflowActivityKind(storageType string) string {
	switch storageType {
	case "Workflows$SingleUserTaskActivity", "Workflows$UserTaskActivity":
		return "user task"
	case "Workflows$MultiUserTaskActivity":
		return "multi user task"
	case "Workflows$ExclusiveSplitActivity":
		return "decision"
	case "Workflows$ParallelSplitActivity":
		return "parallel split"
	case "Workflows$CallMicroflowTask", "Workflows$CallMicroflowActivity":
		return "call microflow"
	case "Workflows$AIAgentTaskActivity":
		return "call agent microflow"
	case "Workflows$CallWorkflowActivity":
		return "call workflow"
	case "Workflows$JumpToActivity":
		return "jump"
	case "Workflows$WaitForTimerActivity":
		return "wait for timer"
	case "Workflows$WaitForNotificationActivity":
		return "wait for notification"
	case "Workflows$NotificationActivity":
		return "notification"
	case "Workflows$EndWorkflowActivity":
		return "end workflow"
	case "Workflows$StartWorkflowActivity":
		return "start"
	case "Workflows$Annotation":
		return "annotation"
	}
	return "activity"
}
