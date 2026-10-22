// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// writtenMicroflowFromMDL parses one CREATE MICROFLOW statement and returns the
// microflow handed to the backend. Parsing (rather than building the AST by
// hand) is the point: the visitor types every bare qualified name in a
// parameter or return position as an entity, and the executor is what has to
// tell an enumeration apart.
func writtenMicroflowFromMDL(t *testing.T, src string) *microflows.Microflow {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}
	ctx, written := microflowWriteProbe(t, nil, model.ID("module-1"))
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	return *written
}

func wantEnumerationType(t *testing.T, what string, dt microflows.DataType, qn string) {
	t.Helper()
	et, ok := dt.(*microflows.EnumerationType)
	if !ok {
		t.Fatalf("%s: got %T (%s), want *microflows.EnumerationType %s", what, dt, dt.GetTypeName(), qn)
	}
	if et.EnumerationQualifiedName != qn {
		t.Fatalf("%s: enumeration = %q, want %q", what, et.EnumerationQualifiedName, qn)
	}
}

// TestCreateMicroflow_SystemEnumerationParameter: a System enumeration named
// bare in a parameter used to be written as DataTypes$ObjectType with Entity
// "System.UserType" — on every engine. The System module is not stored in the
// project, so the entity-then-enumeration lookup was skipped for it outright
// and the visitor's entity guess stood. The local engines wrote that without
// complaint (and describe rendered it as a plausible bare `System.UserType`);
// Studio Pro's PED refused it over --mcp: "Reference with qualified name
// System.UserType of type DomainModels$Entity not found."
func TestCreateMicroflow_SystemEnumerationParameter(t *testing.T) {
	mf := writtenMicroflowFromMDL(t, `create microflow MyModule.MF ($T: System.UserType)
begin
end;`)
	if len(mf.Parameters) != 1 {
		t.Fatalf("got %d parameters, want 1", len(mf.Parameters))
	}
	wantEnumerationType(t, "parameter $T", mf.Parameters[0].Type, "System.UserType")
}

func TestCreateMicroflow_SystemEnumerationReturnType(t *testing.T) {
	mf := writtenMicroflowFromMDL(t, `create microflow MyModule.MF ()
returns System.DeviceType
begin
  return System.DeviceType.Desktop;
end;`)
	wantEnumerationType(t, "return type", mf.ReturnType, "System.DeviceType")
}

// The explicit spelling is what DESCRIBE emits for an enumeration parameter, so
// it must round-trip. It used to be refused outright: the System module's
// enumerations are not in ListEnumerations, so validation reported
// "enumeration 'System.UserType' not found".
func TestCreateMicroflow_ExplicitSystemEnumerationParameter(t *testing.T) {
	mf := writtenMicroflowFromMDL(t, `create microflow MyModule.MF ($T: enum System.UserType)
begin
end;`)
	wantEnumerationType(t, "parameter $T", mf.Parameters[0].Type, "System.UserType")
}

// Control: a System *entity* must stay an object type, or the fix has merely
// flipped the guess.
func TestCreateMicroflow_SystemEntityParameterStaysObject(t *testing.T) {
	mf := writtenMicroflowFromMDL(t, `create microflow MyModule.MF ($U: System.User)
begin
end;`)
	ot, ok := mf.Parameters[0].Type.(*microflows.ObjectType)
	if !ok {
		t.Fatalf("parameter $U: got %T, want *microflows.ObjectType", mf.Parameters[0].Type)
	}
	if ot.EntityQualifiedName != "System.User" {
		t.Fatalf("parameter $U: entity = %q, want System.User", ot.EntityQualifiedName)
	}
}

// The nanoflow builder carries its own copy of the parameter resolution, so it
// needs its own case.
func TestCreateNanoflow_SystemEnumerationParameter(t *testing.T) {
	prog, errs := visitor.Build(`create nanoflow MyModule.NF ($D: System.DeviceType)
begin
end;`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	stmt, ok := prog.Statements[0].(*ast.CreateNanoflowStmt)
	if !ok {
		t.Fatalf("expected CreateNanoflowStmt, got %T", prog.Statements[0])
	}
	const moduleID = model.ID("module-1")
	var written *microflows.Nanoflow
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: moduleID}, Name: "MyModule"}}, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			return &model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: "MyModule"}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
		CreateNanoflowFunc: func(nf *microflows.Nanoflow) error {
			written = nf
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	if err := execCreateNanoflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE NANOFLOW failed: %v", err)
	}
	if written == nil {
		t.Fatal("no nanoflow was written")
	}
	wantEnumerationType(t, "parameter $D", written.Parameters[0].Type, "System.DeviceType")
}
