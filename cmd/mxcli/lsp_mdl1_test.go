// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/migration"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// The language server for the mdl 1 freeze (ako/mxcli#714 decision 5): the
// editor reports what `mxcli check` reports for the document's own language
// version, explains a migration code where it is reported, and offers the
// `fmt --upgrade` rewrite as the quick fix.

const lspTestURI = uri.URI("file:///work/script.mdl")

// replaceEntity uses MDL-DEPR001 (`create or replace` for `create or modify`)
// at line 1, column 10 ("create or " is ten characters).
const replaceEntity = "create or replace entity M.E (Name: String(20));\n"

// limitOneFlow is MDL-V1-LIMIT1: `limit 1` is an object under mdl 0 and a list
// of one under mdl 1, so a headerless script warns and a script with the header
// does not.
const limitOneFlow = "create microflow M.F ()\nbegin\n  retrieve $x from M.E limit 1;\nend;\n"

func diagsWithCode(diags []protocol.Diagnostic, code string) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	for _, d := range diags {
		if fmt.Sprint(d.Code) == code {
			out = append(out, d)
		}
	}
	return out
}

func TestLSPReportsDeprecationAtItsToken(t *testing.T) {
	s := newMDLServer(nil)
	diags := s.documentDiagnostics(lspTestURI, replaceEntity)
	got := diagsWithCode(diags, "MDL-DEPR001")
	if len(got) != 1 {
		t.Fatalf("want one MDL-DEPR001 diagnostic, got %d in %+v", len(got), diags)
	}
	d := got[0]
	want := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 10},
		End:   protocol.Position{Line: 0, Character: 17},
	}
	if d.Range != want {
		t.Errorf("range = %+v, want %+v (the `replace` token)", d.Range, want)
	}
	if d.Severity != protocol.DiagnosticSeverityWarning {
		t.Errorf("severity = %v, want warning", d.Severity)
	}
	if strings.HasPrefix(d.Message, "line ") {
		t.Errorf("message keeps check's `line N:` prefix, which the range already says: %q", d.Message)
	}
}

// The header chooses the language version: a construct whose meaning differs
// under mdl 1 warns in a headerless document and not under `mdl 1;`.
func TestLSPDiagnosticsFollowTheHeader(t *testing.T) {
	s := newMDLServer(nil)
	headerless := diagsWithCode(s.documentDiagnostics(lspTestURI, limitOneFlow), "MDL-V1-LIMIT1")
	if len(headerless) != 1 {
		t.Fatalf("headerless: want one MDL-V1-LIMIT1 diagnostic, got %+v", headerless)
	}
	if headerless[0].Range.Start.Line != 2 {
		t.Errorf("MDL-V1-LIMIT1 on line %d, want 2 (the retrieve)", headerless[0].Range.Start.Line)
	}
	if got := diagsWithCode(s.documentDiagnostics(lspTestURI, "mdl 1;\n"+limitOneFlow), "MDL-V1-LIMIT1"); len(got) != 0 {
		t.Errorf("under `mdl 1;` limit 1 means what it says and must not warn, got %+v", got)
	}
}

// A use mdl 1 refuses is an error under the header and a warning without it.
func TestLSPRefusalFollowsTheHeader(t *testing.T) {
	s := newMDLServer(nil)
	src := "create microflow M.F ()\nbegin\n  declare $x String = '';\n  $x = 'a';\nend;\n"
	if got := diagsWithCode(s.documentDiagnostics(lspTestURI, src), "MDL-V1-SET"); len(got) != 1 {
		t.Errorf("headerless `$x = 'a'` should warn MDL-V1-SET once, got %+v", s.documentDiagnostics(lspTestURI, src))
	}
	var errs int
	for _, d := range s.documentDiagnostics(lspTestURI, "mdl 1;\n"+src) {
		if d.Severity == protocol.DiagnosticSeverityError {
			errs++
		}
	}
	if errs == 0 {
		t.Error("under `mdl 1;` an assignment without `set` must be an error")
	}
}

func hoverText(t *testing.T, s *mdlServer, text string, line, char uint32) string {
	t.Helper()
	s.docs[lspTestURI] = text
	h, err := s.Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(lspTestURI)},
			Position:     protocol.Position{Line: line, Character: char},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		return ""
	}
	return h.Contents.Value
}

func TestLSPHoverOnMigrationDiagnosticShowsHelp(t *testing.T) {
	s := newMDLServer(nil)
	want, ok := migration.Help("MDL-DEPR001")
	if !ok {
		t.Fatal("migration.Help has no MDL-DEPR001")
	}
	got := hoverText(t, s, replaceEntity, 0, 12) // inside `replace`
	if !strings.Contains(got, strings.TrimSpace(want)) {
		t.Errorf("hover on `replace` should show `mxcli help MDL-DEPR001`:\n%s\ngot:\n%s", want, got)
	}
	if got := hoverText(t, s, replaceEntity, 0, 2); strings.Contains(got, "MDL-DEPR001") {
		t.Errorf("hover on `create`, outside the diagnostic, shows its help: %q", got)
	}

	wantV1, _ := migration.Help("MDL-V1-LIMIT1")
	if got := hoverText(t, s, limitOneFlow, 2, 25); !strings.Contains(got, strings.TrimSpace(wantV1)) {
		t.Errorf("hover on the retrieve should show `mxcli help MDL-V1-LIMIT1`, got:\n%s", got)
	}
}

func codeActions(t *testing.T, s *mdlServer, text string, diags []protocol.Diagnostic) []protocol.CodeAction {
	t.Helper()
	s.docs[lspTestURI] = text
	acts, err := s.CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(lspTestURI)},
		Range:        diags[0].Range,
		Context:      protocol.CodeActionContext{Diagnostics: diags},
	})
	if err != nil {
		t.Fatal(err)
	}
	return acts
}

// applyEdits applies LSP text edits (UTF-16 positions, non-overlapping, in
// document order) to text.
func applyEdits(t *testing.T, text string, edits []protocol.TextEdit) string {
	t.Helper()
	offset := func(p protocol.Position) int {
		lines := strings.SplitAfter(text, "\n")
		off := 0
		for i := 0; i < int(p.Line) && i < len(lines); i++ {
			off += len(lines[i])
		}
		if int(p.Line) >= len(lines) {
			return len(text)
		}
		u16 := 0
		for i, r := range lines[p.Line] {
			if u16 >= int(p.Character) {
				return off + i
			}
			u16++
			if r > 0xFFFF {
				u16++
			}
		}
		return off + len(lines[p.Line])
	}
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		a, b := offset(e.Range.Start), offset(e.Range.End)
		text = text[:a] + e.NewText + text[b:]
	}
	return text
}

func actionEdits(t *testing.T, a protocol.CodeAction) []protocol.TextEdit {
	t.Helper()
	if a.Edit == nil {
		t.Fatalf("action %q has no edit", a.Title)
	}
	return a.Edit.Changes[protocol.DocumentURI(lspTestURI)]
}

func TestLSPQuickFixRewritesDeprecatedSpelling(t *testing.T) {
	s := newMDLServer(nil)
	diags := diagsWithCode(s.documentDiagnostics(lspTestURI, replaceEntity), "MDL-DEPR001")
	if len(diags) != 1 {
		t.Fatalf("want one MDL-DEPR001 diagnostic, got %+v", diags)
	}
	acts := codeActions(t, s, replaceEntity, diags)
	var fix *protocol.CodeAction
	for i := range acts {
		if acts[i].Kind == protocol.QuickFix && acts[i].IsPreferred {
			fix = &acts[i]
		}
	}
	if fix == nil {
		t.Fatalf("no preferred quick fix among %+v", acts)
	}
	got := applyEdits(t, replaceEntity, actionEdits(t, *fix))
	if want := "create or modify entity M.E (Name: String(20));\n"; got != want {
		t.Errorf("quick fix gave\n%q\nwant\n%q", got, want)
	}
	if len(fix.Diagnostics) != 1 || fmt.Sprint(fix.Diagnostics[0].Code) != "MDL-DEPR001" {
		t.Errorf("the fix should name the diagnostic it resolves, got %+v", fix.Diagnostics)
	}
}

// A use's quick fix is that use's rewrite and nothing else: a second
// deprecated use elsewhere in the document is left alone.
func TestLSPQuickFixTouchesOnlyItsUse(t *testing.T) {
	s := newMDLServer(nil)
	src := replaceEntity + "create or replace entity M.F (Name: String(20));\n"
	diags := diagsWithCode(s.documentDiagnostics(lspTestURI, src), "MDL-DEPR001")
	if len(diags) != 2 {
		t.Fatalf("want two MDL-DEPR001 diagnostics, got %+v", diags)
	}
	acts := codeActions(t, s, src, diags[1:])
	for _, a := range acts {
		if !a.IsPreferred {
			continue
		}
		got := applyEdits(t, src, actionEdits(t, a))
		if want := replaceEntity + "create or modify entity M.F (Name: String(20));\n"; got != want {
			t.Errorf("quick fix gave\n%s\nwant\n%s", got, want)
		}
		return
	}
	t.Fatalf("no preferred quick fix among %+v", acts)
}

// A header-gated construct has no rewrite on its own: under mdl 0 its rewrite
// can mean something else. Its quick fix is the whole `fmt --upgrade --header`.
func TestLSPQuickFixForLanguageChangeUpgradesTheScript(t *testing.T) {
	s := newMDLServer(nil)
	diags := diagsWithCode(s.documentDiagnostics(lspTestURI, limitOneFlow), "MDL-V1-LIMIT1")
	if len(diags) != 1 {
		t.Fatalf("want one MDL-V1-LIMIT1 diagnostic, got %+v", diags)
	}
	acts := codeActions(t, s, limitOneFlow, diags)
	if len(acts) == 0 {
		t.Fatal("no code action for MDL-V1-LIMIT1")
	}
	want, err := upgrade.Upgrade(limitOneFlow, upgrade.Options{AddHeader: true})
	if err != nil {
		t.Fatal(err)
	}
	got := applyEdits(t, limitOneFlow, actionEdits(t, acts[0]))
	if got != want.Source {
		t.Errorf("quick fix gave\n%s\nwant fmt --upgrade --header's\n%s", got, want.Source)
	}
	if !strings.HasPrefix(got, "mdl 1;") || strings.Contains(got, "limit 1") {
		t.Errorf("the upgraded script should carry the header and `first`:\n%s", got)
	}
}

// A use with no mechanical rewrite gets no quick fix: it is reported, never
// guessed at, as `fmt --upgrade` does.
func TestLSPNoQuickFixWithoutARewrite(t *testing.T) {
	s := newMDLServer(nil)
	src := "create or replace view entity M.V (Name: String) as select e.Name as Name from M.E as e;\n"
	diags := diagsWithCode(s.documentDiagnostics(lspTestURI, src), "MDL-V1-REPLACE01")
	if len(diags) != 1 {
		t.Fatalf("fixture should warn MDL-V1-REPLACE01 once: %+v", s.documentDiagnostics(lspTestURI, src))
	}
	if acts := codeActions(t, s, src, diags); len(acts) != 0 {
		t.Errorf("MDL-V1-REPLACE01 has no rewrite, but got actions %+v", acts)
	}
}

// completionLabels lists what completion offers after linePrefix.
func completionLabels(s *mdlServer, linePrefix string) map[string]protocol.CompletionItem {
	out := map[string]protocol.CompletionItem{}
	for _, it := range s.mdlCompletionItems(strings.ToUpper(linePrefix)) {
		out[strings.ToUpper(it.Label)] = it
	}
	return out
}

// Completion offers mdl 1 spellings only: a keyword the grammar accepts only
// as a deprecated alias is not offered, and the catalog listings follow `list`.
func TestCompletionOffersNoDeprecatedSpelling(t *testing.T) {
	s := newMDLServer(nil)
	general := completionLabels(s, "")
	for _, alias := range []string{"SHOW_PAGE", "CLOSE_PAGE", "CREATE_OBJECT", "DELETE_OBJECT", "OPEN_LINK", "DELETE_BEHAVIOR", "DEFINE"} {
		if _, ok := general[alias]; ok {
			t.Errorf("completion offers %s, a deprecated alias", alias)
		}
	}
	for _, canonical := range []string{"SHOW", "LIST", "SAVE CHANGES", "ERROR MESSAGE", "CREATE"} {
		if _, ok := general[canonical]; !ok {
			t.Errorf("completion lost the canonical keyword %s", canonical)
		}
	}
	if _, ok := completionLabels(s, "list ")["ENTITIES"]; !ok {
		t.Error("`list ` should offer ENTITIES")
	}
	if _, ok := completionLabels(s, "show ")["ENTITIES"]; ok {
		t.Error("`show ` offers ENTITIES, which is `show entities`, the deprecated MDL-DEPR002 spelling")
	}
}

var (
	snippetChoice = regexp.MustCompile(`\$\{\d+\|([^,|]*)[^}]*\}`)
	snippetHole   = regexp.MustCompile(`\$\{\d+:([^{}]*)\}`)
	snippetStop   = regexp.MustCompile(`\$\{\d+\}|\$\d+`)
)

// expandSnippet fills a snippet with its defaults, as accepting it unedited does.
func expandSnippet(body string) string {
	body = strings.ReplaceAll(body, "$$", "\x00")
	body = snippetChoice.ReplaceAllString(body, "$1")
	for snippetHole.MatchString(body) {
		body = snippetHole.ReplaceAllString(body, "$1")
	}
	body = snippetStop.ReplaceAllString(body, "")
	return strings.ReplaceAll(body, "\x00", "$")
}

// Every snippet completion inserts canonical mdl 1: it parses under the header
// and records no deprecated spelling.
func TestCompletionSnippetsAreMdl1(t *testing.T) {
	check := func(label, src string) {
		t.Helper()
		prog, errs := visitor.Build("mdl 1;\n" + src)
		if len(errs) > 0 {
			t.Errorf("snippet %q does not parse as mdl 1: %v\n%s", label, errs[0], src)
			return
		}
		if len(prog.Deprecations) > 0 {
			t.Errorf("snippet %q uses a deprecated spelling (%s):\n%s", label, prog.Deprecations[0].Code, src)
		}
	}
	for _, it := range mdlCreateSnippets {
		check(it.Label, expandSnippet(it.InsertText))
	}
	for _, it := range mdlStatementSnippets {
		body := expandSnippet(it.InsertText)
		switch {
		case strings.HasPrefix(body, "DATAVIEW"):
			check(it.Label, "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n"+body+"\n};")
		case strings.HasPrefix(body, "INDEX"):
			check(it.Label, "create entity M.E (AttributeName: String) "+body)
		default:
			check(it.Label, "create microflow M.F ()\nbegin\n"+body+"\nend;")
		}
	}
}

// What completion offers after `list` and `show` continues into mdl 1: each
// item, completed as a user would, parses under the header and records no
// deprecated spelling.
func TestCompletionListAndShowContinueIntoMdl1(t *testing.T) {
	rest := map[string]string{
		"CALLERS": " of M.F", "CALLEES": " of M.F", "REFERENCES": " to M.F",
		"IMPACT": " of M.F", "CATALOG": " tables",
	}
	check := func(src string) {
		t.Helper()
		prog, errs := visitor.Build("mdl 1;\n" + src)
		if len(errs) > 0 {
			t.Errorf("%q does not parse as mdl 1: %v", src, errs[0])
			return
		}
		if len(prog.Deprecations) > 0 {
			t.Errorf("%q uses a deprecated spelling (%s)", src, prog.Deprecations[0].Code)
		}
	}
	for _, it := range mdlListContextKeywords {
		check("list " + strings.ToLower(it.Label) + rest[it.Label] + ";")
	}
	show := map[string]string{"PAGE": " M.P", "MESSAGE": " 'hi'", "HOME PAGE": ""}
	for _, it := range mdlShowContextKeywords {
		check("create microflow M.F ()\nbegin\n  show " + strings.ToLower(it.Label) + show[it.Label] + ";\nend;")
	}
}

// Positions are UTF-16 code units, the LSP default, while the parser and the
// upgrade count runes: a character outside the BMP before the use moves the
// diagnostic and its fix by two units, not one.
func TestLSPMigrationPositionsAreUTF16(t *testing.T) {
	s := newMDLServer(nil)
	src := "/* 😀 */ create or replace entity M.E (Name: String(20));\n"
	diags := diagsWithCode(s.documentDiagnostics(lspTestURI, src), "MDL-DEPR001")
	if len(diags) != 1 {
		t.Fatalf("want one MDL-DEPR001 diagnostic, got %+v", diags)
	}
	// "/* 😀 */ create or " is 18 runes, 19 UTF-16 units.
	if got := diags[0].Range.Start.Character; got != 19 {
		t.Errorf("diagnostic starts at character %d, want 19", got)
	}
	for _, a := range codeActions(t, s, src, diags) {
		if a.IsPreferred {
			got := applyEdits(t, src, actionEdits(t, a))
			if want := "/* 😀 */ create or modify entity M.E (Name: String(20));\n"; got != want {
				t.Errorf("quick fix gave %q, want %q", got, want)
			}
			return
		}
	}
	t.Fatal("no preferred quick fix")
}

// The VS Code extension's snippets start a script with `mdl 1;` and are
// canonical mdl 1 (decision 5).
func TestVSCodeSnippetsStartWithTheHeader(t *testing.T) {
	raw, err := os.ReadFile("../../vscode-mdl/snippets/mdl.json")
	if err != nil {
		t.Fatal(err)
	}
	var snippets map[string]struct {
		Body []string `json:"body"`
	}
	if err := json.Unmarshal(raw, &snippets); err != nil {
		t.Fatal(err)
	}
	if len(snippets) == 0 {
		t.Fatal("no snippets")
	}
	for name, sn := range snippets {
		src := expandSnippet(strings.Join(sn.Body, "\n"))
		if !strings.HasPrefix(src, "mdl 1;") {
			t.Errorf("snippet %q does not start with `mdl 1;`:\n%s", name, src)
		}
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Errorf("snippet %q does not parse: %v\n%s", name, errs[0], src)
			continue
		}
		if prog.LanguageVersion != langver.V1 || len(prog.Deprecations) > 0 || len(prog.LanguageNotes) > 0 {
			t.Errorf("snippet %q is not canonical mdl 1 (version %v, %d deprecations, %d notes)",
				name, prog.LanguageVersion, len(prog.Deprecations), len(prog.LanguageNotes))
		}
	}
}

// The extension highlights the header on the lines langver reads as one.
func TestVSCodeGrammarHighlightsTheHeader(t *testing.T) {
	raw, err := os.ReadFile("../../vscode-mdl/syntaxes/mdl.tmLanguage.json")
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Patterns []struct {
			Include string `json:"include"`
		} `json:"patterns"`
		Repository map[string]struct {
			Match string `json:"match"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Patterns) == 0 || g.Patterns[0].Include != "#language-header" {
		t.Error("#language-header must be the first pattern, ahead of the keywords that would claim `mdl`")
	}
	re, err := regexp.Compile(g.Repository["language-header"].Match)
	if err != nil {
		t.Fatalf("language-header match: %v", err)
	}
	for _, line := range []string{"mdl 1;", "MDL 1;", "  mdl 1 ;", "mdl 1; -- beta", "mdl 0;"} {
		if !langver.IsHeaderLine(line) || !re.MatchString(line) {
			t.Errorf("%q: langver header %v, highlighted %v", line, langver.IsHeaderLine(line), re.MatchString(line))
		}
	}
	for _, line := range []string{"create mdl 1;", "-- mdl 1;", "mdl one;"} {
		if re.MatchString(line) {
			t.Errorf("%q is highlighted as a header", line)
		}
	}
}
