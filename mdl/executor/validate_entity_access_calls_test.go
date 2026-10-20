// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1500 (Mendix 11.14.0, MPR v2): `check -p --references`
// passed a microflow with @applyentityaccess(false) calling one with
// @applyentityaccess(true) — "no flow change exec would refuse … no used flow
// left without access … Check passed!" — and mxbuild refused it:
//
//	CE0114 A microflow that does not apply entity access can only call
//	microflows that also do not apply entity access.
//
// Measured with mxbuild 11.14.0 on a fresh project (see the finding): the
// reverse direction, true→true and false→false build clean; a decision calling
// a rule with entity access from a flow without it is CE0114 with "rules"; a
// nanoflow calling a microflow with entity access, and an EXCLUDED caller, are
// not reported. The security level does not matter (Off and Prototype alike).

var eaCallsModule = &model.Module{BaseElement: model.BaseElement{ID: "mod-mymodule"}, Name: "MyModule"}

type eaCallsFixture struct {
	units []*types.RawUnit
}

// addStored stores a microflow ("Microflows$Microflow") or rule
// ("Microflows$Rule") with the given apply-entity-access setting, whose body
// calls each of calls ("microflow:Mod.Name" or "rule:Mod.Name").
func (f *eaCallsFixture) addStored(typ, name string, ea, excluded bool, calls ...string) {
	var objs bson.A
	objs = append(objs, int32(3))
	for _, c := range calls {
		kind, target, _ := strings.Cut(c, ":")
		switch kind {
		case "microflow":
			objs = append(objs, bson.D{
				{Key: "$Type", Value: "Microflows$ActionActivity"},
				{Key: "Action", Value: bson.D{
					{Key: "$Type", Value: "Microflows$MicroflowCallAction"},
					{Key: "MicroflowCall", Value: bson.D{
						{Key: "$Type", Value: "Microflows$MicroflowCall"},
						{Key: "Microflow", Value: target},
					}},
				}},
			})
		case "rule":
			objs = append(objs, bson.D{
				{Key: "$Type", Value: "Microflows$ExclusiveSplit"},
				{Key: "SplitCondition", Value: bson.D{
					{Key: "$Type", Value: "Microflows$RuleSplitCondition"},
					{Key: "RuleCall", Value: bson.D{
						{Key: "$Type", Value: "Microflows$RuleCall"},
						{Key: "Microflow", Value: target},
					}},
				}},
			})
		}
	}
	doc := bson.D{
		{Key: "$Type", Value: typ},
		{Key: "Name", Value: name},
		{Key: "Excluded", Value: excluded},
		{Key: "ApplyEntityAccess", Value: ea},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: objs},
		}},
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		panic(err)
	}
	f.units = append(f.units, &types.RawUnit{
		ID: nextID("mf"), ContainerID: eaCallsModule.ID, Type: typ, Contents: raw,
	})
}

func (f *eaCallsFixture) check(t *testing.T, src string) []linter.Violation {
	t.Helper()
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{eaCallsModule}, nil },
		ListRawUnitsByTypeFunc: func(prefix string) ([]*types.RawUnit, error) {
			var out []*types.RawUnit
			for _, u := range f.units {
				if strings.HasPrefix(u.Type, prefix) {
					out = append(out, u)
				}
			}
			return out, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(eaCallsModule)))
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, v := range CheckEntityAccessCalls(ctx, prog) {
		if v.RuleID == entityAccessCallRule {
			out = append(out, v)
		}
	}
	return out
}

const eaCaller = `mdl 1;
@applyentityaccess(false)
create or modify microflow MyModule.DS_Tile_Count () returns Integer
begin
  $List = call microflow MyModule.SUB_Scoped_List();
  return 1;
end;`

const eaCallee = `@applyentityaccess(true)
create microflow MyModule.SUB_Scoped_List () returns List of MyModule.Entity
begin
  retrieve $L from MyModule.Entity;
  return $L;
end;
`

func wantOneCE0114(t *testing.T, got []linter.Violation, kind, caller, callee string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want 1 %s violation, got %d: %+v", entityAccessCallRule, len(got), got)
	}
	v := got[0]
	if v.Severity != linter.SeverityError {
		t.Errorf("mxbuild refuses this (CE0114); want error, got %v", v.Severity)
	}
	for _, want := range []string{
		caller, callee, "CE0114",
		"A microflow that does not apply entity access can only call " + kind + "s that also do not apply entity access.",
	} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message lacks %q: %s", want, v.Message)
		}
	}
	if v.Location.DocumentName != splitQualifiedName(caller).Name {
		t.Errorf("located at %q, want the caller %s", v.Location.DocumentName, caller)
	}
}

// The reporter's first shape: the callee is already in the project.
func TestEntityAccessCalls_CalleeInProject(t *testing.T) {
	f := &eaCallsFixture{}
	f.addStored("Microflows$Microflow", "SUB_Scoped_List", true, false)
	wantOneCE0114(t, f.check(t, eaCaller), "microflow", "MyModule.DS_Tile_Count", "MyModule.SUB_Scoped_List")
}

// The reporter's second shape: the callee is created earlier in the same script.
func TestEntityAccessCalls_CalleeEarlierInScript(t *testing.T) {
	f := &eaCallsFixture{}
	src := strings.Replace(eaCaller, "mdl 1;\n", "mdl 1;\n"+eaCallee, 1)
	wantOneCE0114(t, f.check(t, src), "microflow", "MyModule.DS_Tile_Count", "MyModule.SUB_Scoped_List")
}

// Controls: every other combination builds at 0 errors (measured).
func TestEntityAccessCalls_AllowedDirectionsAreClean(t *testing.T) {
	for name, tc := range map[string]struct {
		calleeEA  bool
		callerAnn string
	}{
		"caller on, callee off": {false, "@applyentityaccess(true)"},
		"both on":               {true, "@applyentityaccess(true)"},
		"both off":              {false, "@applyentityaccess(false)"},
	} {
		f := &eaCallsFixture{}
		f.addStored("Microflows$Microflow", "SUB_Scoped_List", tc.calleeEA, false)
		src := strings.Replace(eaCaller, "@applyentityaccess(false)", tc.callerAnn, 1)
		if got := f.check(t, src); len(got) != 0 {
			t.Errorf("%s: mxbuild accepts this; got %+v", name, got)
		}
	}
}

// A callee neither the project nor the script holds cannot be judged: silent.
func TestEntityAccessCalls_UnresolvedCalleeIsSilent(t *testing.T) {
	f := &eaCallsFixture{}
	if got := f.check(t, eaCaller); len(got) != 0 {
		t.Errorf("callee could not be established; got %+v", got)
	}
}

// A decision calling a rule with entity access is the same CE with "rules".
func TestEntityAccessCalls_RuleSplit(t *testing.T) {
	f := &eaCallsFixture{}
	f.addStored("Microflows$Rule", "R_Scoped", true, false)
	got := f.check(t, `mdl 1;
@applyentityaccess(false)
create microflow MyModule.UsesRule ($X: Integer)
begin
  if MyModule.R_Scoped(X = $X) then
    log info node 'T' 'big';
  end if;
  return;
end;`)
	wantOneCE0114(t, got, "rule", "MyModule.UsesRule", "MyModule.R_Scoped")

	// A rule created earlier in the script is a rule call too.
	f = &eaCallsFixture{}
	got = f.check(t, `mdl 1;
@applyentityaccess
create rule MyModule.R_New (X: Integer) returns Boolean
begin
  return $X > 1;
end;
@applyentityaccess(false)
create microflow MyModule.UsesRule ($X: Integer)
begin
  if MyModule.R_New(X = $X) then
    log info node 'T' 'big';
  end if;
  return;
end;`)
	wantOneCE0114(t, got, "rule", "MyModule.UsesRule", "MyModule.R_New")
}

// Measured clean: a nanoflow caller (it has no such setting) and an excluded
// microflow caller.
func TestEntityAccessCalls_NanoflowAndExcludedCallersAreClean(t *testing.T) {
	f := &eaCallsFixture{}
	f.addStored("Microflows$Microflow", "SUB_Scoped_List", true, false)
	if got := f.check(t, `mdl 1;
create nanoflow MyModule.NF ()
begin
  $List = call microflow MyModule.SUB_Scoped_List();
  return;
end;`); len(got) != 0 {
		t.Errorf("nanoflow caller: got %+v", got)
	}
	src := strings.Replace(eaCaller, "@applyentityaccess(false)", "@excluded\n@applyentityaccess(false)", 1)
	if got := f.check(t, src); len(got) != 0 {
		t.Errorf("excluded caller: got %+v", got)
	}
}

// An absent annotation on `create or modify` keeps the stored setting
// (carriedApplyEntityAccess), so a stored caller WITH entity access stays clean.
func TestEntityAccessCalls_AbsentAnnotationCarriesStored(t *testing.T) {
	f := &eaCallsFixture{}
	f.addStored("Microflows$Microflow", "SUB_Scoped_List", true, false)
	f.addStored("Microflows$Microflow", "DS_Tile_Count", true, false)
	src := strings.Replace(eaCaller, "@applyentityaccess(false)\n", "", 1)
	if got := f.check(t, src); len(got) != 0 {
		t.Errorf("stored caller applies entity access and keeps it; got %+v", got)
	}
	// Without a stored caller the new microflow gets the default, off.
	f = &eaCallsFixture{}
	f.addStored("Microflows$Microflow", "SUB_Scoped_List", true, false)
	wantOneCE0114(t, f.check(t, src), "microflow", "MyModule.DS_Tile_Count", "MyModule.SUB_Scoped_List")
}

// The script turning entity access ON for a stored callee breaks a stored
// caller it does not mention; the report is on that caller.
func TestEntityAccessCalls_ScriptChangesCallee(t *testing.T) {
	f := &eaCallsFixture{}
	f.addStored("Microflows$Microflow", "SUB_Scoped_List", false, false)
	f.addStored("Microflows$Microflow", "DS_Tile_Count", false, false, "microflow:MyModule.SUB_Scoped_List")
	got := f.check(t, "mdl 1;\n"+strings.Replace(eaCallee, "create microflow", "create or modify microflow", 1))
	wantOneCE0114(t, got, "microflow", "MyModule.DS_Tile_Count", "MyModule.SUB_Scoped_List")
}

// A project that already has the defect, untouched by the script, is not a
// verdict on the script.
func TestEntityAccessCalls_PreexistingNotReported(t *testing.T) {
	f := &eaCallsFixture{}
	f.addStored("Microflows$Microflow", "SUB_Scoped_List", true, false)
	f.addStored("Microflows$Microflow", "DS_Tile_Count", false, false, "microflow:MyModule.SUB_Scoped_List")
	if got := f.check(t, "mdl 1;\ncreate microflow MyModule.Other () begin end;"); len(got) != 0 {
		t.Errorf("pre-existing CE0114 reported against an unrelated script: %+v", got)
	}
}

// alter microflow inserting the call into a stored caller without entity access.
func TestEntityAccessCalls_AlterInsertsCall(t *testing.T) {
	f := &eaCallsFixture{}
	f.addStored("Microflows$Microflow", "SUB_Scoped_List", true, false)
	f.addStored("Microflows$Microflow", "DS_Tile_Count", false, false)
	got := f.check(t, `mdl 1;
alter microflow MyModule.DS_Tile_Count {
  insert before return {
    $List = call microflow MyModule.SUB_Scoped_List();
  }
};`)
	wantOneCE0114(t, got, "microflow", "MyModule.DS_Tile_Count", "MyModule.SUB_Scoped_List")
}
