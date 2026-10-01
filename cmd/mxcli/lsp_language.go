// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/migration"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// The language version in the editor (ako/mxcli#714 decision 5).
//
// The parse already reads each document's `mdl <n>;` header, so a refusal under
// mdl 1 is a parse error and a construct kept at its mdl 0 meaning is recorded
// on the program. What `mxcli check` reports from those records — MDL-DEPRnnn
// for a deprecated spelling, MDL-V1-* for a construct whose meaning the header
// would change, MDL-LANG01 for a preview header — the editor reports too, at
// the token or line it is about. Hover on one shows `mxcli help <code>`, and
// the quick fix is the `fmt --upgrade` rewrite where there is one.

// checkLinePrefix is the "line N: " check puts before a message; the
// diagnostic's range already says where.
var checkLinePrefix = regexp.MustCompile(`^line \d+: `)

// languageDiagnostics reports prog's deprecated spellings and language-version
// notes, with check's own messages (executor.ValidateDeprecations,
// executor.ValidateLanguageVersion) — one record at a time, so each message is
// placed at its own record.
func languageDiagnostics(text string, prog *ast.Program) []protocol.Diagnostic {
	lines := strings.Split(text, "\n")
	var diags []protocol.Diagnostic
	for _, d := range prog.Deprecations {
		vs := executor.ValidateDeprecations(&ast.Program{Deprecations: []ast.DeprecatedSpelling{d}})
		diags = append(diags, violationDiagnostics(vs, tokenRange(lines, d.Line, d.Column))...)
	}
	if prog.LanguageVersion.IsPreview() {
		vs := executor.ValidateLanguageVersion(&ast.Program{
			LanguageVersion: prog.LanguageVersion, LanguageHeaderLine: prog.LanguageHeaderLine,
		})
		diags = append(diags, violationDiagnostics(vs, lineRange(lines, prog.LanguageHeaderLine))...)
	}
	for _, n := range prog.LanguageNotes {
		vs := executor.ValidateLanguageVersion(&ast.Program{LanguageNotes: []ast.LanguageNote{n}})
		diags = append(diags, violationDiagnostics(vs, lineRange(lines, n.Line))...)
	}
	return diags
}

func violationDiagnostics(vs []linter.Violation, r protocol.Range) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(vs))
	for _, v := range vs {
		msg := checkLinePrefix.ReplaceAllString(v.Message, "")
		if v.Suggestion != "" {
			msg += " → " + v.Suggestion
		}
		out = append(out, protocol.Diagnostic{
			Range:    r,
			Severity: violationToLSPSeverity(v.Severity),
			Source:   "mdl-check",
			Code:     v.RuleID,
			Message:  msg,
		})
	}
	return out
}

// tokenRange is the word at (line, col): line 1-based, col a 0-based rune
// column as the parser reports it.
func tokenRange(lines []string, line, col int) protocol.Range {
	if line < 1 || line > len(lines) {
		return protocol.Range{}
	}
	runes := []rune(lines[line-1])
	if col < 0 || col > len(runes) {
		return lineRange(lines, line)
	}
	end := col
	for end < len(runes) && (isWordRune(runes[end])) {
		end++
	}
	if end == col && end < len(runes) {
		end++ // punctuation: a `{`, a `:`
	}
	l := uint32(line - 1)
	return protocol.Range{
		Start: protocol.Position{Line: l, Character: utf16Len(runes[:col])},
		End:   protocol.Position{Line: l, Character: utf16Len(runes[:end])},
	}
}

func isWordRune(r rune) bool {
	return r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r >= 0x80
}

// lineRange is line (1-based) without its indentation and trailing space.
func lineRange(lines []string, line int) protocol.Range {
	if line < 1 || line > len(lines) {
		return protocol.Range{}
	}
	runes := []rune(strings.TrimRight(lines[line-1], " \t\r"))
	start := 0
	for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
		start++
	}
	l := uint32(line - 1)
	return protocol.Range{
		Start: protocol.Position{Line: l, Character: utf16Len(runes[:start])},
		End:   protocol.Position{Line: l, Character: utf16Len(runes)},
	}
}

// utf16Len is the length of runes in UTF-16 code units, the unit of an LSP
// position's character offset.
func utf16Len(runes []rune) uint32 {
	n := 0
	for _, r := range runes {
		n += len(utf16.Encode([]rune{r}))
	}
	return uint32(n)
}

// runePosition is the LSP position of the rune offset off in text, the
// coordinates upgrade's edits are in.
func runePosition(text []rune, off int) protocol.Position {
	off = min(max(off, 0), len(text))
	line, start := 0, 0
	for i := 0; i < off; i++ {
		if text[i] == '\n' {
			line++
			start = i + 1
		}
	}
	return protocol.Position{Line: uint32(line), Character: utf16Len(text[start:off])}
}

// diagnosticCode is a diagnostic's code as a string; a code read back from the
// client is whatever JSON decoded it into.
func diagnosticCode(d protocol.Diagnostic) string {
	if d.Code == nil {
		return ""
	}
	return fmt.Sprint(d.Code)
}

func rangeContains(r protocol.Range, p protocol.Position) bool {
	after := p.Line > r.Start.Line || p.Line == r.Start.Line && p.Character >= r.Start.Character
	before := p.Line < r.End.Line || p.Line == r.End.Line && p.Character <= r.End.Character
	return after && before
}

// migrationHover is `mxcli help <code>` for the migration diagnostic under the
// cursor, or nil when there is none.
func (s *mdlServer) migrationHover(docURI uri.URI, text string, pos protocol.Position) *protocol.Hover {
	for _, d := range s.documentDiagnostics(docURI, text) {
		code := diagnosticCode(d)
		if !migration.IsCode(code) || !rangeContains(d.Range, pos) {
			continue
		}
		help, ok := migration.Help(code)
		if !ok {
			continue
		}
		r := d.Range
		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: "```text\n" + strings.TrimRight(help, "\n") + "\n```",
			},
			Range: &r,
		}
	}
	return nil
}

// CodeAction offers the `fmt --upgrade` rewrite for the migration diagnostics
// in the request:
//
//   - a deprecated spelling, or a version-neutral language change: that use's
//     own rewrite (upgrade.FixesOnLines), the preferred fix;
//   - any other language change: `fmt --upgrade --header` on the whole script,
//     since its rewrite keeps the old meaning only once the header is there.
//     It is offered only when the upgrade succeeds; a construct with no
//     rewrite blocks it, as it blocks the command.
//
// With more than one deprecated use in the script, the whole-file
// `fmt --upgrade` is offered too.
func (s *mdlServer) CodeAction(ctx context.Context, params *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	docURI := uri.URI(params.TextDocument.URI)
	if testrunner.IsTestFile(docURI.Filename()) {
		return nil, nil // diagnosed as the MDL rendered from it, not as written
	}
	s.mu.Lock()
	text := s.docs[docURI]
	s.mu.Unlock()
	if text == "" {
		return nil, nil
	}

	var migrationDiags []protocol.Diagnostic
	for _, d := range params.Context.Diagnostics {
		if migration.IsCode(diagnosticCode(d)) {
			migrationDiags = append(migrationDiags, d)
		}
	}
	if len(migrationDiags) == 0 {
		return nil, nil
	}

	runes := []rune(text)
	whole := func(newText string) *protocol.WorkspaceEdit {
		return &protocol.WorkspaceEdit{Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			params.TextDocument.URI: {{
				Range:   protocol.Range{End: runePosition(runes, len(runes))},
				NewText: newText,
			}},
		}}
	}

	var acts []protocol.CodeAction
	var gated []protocol.Diagnostic
	deprecated := false
	for _, d := range migrationDiags {
		code := diagnosticCode(d)
		if _, ok := deprecation.Lookup(code); ok {
			deprecated = true
		}
		if f, ok := fixFor(text, d); ok {
			edits := make([]protocol.TextEdit, 0, len(f.Edits))
			for _, e := range f.Edits {
				edits = append(edits, protocol.TextEdit{
					Range:   protocol.Range{Start: runePosition(runes, e.Start), End: runePosition(runes, e.Stop)},
					NewText: e.Text,
				})
			}
			acts = append(acts, protocol.CodeAction{
				Title:       fixTitle(code),
				Kind:        protocol.QuickFix,
				Diagnostics: []protocol.Diagnostic{d},
				IsPreferred: true,
				Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					params.TextDocument.URI: edits,
				}},
			})
			continue
		}
		if strings.HasPrefix(code, "MDL-V1-") {
			gated = append(gated, d)
		}
	}

	if len(gated) > 0 {
		if res, err := upgrade.Upgrade(text, upgrade.Options{AddHeader: true}); err == nil && res.HeaderAdded {
			acts = append(acts, protocol.CodeAction{
				Title:       "Upgrade the script to mdl 1 (fmt --upgrade --header)",
				Kind:        protocol.QuickFix,
				Diagnostics: gated,
				Edit:        whole(res.Source),
			})
		}
	}
	if deprecated {
		if res, err := upgrade.Upgrade(text, upgrade.Options{}); err == nil && rewrites(res) > 1 {
			acts = append(acts, protocol.CodeAction{
				Title: fmt.Sprintf("Rewrite all %d deprecated spellings in the script (fmt --upgrade)", rewrites(res)),
				Kind:  protocol.QuickFix,
				Edit:  whole(res.Source),
			})
		}
	}
	return acts, nil
}

// fixFor is the rewrite of the one use d reports: the fix on d's line with d's
// code, at d's column for a deprecated spelling.
func fixFor(text string, d protocol.Diagnostic) (upgrade.Fix, bool) {
	line := int(d.Range.Start.Line) + 1
	fixes, err := upgrade.FixesOnLines(text, line, line)
	if err != nil {
		return upgrade.Fix{}, false
	}
	lines := strings.Split(text, "\n")
	code := diagnosticCode(d)
	for _, f := range fixes {
		if f.Code != code {
			continue
		}
		if f.Column < 0 || tokenRange(lines, f.Line, f.Column).Start == d.Range.Start {
			return f, true
		}
	}
	return upgrade.Fix{}, false
}

// fixTitle names a use's quick fix by what it writes.
func fixTitle(code string) string {
	if e, ok := deprecation.Lookup(code); ok {
		return fmt.Sprintf("Write `%s` (%s)", e.Canonical, code)
	}
	return fmt.Sprintf("Rewrite for mdl 1 (%s)", code)
}

// rewrites counts the uses an upgrade rewrote.
func rewrites(res upgrade.Result) int {
	n := 0
	for _, c := range res.Rewritten {
		n += c
	}
	for _, c := range res.GatedRewritten {
		n += c
	}
	return n
}
