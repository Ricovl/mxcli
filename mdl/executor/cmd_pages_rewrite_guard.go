// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// A rewrite that would delete widgets MDL cannot author (mendixlabs/mxcli#1622).
//
// `describe page` prints a widget MDL has no way to write — a classic data
// grid, a reference selector, a drop-down, a reference set selector — as a
// `-- NOT re-executable` comment, so executing the description rebuilt the page
// without it and reported "Replaced page", with mx check clean on both sides.
// The statement cannot have asked for that: it has no words for the widget.
// So the rewrite is refused (guard-don't-drop, ADR-0005), naming each widget,
// and ALTER PAGE — which edits the stored document and leaves these widgets in
// place — is the way to change such a page; dropping them with it first makes
// the rewrite go through.
//
// The widgets are learnt from describe itself (describeDroppedWidget), not
// from a list of authorable types kept here: what a rewrite loses is exactly
// what the description cannot say, and a second list would drift from the
// describer's switch. A widget the statement does declare by name is the
// author writing it, so it does not count — that is what keeps a widget the
// builder can write but describe cannot print from refusing its own script.

// unauthorableStoredWidgets describes the stored document through describe,
// output discarded, and returns the widgets the statement does not declare
// that the description could only print as a comment.
func unauthorableStoredWidgets(ctx *ExecContext, declared map[string]bool, describe func(c *ExecContext) error) []unauthorableWidget {
	var found []unauthorableWidget
	c := *ctx
	c.Output = io.Discard
	c.unauthorableSink = &found
	if err := describe(&c); err != nil {
		// A document describe cannot read is not this guard's business; the
		// rewrite reports its own errors.
		return nil
	}
	var out []unauthorableWidget
	for _, w := range found {
		if w.Name != "" && declared[w.Name] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// checkNoUnauthorableWidgetsDropped refuses a rewrite of the stored page,
// snippet or layout unitID that would delete widgets MDL cannot author. kind
// is "page", "snippet" or "layout"; widgets are the statement's.
func checkNoUnauthorableWidgetsDropped(ctx *ExecContext, kind string, name ast.QualifiedName, unitID model.ID, widgets []*ast.WidgetV3) error {
	if ctx == nil || ctx.Backend == nil || unitID == "" {
		return nil
	}
	declared := map[string]bool{}
	collectWidgetNames(widgets, declared)
	lost := unauthorableStoredWidgets(ctx, declared, func(c *ExecContext) error {
		switch kind {
		case "snippet":
			return describeSnippet(c, name)
		case "layout":
			return describeLayout(c, name)
		}
		c.describeID = unitID
		return describePage(c, name)
	})
	if len(lost) == 0 {
		return nil
	}
	return mdlerrors.NewUnsupported(unauthorableRewriteMessage(kind, name, lost))
}

// unauthorableRewriteMessage explains the refusal: every widget, and the two
// ways forward.
func unauthorableRewriteMessage(kind string, name ast.QualifiedName, lost []unauthorableWidget) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s holds %d widget(s) MDL cannot author, and rewriting it from this statement would delete them:\n",
		kind, name, len(lost))
	var named []string
	for _, w := range lost {
		if w.Name == "" {
			fmt.Fprintf(&b, "  - an unnamed %s\n", w.Type)
			continue
		}
		fmt.Fprintf(&b, "  - %s (%s)\n", w.Name, w.Type)
		named = append(named, w.Name)
	}
	fmt.Fprintf(&b, "  `describe %s` prints them as `-- NOT re-executable` comments, which is why executing its output drops them.\n", kind)
	fmt.Fprintf(&b, "  To change the %s and keep them, use ALTER, which edits only what it names:\n", kind)
	fmt.Fprintf(&b, "    alter %s %s { set (Caption: '…') on <widget>; insert after <widget> { … }; replace <widget> with { … }; };\n", kind, name)
	if len(named) > 0 {
		fmt.Fprintf(&b, "  To rebuild it without them, remove them on purpose first:\n")
		fmt.Fprintf(&b, "    alter %s %s { drop %s; };", kind, name, strings.Join(named, ", "))
	} else {
		fmt.Fprintf(&b, "  Unnamed widgets cannot be dropped by name: give them a name in Studio Pro, or drop the container that holds them.")
	}
	return b.String()
}

// validateUnauthorableRewrites is the check -p half: every `create or modify`
// or `create or replace` of a stored page or snippet that exec would refuse
// for the widgets it deletes. It walks the script in order and skips a
// document an earlier statement already creates, alters or drops, since what
// is stored by then is not what the project holds now.
func validateUnauthorableRewrites(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}
	touched := map[string]bool{}
	key := func(kind string, qn ast.QualifiedName) string { return kind + " " + qn.String() }
	var errs []error
	for i, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			k := key("page", s.Name)
			if (s.IsModify || s.IsReplace) && !touched[k] {
				if id := storedRewriteTarget(ctx, "page", s.Name); id != "" {
					if err := checkNoUnauthorableWidgetsDropped(ctx, "page", s.Name, id, allPageWidgets(s)); err != nil {
						errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
					}
				}
			}
			touched[k] = true
		case *ast.CreateSnippetStmtV3:
			k := key("snippet", s.Name)
			if (s.IsModify || s.IsReplace) && !touched[k] {
				if id := storedRewriteTarget(ctx, "snippet", s.Name); id != "" {
					if err := checkNoUnauthorableWidgetsDropped(ctx, "snippet", s.Name, id, s.Widgets); err != nil {
						errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
					}
				}
			}
			touched[k] = true
		case *ast.CreateLayoutStmt:
			k := key("layout", s.Name)
			if (s.IsModify || s.IsReplace) && !touched[k] {
				if id := storedRewriteTarget(ctx, "layout", s.Name); id != "" {
					if err := checkNoUnauthorableWidgetsDropped(ctx, "layout", s.Name, id, s.AllWidgets()); err != nil {
						errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
					}
				}
			}
			touched[k] = true
		case *ast.AlterPageStmt:
			touched[key(strings.ToLower(s.ContainerType), s.PageName)] = true
		case *ast.DropPageStmt:
			touched[key("page", s.Name)] = true
		case *ast.DropSnippetStmt:
			touched[key("snippet", s.Name)] = true
		}
	}
	return errs
}

// storedRewriteTarget is the stored document a rewrite of qn writes over, as
// execCreatePageV3 / execCreateSnippetV3 / execCreateLayout choose it: the
// first live document of the name, else the first excluded one; for a snippet
// or a layout only when the name has a single document, which is all describe
// can tell apart. "" when none.
func storedRewriteTarget(ctx *ExecContext, kind string, qn ast.QualifiedName) model.ID {
	h, err := getHierarchy(ctx)
	if err != nil {
		return ""
	}
	inModule := func(container model.ID) bool {
		return h.GetModuleName(h.FindModuleID(container)) == qn.Module
	}
	var live, excluded []model.ID
	switch kind {
	case "page":
		ps, err := ctx.Backend.ListPages()
		if err != nil {
			return ""
		}
		for _, p := range ps {
			if p.Name != qn.Name || !inModule(p.ContainerID) {
				continue
			}
			if p.Excluded {
				excluded = append(excluded, p.ID)
			} else {
				live = append(live, p.ID)
			}
		}
	case "layout":
		ls, err := ctx.Backend.ListLayouts()
		if err != nil {
			return ""
		}
		for _, l := range ls {
			if l.Name == qn.Name && inModule(l.ContainerID) {
				live = append(live, l.ID)
			}
		}
		if len(live) != 1 {
			return ""
		}
	case "snippet":
		ss, err := ctx.Backend.ListSnippets()
		if err != nil {
			return ""
		}
		for _, sn := range ss {
			if sn.Name != qn.Name || !inModule(sn.ContainerID) {
				continue
			}
			if sn.Excluded {
				excluded = append(excluded, sn.ID)
			} else {
				live = append(live, sn.ID)
			}
		}
		if len(live)+len(excluded) != 1 {
			return ""
		}
	}
	if len(live) > 0 {
		return live[0]
	}
	if len(excluded) > 0 {
		return excluded[0]
	}
	return ""
}
