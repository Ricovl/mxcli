// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/translations"
)

// defaultCaptionRule is check's report of a required caption the script leaves
// without text in the project's default language: mxbuild CE4899 "Empty
// caption. [German, Germany]" (ako/mxcli#944). Which captions are required is
// translations.RequiredCaptionKind — measured, tab page captions only.
const defaultCaptionRule = "MDL-I18N01"

// documentCaption is a required caption with the qualified name of the
// document it sits in.
type documentCaption struct {
	Document string // "Administration.Account_Overview"
	translations.MissingCaption
}

func (d documentCaption) String() string { return d.Document + ": " + d.MissingCaption.String() }

// missingDefaultCaptions lists the project's required captions with no text in
// lang, sorted by document, each named by its qualified document name.
func missingDefaultCaptions(ctx *ExecContext, lang string) ([]documentCaption, error) {
	missing, err := translations.MissingRequiredCaptions(ctx.Backend, lang)
	if err != nil || len(missing) == 0 {
		return nil, err
	}
	h, _ := getHierarchy(ctx)
	out := make([]documentCaption, 0, len(missing))
	for _, m := range missing {
		qn := m.UnitName
		if h != nil {
			if mod := h.GetModuleName(h.FindModuleID(m.ContainerID)); mod != "" {
				qn = mod + "." + m.UnitName
			}
		}
		out = append(out, documentCaption{Document: qn, MissingCaption: m})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Document < out[j].Document })
	return out, nil
}

// maxListedCaptions caps how many captions a note lists inline.
const maxListedCaptions = 10

// writeMissingDefaultCaptionsNote is what `alter settings language
// (DefaultLanguageCode: …)` prints after changing the default: how many required
// captions have no text in it, and where they are. Changing the default is the
// moment they break — every caption written so far is in the old language —
// and nothing else said so: check, lint and exec were all silent until mxbuild
// reported CE4899.
func writeMissingDefaultCaptionsNote(w io.Writer, lang string, missing []documentCaption) {
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(w, "\nNote: %d required caption(s) have no %s text, and mxbuild refuses them "+
		"(CE4899 \"Empty caption\"):\n", len(missing), lang)
	for i, m := range missing {
		if i == maxListedCaptions {
			fmt.Fprintf(w, "  … and %d more (mxcli lint lists them all as QUAL006)\n", len(missing)-maxListedCaptions)
			break
		}
		fmt.Fprintf(w, "  %s\n", m)
	}
	fmt.Fprintf(w, "Give each a %s text — an alter writes the default language, e.g. %s — or re-run the "+
		"script that created the page; `create translations for` does not reach the default language.\n",
		lang, captionFixStatement(missing[0].UnitType, missing[0].Document, missing[0].OwnerName))
}

// captionFixStatement is the statement that gives a tab page a caption in the
// default language — an alter writes the authoring language, which is the
// default. A layout has no alter, so it is pointed at Studio Pro.
func captionFixStatement(unitType, doc, tab string) string {
	kind := "page"
	switch unitType {
	case "Forms$Snippet":
		kind = "snippet"
	case "Forms$Layout":
		return "set the caption of " + tab + " in layout " + doc + " in Studio Pro"
	}
	if tab == "" {
		tab = "<tabPage>"
	}
	return fmt.Sprintf("`alter %s %s { set (Caption: '…') on %s; };`", kind, doc, tab)
}

// (e *Executor) CheckDefaultLanguageCaptions reports MDL-I18N01 for the script.
func (e *Executor) CheckDefaultLanguageCaptions(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	return CheckDefaultLanguageCaptions(e.newExecContext(context.Background()), prog)
}

// CheckDefaultLanguageCaptions foresees CE4899 for a script that changes the
// project's default language: every required caption stored in the project, or
// written by the script before the change, has text only in the languages it
// was written in, and a build in the new default refuses each one.
//
// Only what the script causes is reported. A script that does not change the
// default writes its captions in the default (authoringLanguage), so its own
// captions are fine and the project's existing gaps are lint's (QUAL006). A
// document the script writes AFTER the change is written in the new default
// and is not reported; one it drops is gone.
func CheckDefaultLanguageCaptions(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() {
		return nil
	}
	switchAt, newLang := -1, ""
	for i, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.AlterSettingsStmt); ok && strings.EqualFold(s.Section, "language") {
			if v, ok := s.Properties["DefaultLanguageCode"]; ok {
				switchAt, newLang = i, settingsValueToString(v)
			}
		}
	}
	if switchAt < 0 || newLang == "" {
		return nil
	}
	if ps, err := ctx.Backend.GetProjectSettings(); err == nil && ps != nil && ps.Language != nil &&
		strings.EqualFold(ps.Language.DefaultLanguageCode, newLang) {
		return nil // not a change
	}

	// What the script does to each document, in order: the last word wins.
	rewritten := map[string]bool{}     // stored version is replaced or gone
	written := map[string][]string{}   // created before the switch: its tab pages
	writtenType := map[string]string{} // a created snippet's unit type, for the fix statement
	for i, stmt := range prog.Statements {
		var name string
		var widgets []*ast.WidgetV3
		isCreate := false
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			name, widgets, isCreate = s.Name.String(), s.Widgets, true
		case *ast.CreateSnippetStmtV3:
			name, widgets, isCreate = s.Name.String(), s.Widgets, true
			writtenType[name] = "Forms$Snippet"
		case *ast.DropPageStmt:
			name = s.Name.String()
		case *ast.DropSnippetStmt:
			name = s.Name.String()
		case *ast.AlterPageStmt:
			if i < switchAt {
				continue // an edit before the switch writes the old default
			}
			name = s.PageName.String()
		default:
			continue
		}
		rewritten[name] = true
		delete(written, name)
		if isCreate && i < switchAt {
			written[name] = tabPageNames(widgets)
		}
	}

	var out []linter.Violation
	add := func(unitType, doc, tab, what string) {
		qn := splitQualifiedName(doc)
		out = append(out, linter.Violation{
			RuleID:   defaultCaptionRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("%s: %s has no %s text once this script makes %s the default language — "+
				"mxbuild reports CE4899 \"Empty caption\"", doc, what, newLang, newLang),
			Suggestion: fmt.Sprintf("change the default language before creating the document, or give it a %s "+
				"text after the change: %s", newLang, captionFixStatement(unitType, doc, tab)),
			Location: linter.Location{Module: qn.Module, DocumentName: qn.Name},
		})
	}

	stored, _ := missingDefaultCaptions(ctx, newLang)
	for _, m := range stored {
		if rewritten[m.Document] {
			continue
		}
		add(m.UnitType, m.Document, m.OwnerName, m.MissingCaption.String())
	}
	var names []string
	for n := range written {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, tab := range written[n] {
			add(writtenType[n], n, tab, "tab page caption "+tab+" (written by this script before the change)")
		}
	}
	return out
}

// tabPageNames lists the tab pages in a widget tree, in document order.
func tabPageNames(ws []*ast.WidgetV3) []string {
	var out []string
	for _, w := range ws {
		if w == nil {
			continue
		}
		if strings.EqualFold(w.Type, "tabpage") {
			out = append(out, w.Name)
		}
		out = append(out, tabPageNames(w.Children)...)
	}
	return out
}
