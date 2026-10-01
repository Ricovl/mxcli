// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#843: a flow that calls itself is created in one statement. The
// call target was resolved against the project only, where the flow being
// created does not exist yet, so exec refused "microflow not found" while
// check passed the same script — and the stub-then-real workaround is refused
// by the mdl 1 splice.

const recursiveModuleID = model.ID("module-1")

func findCall[T any](objs []microflows.MicroflowObject) (T, bool) {
	var zero T
	for _, o := range objs {
		if a, ok := o.(*microflows.ActionActivity); ok {
			if c, ok := a.Action.(T); ok {
				return c, true
			}
		}
	}
	return zero, false
}

func TestCreateMicroflow_CallsItself(t *testing.T) {
	ctx, written := microflowWriteProbe(t, nil, recursiveModuleID)
	stmt := firstStatement[*ast.CreateMicroflowStmt](t, `create or modify microflow MyModule.Countdown ($N: Integer)
returns Boolean as $Done
begin
  if $N <= 0 then
    return true;
  end if;
  $Below = call microflow MyModule.Countdown(N = $N - 1);
  return $Below;
end;`)
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("a self-recursive microflow is refused: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	call, ok := findCall[*microflows.MicroflowCallAction]((*written).ObjectCollection.Objects)
	if !ok {
		t.Fatal("the written microflow has no call activity")
	}
	if got := call.MicroflowCall.Microflow; got != "MyModule.Countdown" {
		t.Errorf("the call targets %q, want the microflow itself", got)
	}
}

// The control: the self-name is the only name the statement adds. A call to a
// microflow that is neither stored nor the one being created is still refused.
func TestCreateMicroflow_CallToAMissingMicroflowStillRefused(t *testing.T) {
	ctx, _ := microflowWriteProbe(t, nil, recursiveModuleID)
	stmt := firstStatement[*ast.CreateMicroflowStmt](t, `create or modify microflow MyModule.Countdown ($N: Integer)
returns Boolean
begin
  $Below = call microflow MyModule.Elsewhere(N = $N - 1);
  return $Below;
end;`)
	err := execCreateMicroflow(ctx, stmt)
	if err == nil || !strings.Contains(err.Error(), "MyModule.Elsewhere") {
		t.Fatalf("a call to a missing microflow must still be refused, got %v", err)
	}
}

// A nanoflow cannot call a microflow of its own name: the self-name resolves
// only for the same kind of flow.
func TestCreateMicroflow_SelfNameIsNotANanoflow(t *testing.T) {
	var written *microflows.Nanoflow
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: recursiveModuleID}, Name: "MyModule"}}, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name != "MyModule" {
				return nil, nil
			}
			return &model.Module{BaseElement: model.BaseElement{ID: recursiveModuleID}, Name: "MyModule"}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return nil, nil },
		ListNanoflowsFunc:  func() ([]*microflows.Nanoflow, error) { return nil, nil },
		CreateNanoflowFunc: func(nf *microflows.Nanoflow) error { written = nf; return nil },
		UpdateNanoflowFunc: func(nf *microflows.Nanoflow) error { written = nf; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	self := firstStatement[*ast.CreateNanoflowStmt](t, `create or modify nanoflow MyModule.CountdownNf ($N: Integer)
returns Boolean as $Done
begin
  if $N <= 0 then
    return true;
  end if;
  $Below = call nanoflow MyModule.CountdownNf(N = $N - 1);
  return $Below;
end;`)
	if err := execCreateNanoflow(ctx, self); err != nil {
		t.Fatalf("a self-recursive nanoflow is refused: %v", err)
	}
	if written == nil {
		t.Fatal("no nanoflow was written")
	}
	call, ok := findCall[*microflows.NanoflowCallAction](written.ObjectCollection.Objects)
	if !ok || call.NanoflowCall.Nanoflow != "MyModule.CountdownNf" {
		t.Fatalf("the nanoflow does not call itself: %+v", call)
	}

	cross := firstStatement[*ast.CreateNanoflowStmt](t, `create or modify nanoflow MyModule.CountdownNf ($N: Integer)
begin
  call microflow MyModule.CountdownNf(N = $N - 1);
end;`)
	if err := execCreateNanoflow(ctx, cross); err == nil || !strings.Contains(err.Error(), "microflow not found") {
		t.Fatalf("a nanoflow calling a microflow of its own name must be refused, got %v", err)
	}
}
