// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestDropEventHandlerCallParses covers the CALL clause on DROP EVENT HANDLER.
// An entity can carry several handlers for one event (mendixlabs/mxcli#1540),
// so a drop has to be able to name which one; the bare form still parses and
// leaves the microflow empty.
func TestDropEventHandlerCallParses(t *testing.T) {
	prog, errs := Build(`
alter entity M.E drop event handler on before commit call M.Second;
alter entity M.E drop event handler if exists on after delete call M."Third";
alter entity M.E drop event handler on before rollback;
`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %s", errsText(errs))
	}
	if len(prog.Statements) != 3 {
		t.Fatalf("got %d statements, want 3", len(prog.Statements))
	}

	want := []struct {
		moment, event, mf string
		ifExists          bool
	}{
		{"Before", "Commit", "M.Second", false},
		{"After", "Delete", "M.Third", true},
		{"Before", "RollBack", "", false},
	}
	for i, w := range want {
		s, ok := prog.Statements[i].(*ast.AlterEntityStmt)
		if !ok || s.Operation != ast.AlterEntityDropEventHandler || s.EventHandler == nil {
			t.Fatalf("statement %d: not a DROP EVENT HANDLER (%+v)", i, prog.Statements[i])
		}
		eh := s.EventHandler
		got := ""
		if eh.Microflow.Name != "" {
			got = eh.Microflow.String()
		}
		if eh.Moment != w.moment || eh.Event != w.event || got != w.mf || s.IfExists != w.ifExists {
			t.Errorf("statement %d: got %s %s call %q ifExists=%v, want %s %s call %q ifExists=%v",
				i, eh.Moment, eh.Event, got, s.IfExists, w.moment, w.event, w.mf, w.ifExists)
		}
	}
}
