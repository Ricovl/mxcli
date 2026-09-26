// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A handler whose meaning depends on the script's `mdl <n>;` header reads it
// from its ExecContext. The version belongs to the program being run, so a
// nested EXECUTE SCRIPT runs under its own header and the caller's is back
// afterwards.
func TestExecContextCarriesLanguageVersion(t *testing.T) {
	e := New(io.Discard)
	if got := e.newExecContext(context.Background()).LanguageVersion; got != langver.V0 {
		t.Fatalf("outside a program: got %s, want mdl 0", got)
	}

	restoreOuter := e.enterLanguage(langver.V1)
	if got := e.newExecContext(context.Background()).LanguageVersion; got != langver.V1 {
		t.Fatalf("inside an mdl 1 program: got %s", got)
	}
	restoreInner := e.enterLanguage(langver.V0)
	if got := e.newExecContext(context.Background()).LanguageVersion; got != langver.V0 {
		t.Fatalf("inside a nested headerless script: got %s", got)
	}
	restoreInner()
	if got := e.newExecContext(context.Background()).LanguageVersion; got != langver.V1 {
		t.Fatalf("after the nested script: got %s, want the caller's mdl 1 back", got)
	}
	restoreOuter()
	if got := e.newExecContext(context.Background()).LanguageVersion; got != langver.V0 {
		t.Fatalf("after the program: got %s, want mdl 0", got)
	}
}

// End to end: the header a script was parsed with is the version its
// statements' handlers see, and a statement run outside the program is mdl 0.
// The headerless script is the control.
func TestExecuteProgramRunsStatementsUnderTheHeader(t *testing.T) {
	e := New(io.Discard)
	var seen []langver.Version
	e.registry.handlers[reflect.TypeOf(&ast.ShowStmt{})] = func(ctx *ExecContext, _ ast.Statement) error {
		seen = append(seen, ctx.LanguageVersion)
		return nil
	}
	for _, src := range []string{"show modules;", "mdl 1;\nshow modules;"} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		if err := e.ExecuteProgram(prog); err != nil {
			t.Fatal(err)
		}
		if err := e.Execute(prog.Statements[0]); err != nil {
			t.Fatal(err)
		}
	}
	want := []langver.Version{langver.V0, langver.V0, langver.V1, langver.V0}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("handlers saw %v, want %v (headerless, outside, mdl 1, outside)", seen, want)
	}
}

// `exec --continue-on-error` runs through ExecuteProgramContinueOnError, a
// separate entry point that must enter the header's version too.
func TestExecuteProgramContinueOnErrorRunsStatementsUnderTheHeader(t *testing.T) {
	e := New(io.Discard)
	var seen []langver.Version
	e.registry.handlers[reflect.TypeOf(&ast.ShowStmt{})] = func(ctx *ExecContext, _ ast.Statement) error {
		seen = append(seen, ctx.LanguageVersion)
		return nil
	}
	for _, src := range []string{"show modules;", "mdl 1;\nshow modules;"} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		if _, err := e.ExecuteProgramContinueOnError(prog, io.Discard); err != nil {
			t.Fatal(err)
		}
		if err := e.Execute(prog.Statements[0]); err != nil {
			t.Fatal(err)
		}
	}
	want := []langver.Version{langver.V0, langver.V0, langver.V1, langver.V0}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("handlers saw %v, want %v (headerless, outside, mdl 1, outside)", seen, want)
	}
}
