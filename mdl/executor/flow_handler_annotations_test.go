// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func buildCollectionFromMDL(t *testing.T, nanoflow bool, src string) *microflows.MicroflowObjectCollection {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	var body []ast.MicroflowStatement
	switch s := prog.Statements[len(prog.Statements)-1].(type) {
	case *ast.CreateMicroflowStmt:
		body = s.Body
	case *ast.CreateNanoflowStmt:
		body = s.Body
	}
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, isNanoflow: nanoflow}
	return fb.buildFlowGraph(body, nil)
}

func objectAtPos(oc *microflows.MicroflowObjectCollection, x, y int) model.ID {
	for _, o := range oc.Objects {
		if p := o.GetPosition(); p.X == x && p.Y == y {
			return o.GetID()
		}
	}
	return ""
}

// mendixlabs/mxcli#991. Inside `on error … begin … end error` the statements'
// @position was honoured but @anchor and @curve were not: the handler loop
// created its flows with the builder's default sides and never recorded a
// curve, so check and exec passed and the edges came out straight, on default
// sides. They now mean what they mean outside a handler — @anchor's `to:` is
// the side the statement's incoming edge enters (here, the error edge), `from:`
// and @curve shape the edge leaving it.
func TestErrorHandlerBody_HonoursAnchorAndCurve(t *testing.T) {
	src := `mdl 1;
create nanoflow M.NF ()
begin
  @position(240, 200)
  call microflow M.SUB() on error without rollback begin
    @position(240, 380)
    @anchor(from: right, to: left)
    @curve(from: (0, 30), to: (0, -30))
    log error node 'R' 'one';
    @position(440, 380)
    @anchor(to: top)
    log error node 'R' 'two';
  end error;
  @position(560, 200)
  return;
end;`
	oc := buildCollectionFromMDL(t, true, src)
	one, two := objectAtPos(oc, 240, 380), objectAtPos(oc, 440, 380)
	if one == "" || two == "" {
		t.Fatal("handler activities not at their @position")
	}
	var errEdge, between *microflows.SequenceFlow
	for _, f := range oc.Flows {
		switch {
		case f.IsErrorHandler && f.DestinationID == one:
			errEdge = f
		case f.OriginID == one && f.DestinationID == two:
			between = f
		}
	}
	if errEdge == nil || between == nil {
		t.Fatalf("missing flows: error edge %v, between %v", errEdge, between)
	}
	if errEdge.DestinationConnectionIndex != AnchorLeft {
		t.Errorf("error edge enters side %d, want left (%d) from @anchor(to: left)", errEdge.DestinationConnectionIndex, AnchorLeft)
	}
	if between.OriginConnectionIndex != AnchorRight || between.DestinationConnectionIndex != AnchorTop {
		t.Errorf("handler edge is (%d,%d), want (right,top) = (%d,%d)", between.OriginConnectionIndex, between.DestinationConnectionIndex, AnchorRight, AnchorTop)
	}
	if between.OriginControlVector != "0;30" || between.DestinationControlVector != "0;-30" {
		t.Errorf("handler edge curve is %q/%q, want 0;30/0;-30", between.OriginControlVector, between.DestinationControlVector)
	}
}

// The last handler statement's @anchor(from:) and @curve shape the edge that
// rejoins the main flow; its `to:` stays on its own incoming edge.
func TestErrorHandlerBody_TailEdgeTakesTheLastStatementsFromAndCurve(t *testing.T) {
	src := `mdl 1;
create nanoflow M.NF ()
begin
  @position(240, 200)
  call microflow M.SUB() on error without rollback begin
    @position(240, 380)
    @anchor(from: top, to: left)
    @curve(from: (0, -30), to: (-30, 0))
    log error node 'R' 'one';
  end error;
  @position(560, 200)
  return;
end;`
	oc := buildCollectionFromMDL(t, true, src)
	one := objectAtPos(oc, 240, 380)
	var tail *microflows.SequenceFlow
	for _, f := range oc.Flows {
		if f.OriginID == one && !f.IsErrorHandler {
			tail = f
		}
	}
	if tail == nil {
		t.Fatal("no edge leaves the handler")
	}
	if tail.OriginConnectionIndex != AnchorTop {
		t.Errorf("rejoin edge leaves side %d, want top (%d)", tail.OriginConnectionIndex, AnchorTop)
	}
	if tail.DestinationConnectionIndex == AnchorLeft {
		t.Errorf("the statement's `to:` leaked onto its outgoing edge")
	}
	if tail.OriginControlVector != "0;-30" || tail.DestinationControlVector != "-30;0" {
		t.Errorf("rejoin edge curve is %q/%q, want 0;-30/-30;0", tail.OriginControlVector, tail.DestinationControlVector)
	}
}

// mendixlabs/mxcli#992, end to end: the one-sided per-case form describe emits
// now reaches the true edge.
func TestSplitBranch_OneSidedAnchorReachesTheEdge(t *testing.T) {
	src := `mdl 1;
create nanoflow M.NF ($Q: Integer)
begin
  @position(360, 200)
  @anchor(true: (to: top))
  if $Q = 0 then
    @position(560, 80)
    log info node 'a' 'b';
  else
    @position(560, 320)
    log info node 'a' 'c';
  end if;
end;`
	oc := buildCollectionFromMDL(t, true, src)
	dest := objectAtPos(oc, 560, 80)
	for _, f := range oc.Flows {
		if f.DestinationID == dest {
			if f.DestinationConnectionIndex != AnchorTop {
				t.Errorf("true edge enters side %d, want top (%d)", f.DestinationConnectionIndex, AnchorTop)
			}
			return
		}
	}
	t.Fatal("no true edge")
}

// mendixlabs/mxcli#991, the read half. DESCRIBE printed no @position — nor
// @anchor, @curve, @caption, @color or @excluded — for a statement inside an
// error handler, though the builder honours each there. So describe → exec of a
// flow whose handler someone laid out put the handler back on auto-placement,
// reporting success. The handler body now carries the same annotations as the
// main path, and a second describe is a fixed point.
func TestErrorHandlerBody_DescribeKeepsItsLayout(t *testing.T) {
	src := `mdl 1;
create microflow M.F ()
begin
  @position(240, 200)
  call microflow M.SUB() on error without rollback begin
    @position(240, 380)
    @anchor(from: right, to: left)
    @curve(from: (0, 30), to: (0, -30))
    @caption 'Handled'
    @color Red
    log error node 'R' 'one';
  end error;
  @position(560, 200)
  return;
end;`
	oc := buildCollectionFromMDL(t, false, src)
	body := describeBody(t, &microflows.Microflow{ObjectCollection: oc})
	for _, want := range []string{"@position(240, 380)", "to: left", "@curve(from: (0, 30), to: (0, -30))", "@caption 'Handled'", "@color Red"} {
		if !strings.Contains(body, want) {
			t.Errorf("describe dropped %q inside the handler:\n%s", want, body)
		}
	}
	again := buildCollectionFromMDL(t, false, "mdl 1;\ncreate microflow M.F ()\nbegin\n"+body+"\nend;")
	if second := describeBody(t, &microflows.Microflow{ObjectCollection: again}); second != body {
		t.Errorf("describe is not a fixed point:\n--- first\n%s\n--- second\n%s", body, second)
	}
}
