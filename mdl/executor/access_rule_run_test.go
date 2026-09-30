// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
)

// deferringMock is a mock backend with the deferred-write capability whose
// flush fails, standing in for a held write that cannot land.
type deferringMock struct {
	*mock.MockBackend
	opened, flushed int
	flushErr        error
}

func (d *deferringMock) DeferUnitWrites() { d.opened++ }
func (d *deferringMock) FlushDeferredWrites() error {
	d.flushed++
	return d.flushErr
}

// ako/mxcli#872: a run of access-rule statements is written at its end. When a
// statement of the run fails, the run is still flushed — and if that flush fails
// too, the earlier statements' writes did not land although each printed that it
// had. That failure must be returned with the statement's, not discarded.
func TestAccessRuleRun_FlushErrorOnFailedStatementIsReturned(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(e *Executor, prog *ast.Program) error
	}{
		{"ExecuteProgram", func(e *Executor, prog *ast.Program) error { return e.ExecuteProgram(prog) }},
		{"ExecuteProgramContinueOnError", func(e *Executor, prog *ast.Program) error {
			var out bytes.Buffer
			_, err := e.ExecuteProgramContinueOnError(prog, &out)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			e := New(&buf)
			d := &deferringMock{
				MockBackend: &mock.MockBackend{IsConnectedFunc: func() bool { return true }},
				flushErr:    errors.New("held write could not land"),
			}
			e.backend = d
			// The mock knows no module, so the grant fails after the run opened.
			prog := &ast.Program{Statements: []ast.Statement{
				&ast.GrantEntityAccessStmt{Entity: ast.QualifiedName{Module: "M", Name: "E"}},
			}}
			err := tc.run(e, prog)
			if d.opened != 1 || d.flushed != 1 {
				t.Fatalf("run opened %d / flushed %d times, want 1 / 1", d.opened, d.flushed)
			}
			if err == nil || !strings.Contains(err.Error(), "held write could not land") {
				t.Errorf("the failed flush was discarded; got error %v", err)
			}
		})
	}

	// Control: a flush that succeeds adds nothing to the statement's error.
	var buf bytes.Buffer
	e := New(&buf)
	d := &deferringMock{MockBackend: &mock.MockBackend{IsConnectedFunc: func() bool { return true }}}
	e.backend = d
	err := e.ExecuteProgram(&ast.Program{Statements: []ast.Statement{
		&ast.GrantEntityAccessStmt{Entity: ast.QualifiedName{Module: "M", Name: "E"}},
	}})
	if err == nil || strings.Contains(err.Error(), "write access rules") {
		t.Errorf("control: want only the statement's own error, got %v", err)
	}
}
