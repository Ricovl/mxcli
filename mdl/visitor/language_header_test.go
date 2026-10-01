// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// The `mdl <n>;` header declares the language version a script is written in
// (ADR-0011 decision 2, ako/mxcli#710). A script without one is mdl 0.

func TestLanguageHeader_HeaderlessScriptIsMdl0(t *testing.T) {
	prog, errs := Build("show entities;")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if prog.LanguageVersion != langver.V0 || prog.LanguageHeaderLine != 0 {
		t.Fatalf("headerless script: got version %d, header line %d; want mdl 0 with no header",
			prog.LanguageVersion, prog.LanguageHeaderLine)
	}
}

func TestLanguageHeader_Parses(t *testing.T) {
	for _, src := range []string{
		"mdl 1;\nshow entities;",
		"MDL 1;\nshow entities;",
		"-- a leading comment is not a statement\nmdl 1;\nshow entities;",
		"mdl 0;\nshow entities;", // only ever implicit, but harmless when written
	} {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Errorf("%q: unexpected errors: %v", src, errs)
			continue
		}
		if len(prog.Statements) != 1 {
			t.Errorf("%q: got %d statements, want 1 (the header is not a statement)", src, len(prog.Statements))
		}
		want := langver.V1
		if strings.HasPrefix(src, "mdl 0") {
			want = langver.V0
		}
		if prog.LanguageVersion != want {
			t.Errorf("%q: got version %d, want %d", src, prog.LanguageVersion, want)
		}
		if prog.LanguageHeaderLine == 0 {
			t.Errorf("%q: header line not recorded", src)
		}
	}
}

func TestLanguageHeader_HeaderOnlyScript(t *testing.T) {
	prog, errs := Build("mdl 1;")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if prog.LanguageVersion != langver.V1 || len(prog.Statements) != 0 {
		t.Fatalf("got version %d and %d statements", prog.LanguageVersion, len(prog.Statements))
	}
}

func TestLanguageHeader_UnknownVersionIsRefused(t *testing.T) {
	_, errs := Build("mdl 2;\nshow entities;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "unknown MDL language version 2") {
		t.Fatalf("want an unknown-version error, got %v", errs)
	}
}

// A version too large for an int is still a whole number: it is unknown, not
// malformed, and the message must say so.
func TestLanguageHeader_OverflowingVersionIsUnknown(t *testing.T) {
	_, errs := Build("mdl 99999999999999999999;\nshow entities;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "unknown MDL language version 99999999999999999999") {
		t.Fatalf("want an unknown-version error, got %v", errs)
	}
}

func TestLanguageHeader_NonIntegerVersionIsRefused(t *testing.T) {
	_, errs := Build("mdl 1.5;\nshow entities;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "whole number") {
		t.Fatalf("want a whole-number error, got %v", errs)
	}
}

func TestLanguageHeader_OnlyAsFirstStatement(t *testing.T) {
	_, errs := Build("show entities;\nmdl 1;")
	if len(errs) == 0 {
		t.Fatal("a header after a statement parsed")
	}
	if !strings.Contains(errs[0].Error(), "must be the first statement") {
		t.Fatalf("error does not say where the header belongs: %v", errs[0])
	}
}

func TestLanguageHeader_OnlyTheWordMdl(t *testing.T) {
	_, errs := Build("mdk 1;\nshow entities;")
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "`mdl <n>;`") {
		t.Fatalf("a misspelled header was accepted or not explained: %v", errs)
	}
}

// The header must not take the word away from names, including positions that
// accept only an IDENTIFIER.
func TestLanguageHeader_MdlIsStillAName(t *testing.T) {
	prog, errs := Build("create entity mdl.Mdl ( mdl: String(20) );\n" +
		"describe contract entity MyModule.Api.Product format mdl;")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	e, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok || e.Name.Module != "mdl" || e.Attributes[0].Name != "mdl" {
		t.Fatalf("got %#v", prog.Statements[0])
	}
	if prog.LanguageHeaderLine != 0 {
		t.Fatal("a name was read as a header")
	}
}

// A gated construct: one whose meaning differs between mdl 0 and mdl 1. No
// real construct is gated yet (the §5 changes land in their own issues), so a
// test-only change is attached to `show`, through the same Builder.gate the
// real ones will call.
var testGatedChange = langver.Change{
	Code:  "MDL-TEST-GATE",
	Since: langver.V1,
	Old:   "`show` means the old thing",
	New:   "the new thing",
}

type gateProbe struct {
	*Builder
	newMeaning []bool
}

func (p *gateProbe) ExitShowStatement(ctx *parser.ShowStatementContext) {
	p.Builder.ExitShowStatement(ctx)
	p.newMeaning = append(p.newMeaning, p.gate(testGatedChange, ctx))
}

func buildWithProbe(t *testing.T, src string) (*ast.Program, *gateProbe) {
	t.Helper()
	var probe *gateProbe
	prog, errs := build(src, buildOptions{}, func(b *Builder) antlr.ParseTreeListener {
		probe = &gateProbe{Builder: b}
		return probe
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	return prog, probe
}

func TestLanguageHeader_GatedConstructDiffersByVersion(t *testing.T) {
	prog, probe := buildWithProbe(t, "show entities;\nshow modules;")
	if len(probe.newMeaning) != 2 || probe.newMeaning[0] || probe.newMeaning[1] {
		t.Fatalf("mdl 0: want the old meaning for both statements, got %v", probe.newMeaning)
	}
	if len(prog.LanguageNotes) != 2 {
		t.Fatalf("mdl 0: want one warning per gated construct, got %v", prog.LanguageNotes)
	}
	n := prog.LanguageNotes[1]
	if n.Code != "MDL-TEST-GATE" || n.Line != 2 || !strings.Contains(n.Message, "under mdl 1 it means the new thing") {
		t.Fatalf("mdl 0 warning: got %+v", n)
	}

	prog, probe = buildWithProbe(t, "mdl 1;\nshow entities;")
	if len(probe.newMeaning) != 1 || !probe.newMeaning[0] {
		t.Fatalf("mdl 1: want the new meaning, got %v", probe.newMeaning)
	}
	if len(prog.LanguageNotes) != 0 {
		t.Fatalf("mdl 1: the new meaning must not warn, got %v", prog.LanguageNotes)
	}
}
