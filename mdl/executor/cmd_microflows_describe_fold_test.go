// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// #750 (R12): describe folds structured control flow back into the shape it was
// written in. These tests build a microflow from MDL, describe it, and hold the
// description to two laws:
//
//   - canonical: the folded form is what comes out;
//   - round trip: executing the description rebuilds the same graph, and
//     describing that graph again gives the same text (PutGet).

const foldHeader = "create microflow M.Fold ($In: Integer) returns Integer\nbegin\n"

// describeFold builds src and returns the described body.
func describeFold(t *testing.T, src string) (string, *microflows.MicroflowObjectCollection) {
	t.Helper()
	oc := buildMicroflowFromMDL(t, src)
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()
	return strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n"), oc
}

// graphShape is the topology of a collection by node kind, order-independent.
func graphShape(col *microflows.MicroflowObjectCollection) string {
	var parts []string
	for _, e := range graphEdges(col) {
		parts = append(parts, fmt.Sprintf("%s->%s/%v", e.from, e.to, e.isError))
	}
	sort.Strings(parts)
	return fmt.Sprintf("%d objects; %s", len(col.Objects), strings.Join(parts, " "))
}

// assertFoldRoundTrips re-executes a description and checks the rebuilt graph
// and its description against the first.
func assertFoldRoundTrips(t *testing.T, described string, original *microflows.MicroflowObjectCollection) {
	t.Helper()
	again, rebuilt := describeFold(t, foldHeader+described+"\nend;")
	if got, want := graphShape(rebuilt), graphShape(original); got != want {
		t.Errorf("re-executing the description built a different graph\n got: %s\nwant: %s\ndescription:\n%s", got, want, described)
	}
	if again != described {
		t.Errorf("describe is not a fixed point\nfirst:\n%s\n\nsecond:\n%s", described, again)
	}
}

const elsifMDL = foldHeader + `  declare $N Integer = 0;
  if $In = 1 then
    set $N = 10;
  elsif $In = 2 then
    set $N = 20;
  elsif $In = 3 then
    set $N = 30;
  else
    set $N = 40;
  end if;
  return $N;
end;`

// A lone if in an else branch comes back as elsif, not as a nested if.
func TestDescribeFold_LoneIfInElseIsElsif(t *testing.T) {
	out, oc := describeFold(t, elsifMDL)

	for _, want := range []string{"elsif $In = 2 then", "elsif $In = 3 then"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q — a lone if in an else branch must come back as elsif:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "end if;"); n != 1 {
		t.Errorf("got %d `end if;`, want 1 for a single if/elsif chain:\n%s", n, out)
	}
	assertFoldRoundTrips(t, out, oc)
}

// The control: an else branch holding an if AND another statement is not a
// lone if, and must stay nested — folding it would move the trailing statement
// into the last arm.
func TestDescribeFold_ElseWithMoreThanAnIfStaysNested(t *testing.T) {
	out, oc := describeFold(t, foldHeader+`  declare $N Integer = 0;
  if $In = 1 then
    set $N = 10;
  else
    if $In = 2 then
      set $N = 20;
    end if;
    set $N = $N + 1;
  end if;
  return $N;
end;`)

	if strings.Contains(out, "elsif") {
		t.Errorf("an else branch with a statement after its if was folded into elsif:\n%s", out)
	}
	assertFoldRoundTrips(t, out, oc)
}

const fallThroughHandlerMDL = foldHeader + `  declare $N Integer = 0;
  $R = call microflow M.Sub(In = 1) on error without rollback {
    log error node 'X' 'failed';
  };
  set $N = 5;
  return $N;
end;`

// An error handler that does not end in return or raise error falls through:
// its path rejoins the normal one right after the activity it guards. That is
// how it was written, and describe used to spell it back as a goto —
// `join rejoin1;` in the handler and a `merge rejoin1;` after the activity.
func TestDescribeFold_FallThroughHandlerHasNoLabels(t *testing.T) {
	out, oc := describeFold(t, fallThroughHandlerMDL)

	if strings.Contains(out, "join ") || strings.Contains(out, "merge ") {
		t.Errorf("a fall-through handler came back as join/merge labels:\n%s", out)
	}
	if !regexp.MustCompile(`log error node 'X' 'failed';\n\s*};`).MatchString(out) {
		t.Errorf("the handler body is not closed right after its statement:\n%s", out)
	}
	assertFoldRoundTrips(t, out, oc)
}

// The rejoin merge is a node on the canvas, and Studio Pro puts it wherever the
// modeller dragged it. Folding away its `merge` line must not fold away its
// position: it is carried as @merge on the guarded activity, the same
// annotation a decision uses for the merge that closes it.
func TestDescribeFold_FallThroughHandlerKeepsMergePosition(t *testing.T) {
	_, oc := describeFold(t, foldHeader+`  @position(360, 200)
  declare $N Integer = 0;
  @position(520, 200)
  $R = call microflow M.Sub(In = 1) on error without rollback {
    @position(520, 350)
    log error node 'X' 'failed';
    join rejoin1;
  };
  @position(905, 333)
  merge rejoin1;
  @position(1000, 200)
  set $N = 5;
  @position(1200, 200)
  return $N;
end;`)
	out, _ := describeFold(t, foldHeader+mustDescribe(t, oc)+"\nend;")

	if strings.Contains(out, "join ") {
		t.Fatalf("a fall-through handler came back as join/merge labels:\n%s", out)
	}
	if !strings.Contains(out, "@merge(905, 333)") {
		t.Errorf("the rejoin merge's position was dropped with its label:\n%s", out)
	}
	assertFoldRoundTrips(t, out, oc)

	_, rebuilt := describeFold(t, foldHeader+out+"\nend;")
	found := false
	for _, o := range rebuilt.Objects {
		if m, ok := o.(*microflows.ExclusiveMerge); ok {
			found = true
			if p := m.GetPosition(); p.X != 905 || p.Y != 333 {
				t.Errorf("rebuilt rejoin merge at (%d, %d), want (905, 333)", p.X, p.Y)
			}
		}
	}
	if !found {
		t.Errorf("rebuilt graph has no rejoin merge")
	}
}

// The control: a handler that rejoins somewhere OTHER than right after its
// activity is a real goto, and must keep its labels — describing it as a
// fall-through would re-execute to a different graph (the bug the labels were
// introduced for).
func TestDescribeFold_HandlerRejoiningFurtherDownKeepsLabels(t *testing.T) {
	out, oc := describeFold(t, foldHeader+`  declare $N Integer = 0;
  $R = call microflow M.Sub(In = 1) on error without rollback {
    log error node 'X' 'failed';
    join later;
  };
  set $N = 5;
  merge later;
  return $N;
end;`)

	if !strings.Contains(out, "join rejoin1;") || !strings.Contains(out, "merge rejoin1;") {
		t.Errorf("a handler rejoining past the next activity lost its labels:\n%s", out)
	}
	assertFoldRoundTrips(t, out, oc)
}

func mustDescribe(t *testing.T, oc *microflows.MicroflowObjectCollection) string {
	t.Helper()
	e := newTestExecutor()
	return strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), &microflows.Microflow{ObjectCollection: oc}, nil, nil), "\n")
}

// An empty handler is the degenerate fall-through: the error edge lands on the
// rejoin merge itself.
func TestDescribeFold_EmptyFallThroughHandler(t *testing.T) {
	out, oc := describeFold(t, foldHeader+`  declare $N Integer = 0;
  $R = call microflow M.Sub(In = 1) on error without rollback { };
  set $N = 5;
  return $N;
end;`)

	if strings.Contains(out, "join ") || strings.Contains(out, "merge ") {
		t.Errorf("an empty fall-through handler came back as join/merge labels:\n%s", out)
	}
	assertFoldRoundTrips(t, out, oc)
}

// An error handler's body is described by its own emitter; a lone if in an
// else branch there folds the same way.
func TestDescribeFold_ElsifInsideErrorHandler(t *testing.T) {
	out, oc := describeFold(t, foldHeader+`  declare $N Integer = 0;
  $R = call microflow M.Sub(In = 1) on error without rollback {
    if $In = 1 then
      return 1;
    elsif $In = 2 then
      return 2;
    else
      return 3;
    end if;
  };
  return $N;
end;`)

	if !strings.Contains(out, "elsif $In = 2 then") {
		t.Errorf("a lone if in an else inside an error handler did not fold:\n%s", out)
	}
	assertFoldRoundTrips(t, out, oc)
}

// `when A, B then` is one branch with two case values, stored as one flow per
// value into a merge in front of the branch body. The graph analysis counted
// those flows as separate branches sharing a body, so describe labelled the
// merge and spelled the case back as `when A, B then join shared1;` with the
// body after `end case` — or, where it could not label it, under a warning that
// the description was not equivalent to the microflow.
func TestDescribeFold_GroupedCaseValuesCarryNoWarning(t *testing.T) {
	src := `create microflow M.Fold ($C: enum M.Color) returns Integer
begin
  declare $N Integer = 0;
  case $C
    when Red, Green then
      set $N = 1;
    when Blue then
      set $N = 2;
    when (empty) then
      set $N = 3;
  end case;
  return $N;
end;`
	oc := buildMicroflowFromMDL(t, src)
	out := mustDescribe(t, oc)

	if !strings.Contains(out, "when Red, Green then") {
		t.Errorf("grouped case values did not come back as one when:\n%s", out)
	}
	if strings.Contains(out, "WARNING") || strings.Contains(out, "join ") {
		t.Errorf("a grouped case came back as join/merge labels or under a not-equivalent warning:\n%s", out)
	}

	header := src[:strings.Index(src, "begin\n")+len("begin\n")]
	rebuilt := buildMicroflowFromMDL(t, header+out+"\nend;")
	if got, want := graphShape(rebuilt), graphShape(oc); got != want {
		t.Errorf("re-executing the description built a different graph\n got: %s\nwant: %s", got, want)
	}
	if again := mustDescribe(t, rebuilt); again != out {
		t.Errorf("describe is not a fixed point\nfirst:\n%s\n\nsecond:\n%s", out, again)
	}
}
