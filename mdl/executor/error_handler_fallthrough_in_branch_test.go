// SPDX-License-Identifier: Apache-2.0

package executor

// A custom error handler that does not end in RETURN or RAISE ERROR falls
// through: after it runs, the flow continues with the statement after the
// activity that failed. At the top level of a flow that always held. Inside a
// branch — an IF, an IF/ELSE, a CASE, a type split, a loop body — the handler's
// pending rejoin leaked out of the branch, and the enclosing builder settled it
// on whatever flow it drew next (ako/mxcli#1459, mendixlabs/mxcli#1565):
//
//   - with a result variable (`$N = call …`) the handler flowed into a merge
//     spliced in FRONT of the enclosing split, so a call that kept failing
//     re-entered the decision for ever;
//   - without one it flowed into a new end event, and nothing after the
//     failure ran.
//
// check, exec and mx check all accepted both, so the tests here assert on the
// graph itself: where the handler's last activity goes next.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// fallThroughGraph indexes a built collection, nested loop bodies included, for
// the questions these tests ask.
type fallThroughGraph struct {
	objects map[model.ID]microflows.MicroflowObject
	flows   []*microflows.SequenceFlow
}

func newFallThroughGraph(oc *microflows.MicroflowObjectCollection) fallThroughGraph {
	g := fallThroughGraph{objects: map[model.ID]microflows.MicroflowObject{}, flows: oc.Flows}
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(c *microflows.MicroflowObjectCollection) {
		if c == nil {
			return
		}
		for _, o := range c.Objects {
			if o == nil {
				continue
			}
			g.objects[o.GetID()] = o
			if loop, ok := o.(*microflows.LoopedActivity); ok {
				walk(loop.ObjectCollection)
			}
		}
	}
	walk(oc)
	return g
}

// logID is the log activity whose message is text.
func (g fallThroughGraph) logID(t *testing.T, text string) model.ID {
	t.Helper()
	var found model.ID
	for id, o := range g.objects {
		act, ok := o.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		log, ok := act.Action.(*microflows.LogMessageAction)
		if !ok || log.MessageTemplate == nil || log.MessageTemplate.Translations["en_US"] != text {
			continue
		}
		if found != "" {
			t.Fatalf("two log activities say %q; the fixture must keep them distinct", text)
		}
		found = id
	}
	if found == "" {
		t.Fatalf("no log activity says %q", text)
	}
	return found
}

// errorSourceOf is the activity whose error flow leads to the handler that
// contains id (the handler's first activity is the error flow's destination).
func (g fallThroughGraph) errorSourceOf(t *testing.T, handlerFirst model.ID) model.ID {
	t.Helper()
	for _, f := range g.flows {
		if f.IsErrorHandler && f.DestinationID == handlerFirst {
			return f.OriginID
		}
	}
	t.Fatalf("no error flow enters %s", g.describe(handlerFirst))
	return ""
}

// next is where the path leaving id goes: its single normal outgoing flow,
// followed through any exclusive merges. "" when the path ends there — no
// outgoing flow, which inside a loop body ends the iteration.
func (g fallThroughGraph) next(t *testing.T, id model.ID) model.ID {
	t.Helper()
	seen := map[model.ID]bool{}
	for {
		var out []*microflows.SequenceFlow
		for _, f := range g.flows {
			if f.OriginID == id && !f.IsErrorHandler {
				out = append(out, f)
			}
		}
		switch len(out) {
		case 0:
			return ""
		case 1:
		default:
			t.Fatalf("%s has %d normal outgoing flows; expected one", g.describe(id), len(out))
		}
		dest := out[0].DestinationID
		if _, isMerge := g.objects[dest].(*microflows.ExclusiveMerge); !isMerge {
			return dest
		}
		if seen[dest] {
			t.Fatalf("the path from %s cycles through merges", g.describe(id))
		}
		seen[dest] = true
		id = dest
	}
}

func (g fallThroughGraph) describe(id model.ID) string {
	if id == "" {
		return "(nothing: the path ends)"
	}
	switch o := g.objects[id].(type) {
	case *microflows.ActionActivity:
		if log, ok := o.Action.(*microflows.LogMessageAction); ok && log.MessageTemplate != nil {
			return "log '" + log.MessageTemplate.Translations["en_US"] + "'"
		}
		return "activity"
	case *microflows.ExclusiveSplit:
		return "the split '" + o.Caption + "'"
	case *microflows.EndEvent:
		return "an end event"
	case nil:
		return "unknown object " + string(id)
	default:
		return strings.TrimPrefix(fmt.Sprintf("%T", o), "*microflows.")
	}
}

// assertFallsThroughTo checks that the handler whose last activity logs
// handlerTail continues at the activity logging want — and, as the control
// that the fixture's notion of "next" is the right one, that the failing
// activity's own normal path goes to sourceNext (the same statement, unless
// the skip-var rule makes the error path skip some).
func assertFallsThroughTo(t *testing.T, oc *microflows.MicroflowObjectCollection, handlerTail, want, sourceNext string) {
	t.Helper()
	g := newFallThroughGraph(oc)
	tail := g.logID(t, handlerTail)
	source := g.errorSourceOf(t, tail)
	var wantID model.ID
	if want != "" {
		wantID = g.logID(t, want)
	}
	sourceNextID := wantID
	if sourceNext != "" {
		sourceNextID = g.logID(t, sourceNext)
	}
	if got := g.next(t, source); got != sourceNextID {
		t.Fatalf("control: the failing activity's normal path goes to %s, want %s — fix the fixture",
			g.describe(got), g.describe(sourceNextID))
	}
	if got := g.next(t, tail); got != wantID {
		t.Errorf("the handler falls through to %s, want %s (the statement after the failing activity)",
			g.describe(got), g.describe(wantID))
	}
}

// buildProbeFromMDL is buildMicroflowFromMDL for a microflow or a nanoflow.
func buildProbeFromMDL(t *testing.T, src string) *microflows.MicroflowObjectCollection {
	t.Helper()
	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
		varTypes: map[string]string{},
	}
	for _, stmt := range parseMDL(t, src).Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			return fb.buildFlowGraph(s.Body, s.ReturnType)
		case *ast.CreateNanoflowStmt:
			fb.isNanoflow = true
			return fb.buildFlowGraph(s.Body, s.ReturnType)
		}
	}
	t.Fatal("no CREATE MICROFLOW or NANOFLOW in the parsed program")
	return nil
}

// describeCollection is describeBuilt for a collection already built.
func describeCollection(t *testing.T, oc *microflows.MicroflowObjectCollection) string {
	t.Helper()
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()
	return strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n")
}

const fallThroughPreamble = `create microflow M.Probe ($Flag: Boolean, $Flag2: Boolean, $Color: M.Color, $Things: List of M.Thing, $Obj: M.Base)
begin
`

func TestFallThroughHandlerInBranchContinuesAfterTheFailingActivity(t *testing.T) {
	cases := []struct {
		name string
		body string
		// want is the log the handler must continue at; "" means the path
		// ends there (a loop body's last statement ends the iteration).
		want string
		// sourceNext is where the failing activity's own normal path goes,
		// when that is not want.
		sourceNext string
		// joined: the rejoin is not on the flow leaving the failing activity,
		// so DESCRIBE spells it with merge/join labels — as it does at the
		// top level for the same skip-var shape.
		joined bool
		// nanoflow builds the body as a nanoflow.
		nanoflow bool
	}{
		{
			// The report's shape A: the endless loop.
			name: "if without else, call with a result variable",
			body: `  log info node 'P' 'before';
  if $Flag then
    $N = call microflow M.Callee () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			// The report's shape B: the silent end.
			name: "if without else, call without a result variable",
			body: `  log info node 'P' 'before';
  if $Flag then
    call microflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "then branch of an if/else",
			body: `  log info node 'P' 'before';
  if $Flag then
    $N = call microflow M.Callee () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  else
    log info node 'P' 'else';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "else branch of an if/else",
			body: `  log info node 'P' 'before';
  if $Flag then
    log info node 'P' 'then';
  else
    call microflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "create object with a handler",
			body: `  log info node 'P' 'before';
  if $Flag then
    $Obj = create M.Thing (Name = 'x') on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			// The statement after the failing activity is the one after the IF.
			name: "last statement of a then branch without else",
			body: `  log info node 'P' 'before';
  if $Flag then
    log info node 'P' 'then';
    call microflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
  end if;
  log info node 'P' 'end';`,
			want: "end",
		},
		{
			name: "last statement of an else branch",
			body: `  log info node 'P' 'before';
  if $Flag then
    log info node 'P' 'then';
  else
    $N = call microflow M.Callee () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
  end if;
  log info node 'P' 'end';`,
			want: "end",
		},
		{
			// ELSE returns, so THEN continues past the IF with no merge, and
			// the enclosing path draws the flow leaving the failing activity.
			name: "last statement of a then branch whose else returns",
			body: `  log info node 'P' 'before';
  if $Flag then
    log info node 'P' 'then';
    call microflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
  else
    return;
  end if;
  log info node 'P' 'end';`,
			want: "end",
		},
		{
			name: "rest call with a result variable",
			body: `  log info node 'P' 'before';
  if $Flag then
    $Response = call rest service get 'https://example.com/x' returns response on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "rest call returning nothing",
			body: `  log info node 'P' 'before';
  if $Flag then
    call rest service get 'https://example.com/x' returns nothing on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "nested ifs",
			body: `  log info node 'P' 'before';
  if $Flag then
    if $Flag2 then
      $N = call microflow M.Callee () on error without rollback begin
        log warning node 'P' 'failed';
      end error;
      log info node 'P' 'after call';
    else
      log info node 'P' 'inner else';
    end if;
    log info node 'P' 'after inner';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			// Last statement of the inner branch: the next statement is the
			// one after the INNER if, still inside the outer branch.
			name: "last statement of a nested branch",
			body: `  log info node 'P' 'before';
  if $Flag then
    if $Flag2 then
      call microflow M.Callee0 () on error without rollback begin
        log warning node 'P' 'failed';
      end error;
    end if;
    log info node 'P' 'after inner';
  end if;
  log info node 'P' 'end';`,
			want: "after inner",
		},
		{
			// The inner IF has no merge (its THEN returns), so the inner ELSE
			// continues through the outer branch's own next flow.
			name: "last statement of a nested branch without a merge",
			body: `  log info node 'P' 'before';
  if $Flag then
    if $Flag2 then
      return;
    else
      $N = call microflow M.Callee () on error without rollback begin
        log warning node 'P' 'failed';
      end error;
    end if;
    log info node 'P' 'after inner';
  end if;
  log info node 'P' 'end';`,
			want: "after inner",
		},
		{
			// The report's "third shape": THEN ends in RETURN, ELSE falls
			// through. The handler used to rejoin at the IF's exit merge,
			// skipping the rest of THEN, return included.
			name: "branch that ends in return",
			body: `  log info node 'P' 'before';
  if $Flag then
    call microflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
    return;
  else
    log info node 'P' 'else';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "case branch",
			body: `  log info node 'P' 'before';
  case $Color
    when Red then
      call microflow M.Callee0 () on error without rollback begin
        log warning node 'P' 'failed';
      end error;
      log info node 'P' 'after call';
    when Green, (empty) then
      log info node 'P' 'other';
  end case;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "type split branch",
			body: `  log info node 'P' 'before';
  split type $Obj
    when M.Thing then
      call microflow M.Callee0 () on error without rollback begin
        log warning node 'P' 'failed';
      end error;
      log info node 'P' 'after call';
    else
      log info node 'P' 'other';
  end split;
  log info node 'P' 'end';`,
			want: "after call",
		},
		{
			name: "while true body",
			body: `  log info node 'P' 'before';
  while true
  begin
    call nanoflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
    if $Flag then
      return;
    end if;
  end while;`,
			want:     "after call",
			nanoflow: true,
		},
		{
			// Inside a loop the leaked handler had no outgoing flow at all, so
			// a failure ended the iteration and skipped the rest of the body.
			// The loop cases are nanoflows: a microflow refuses a custom
			// handler inside a loop (CE0644, measured on 11.12.2), a nanoflow
			// accepts one.
			name: "loop body",
			body: `  log info node 'P' 'before';
  loop $T in $Things
  begin
    call nanoflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end loop;
  log info node 'P' 'end';`,
			want:     "after call",
			nanoflow: true,
		},
		{
			name: "while body",
			body: `  log info node 'P' 'before';
  while $Flag
  begin
    $N = call nanoflow M.Callee () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end while;
  log info node 'P' 'end';`,
			want:     "after call",
			nanoflow: true,
		},
		{
			// The last statement of a loop body has nothing after it: the
			// iteration ends, on the normal path and on the error path alike.
			name: "last statement of a loop body",
			body: `  log info node 'P' 'before';
  loop $T in $Things
  begin
    log info node 'P' 'first';
    call nanoflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
  end loop;
  log info node 'P' 'end';`,
			want:     "",
			nanoflow: true,
		},
		{
			// The skip-var rule: a statement that reads the failed call's
			// result is skipped on the error path, and the handler rejoins at
			// the first statement after it that does not.
			name: "result variable read later in the branch",
			body: `  log info node 'P' 'before';
  if $Flag then
    $N = call microflow M.Callee () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'got {1}' with ({1} = toString($N));
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';`,
			want:       "after call",
			sourceNext: "got {1}",
			joined:     true,
		},
		{
			// Control: at the top level this always worked.
			name: "top level, just before an if",
			body: `  $N = call microflow M.Callee () on error without rollback begin
    log warning node 'P' 'failed';
  end error;
  log info node 'P' 'after call';
  if $Flag then
    log info node 'P' 'then';
  end if;
  log info node 'P' 'end';`,
			want: "after call",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := fallThroughPreamble + tc.body + "\nend;"
			if tc.nanoflow {
				src = strings.Replace(src, "create microflow", "create nanoflow", 1)
			}
			oc := buildProbeFromMDL(t, src)
			assertFallsThroughTo(t, oc, "failed", tc.want, tc.sourceNext)

			// The description must spell what was written — a handler that
			// falls through — and executing it must rebuild the same graph.
			// The defect's two describe tells were a `join rejoin1;` with a
			// `merge rejoin1;` ahead of the IF, and a `return;` in the handler
			// that the source never had.
			out := describeCollection(t, oc)
			assertDescriptionRebuildsGraph(t, out, oc)
			for _, tell := range []string{"join rejoin", "merge rejoin"} {
				if !tc.joined && strings.Contains(out, tell) {
					t.Errorf("the description routes the handler through a label (%q):\n%s", tell, out)
				}
			}
			if n, m := strings.Count(out, "return;"), strings.Count(src, "return;"); n > m+1 {
				t.Errorf("the description has %d return statements, the source %d (+1 for the implicit end):\n%s", n, m, out)
			}
		})
	}
}

// In a flow that returns a value, the leaked handler without a result variable
// either got a new end event returning `$Result` — a variable that does not
// exist, CE0109 — or joined the flow's final `return true`, skipping the rest
// of the branch and everything after the IF (mendixlabs/mxcli#1565, comment).
func TestFallThroughHandlerInBranchOfAFlowWithAReturnValue(t *testing.T) {
	for name, header := range map[string]string{
		"named return value":   `create microflow M.Probe ($Flag: Boolean) returns Boolean as $Result`,
		"unnamed return value": `create microflow M.Probe ($Flag: Boolean) returns Boolean`,
	} {
		t.Run(name, func(t *testing.T) {
			src := header + `
begin
  log info node 'P' 'before';
  if $Flag then
    call microflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    log info node 'P' 'after call';
  end if;
  log info node 'P' 'end';
  return true;
end;`
			oc := buildMicroflowFromMDL(t, src)
			assertFallsThroughTo(t, oc, "failed", "after call", "")
			ends := 0
			for _, o := range oc.Objects {
				if _, ok := o.(*microflows.EndEvent); ok {
					ends++
				}
			}
			if ends != 1 {
				t.Errorf("got %d end events, want the one `return true` writes", ends)
			}
			out, built := describeBuilt(t, src)
			assertDescriptionRebuildsGraph(t, out, built)
		})
	}
}

// The skip-var rule in a branch that ends in RETURN: when the statement after
// the failing activity reads its result, the handler cannot continue in the
// branch, and continues where the IF does — a merge the IF gains for it. This
// is the shared continuation a30881bbb introduced for THEN, here from ELSE,
// where whether the merge is needed is known only after THEN was built.
func TestSkipVarHandlerInReturningElseContinuesAfterTheIf(t *testing.T) {
	src := `create microflow M.Probe ($Flag: Boolean) returns Integer
begin
  if $Flag then
    log info node 'P' 'then';
  else
    $N = call microflow M.Callee () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
    return $N;
  end if;
  log info node 'P' 'end';
  return 0;
end;`
	oc := buildMicroflowFromMDL(t, src)
	g := newFallThroughGraph(oc)
	if got, want := g.next(t, g.logID(t, "failed")), g.logID(t, "end"); got != want {
		t.Errorf("the handler continues at %s, want %s", g.describe(got), g.describe(want))
	}
	if got, want := g.next(t, g.logID(t, "then")), g.logID(t, "end"); got != want {
		t.Errorf("THEN continues at %s, want %s", g.describe(got), g.describe(want))
	}
	out, built := describeBuilt(t, src)
	assertDescriptionRebuildsGraph(t, out, built)
}

// The merge a handler on a branch's last activity rejoins at is spliced into
// the branch's exit into the IF's own merge. It stays on the branch's row,
// between the activity and the IF's merge, rather than beside the IF's merge
// on the main line.
func TestBranchExitRejoinMergeStaysOnTheBranchRow(t *testing.T) {
	src := fallThroughPreamble + `  if $Flag then
    log info node 'P' 'then';
    call microflow M.Callee0 () on error without rollback begin
      log warning node 'P' 'failed';
    end error;
  end if;
  log info node 'P' 'end';
end;`
	oc := buildMicroflowFromMDL(t, src)
	g := newFallThroughGraph(oc)
	call := g.errorSourceOf(t, g.logID(t, "failed"))
	var rejoin, closing microflows.MicroflowObject
	for _, f := range g.flows {
		if f.OriginID == call && !f.IsErrorHandler {
			rejoin = g.objects[f.DestinationID]
		}
	}
	if _, ok := rejoin.(*microflows.ExclusiveMerge); !ok {
		t.Fatalf("the call's normal flow enters %T, want the rejoin merge", rejoin)
	}
	for _, f := range g.flows {
		if f.OriginID == rejoin.GetID() {
			closing = g.objects[f.DestinationID]
		}
	}
	if _, ok := closing.(*microflows.ExclusiveMerge); !ok {
		t.Fatalf("the rejoin merge leads to %T, want the IF's merge", closing)
	}
	c, r, m := g.objects[call].GetPosition(), rejoin.GetPosition(), closing.GetPosition()
	if r.Y != c.Y {
		t.Errorf("rejoin merge at y=%d, want the branch's row y=%d (the IF's merge is at y=%d)", r.Y, c.Y, m.Y)
	}
	if !(c.X < r.X && r.X < m.X) {
		t.Errorf("rejoin merge at x=%d, want it between the call (x=%d) and the IF's merge (x=%d)", r.X, c.X, m.X)
	}
}
