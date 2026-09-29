// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#793: `create` let a document take a name another KIND already uses
// in the same module — a microflow over a nanoflow, a page over a snippet, an
// enumeration over an entity — and Mendix rejects every one of them:
//
//	[CE0122] "Duplicate document name 'Dup.Same'." at Microflow 'Dup.Same', Nanoflow 'Dup.Same'
//	[CE0065] "Duplicate name 'Status' in module 'Dup'. Entities, associations
//	          and enumerations cannot share names."
//
// Measured with mx check 11.14.0 on copies of TestApp. The rule is narrower
// than "document names are unique per module": only three groups of kinds share
// a name space. A microflow and a page, constant, enumeration, java action or
// workflow of the same name check clean, and so do an entity and a microflow.
// Folders do not scope the name (a microflow in F1 and a nanoflow in F2 still
// clash) and the comparison is case-insensitive (CaseX vs casex clash).
package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ---------------------------------------------------------------------------
// Script level: two creates in one script (plain `check`, no project)
// ---------------------------------------------------------------------------

func nameClashViolations(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, v := range CheckScriptDuplicates(parseScript(t, src)) {
		if v.RuleID == "MDL-DUPNAME" {
			out = append(out, v.Message)
		}
	}
	return out
}

func TestScriptNameClash_SharedNameSpaces(t *testing.T) {
	cases := map[string]string{
		"microflow/nanoflow": `create microflow M.Same () begin end;
create nanoflow M.Same () begin end;`,
		"microflow/rule": `create microflow M.Same () begin end;
create rule M.Same (A: Decimal) returns Boolean begin return $A > 1; end;`,
		"page/snippet": `create page M.Same (Title: 'x', Layout: Atlas_Core.Atlas_Default) { };
create snippet M.Same { };`,
		"page/layout": `create layout M.Same (layouttype: 'Responsive') { placeholder Main };
create page M.Same (Title: 'x', Layout: Atlas_Core.Atlas_Default) { };`,
		"enumeration/entity": `create enumeration M.Same (A 'A');
create persistent entity M.Same (Name: String(20));`,
		"entity/association": `create persistent entity M.A (Name: String(20));
create persistent entity M.B (Name: String(20));
create association M.A from M.A to M.B;`,
		"case-insensitive": `create microflow M.CaseX () begin end;
create nanoflow M.casex () begin end;`,
		"create or modify still creates": `create microflow M.Same () begin end;
create or modify nanoflow M.Same () begin end;`,
		"folders do not scope the name": `create microflow M.Same () folder 'F1' begin end;
create nanoflow M.Same () folder 'F2' begin end;`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if got := nameClashViolations(t, src); len(got) != 1 {
				t.Fatalf("want one MDL-DUPNAME, got %d: %v", len(got), got)
			}
		})
	}
}

// CONTROL: kinds outside a shared name space may share a name — mx check is
// clean for all of these — and re-creating the same kind is MDL-DUPDEF's
// business, not this rule's.
func TestScriptNameClash_NotAClash(t *testing.T) {
	cases := map[string]string{
		"microflow/constant": `create microflow M.Same () begin end;
create constant M.Same type string default 'x';`,
		"microflow/page": `create microflow M.Same () begin end;
create page M.Same (Title: 'x', Layout: Atlas_Core.Atlas_Default) { };`,
		"microflow/enumeration": `create microflow M.Same () begin end;
create enumeration M.Same (A 'A');`,
		"entity/microflow": `create persistent entity M.Same (Name: String(20));
create microflow M.Same () begin end;`,
		"other module": `create microflow M.Same () begin end;
create nanoflow N.Same () begin end;`,
		"same kind, or modify": `create microflow M.Same () begin end;
create or modify microflow M.Same () begin end;`,
		"dropped first": `create microflow M.Same () begin end;
drop microflow M.Same;
create nanoflow M.Same () begin end;`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if got := nameClashViolations(t, src); len(got) != 0 {
				t.Fatalf("want no MDL-DUPNAME, got %v", got)
			}
		})
	}
}

func TestScriptNameClash_MessageNamesTheOtherDocument(t *testing.T) {
	got := nameClashViolations(t, `create microflow M.Same () begin end;
create nanoflow M.Same () begin end;`)
	if len(got) != 1 {
		t.Fatalf("want one violation, got %v", got)
	}
	for _, want := range []string{"nanoflow M.Same", "microflow M.Same", "CE0122"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("message %q does not mention %q", got[0], want)
		}
	}
}

// ---------------------------------------------------------------------------
// Project level: a create against what the project already holds
// ---------------------------------------------------------------------------

// setupNameClashCtx is module M holding microflow "Same" inside folder F,
// enumeration "Status", page "P", entities "A"/"B" and association "Link".
func setupNameClashCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	h := mkHierarchy(mod)
	folder := nextID("folder")
	withContainer(h, folder, mod.ID)

	a, b := mkEntity(mod.ID, "A"), mkEntity(mod.ID, "B")
	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{a, b},
		Associations: []*domainmodel.Association{mkAssociation(mod.ID, "Link", a.ID, b.ID)},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{mkMicroflow(folder, "Same")}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListRulesFunc:     func() ([]*microflows.Rule, error) { return nil, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{mkEnumeration(mod.ID, "Status", "A")}, nil
		},
		ListPagesFunc: func() ([]*pages.Page, error) {
			return []*pages.Page{{BaseElement: model.BaseElement{ID: nextID("pg")}, ContainerID: mod.ID, Name: "P"}}, nil
		},
		ListSnippetsFunc:     func() ([]*pages.Snippet, error) { return nil, nil },
		ListLayoutsFunc:      func() ([]*pages.Layout, error) { return nil, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func projectNameClashes(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	var out []string
	for _, e := range CheckProjectNameClashes(ctx, parseScript(t, src)) {
		out = append(out, e.Error())
	}
	return out
}

func TestProjectNameClash_Refused(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"nanoflow over microflow in a folder": {`create nanoflow M.Same () begin end;`, "microflow M.Same"},
		"rule over microflow, other casing":   {`create rule M.same (A: Decimal) returns Boolean begin return $A > 1; end;`, "microflow M.Same"},
		"snippet over page":                   {`create snippet M.P { };`, "page M.P"},
		"entity over enumeration":             {`create persistent entity M.status (Name: String(20));`, "enumeration M.Status"},
		"enumeration over entity":             {`create enumeration M.A (X 'X');`, "entity M.A"},
		"enumeration over association":        {`create enumeration M.Link (X 'X');`, "association M.Link"},
		"or modify of another kind":           {`create or modify nanoflow M.Same () begin end;`, "microflow M.Same"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := projectNameClashes(t, setupNameClashCtx(t), c.src)
			if len(got) != 1 || !strings.Contains(got[0], c.want) {
				t.Fatalf("want one clash naming %q, got %v", c.want, got)
			}
		})
	}
}

// CONTROL: the same fixture reports nothing for kinds outside the name space,
// for the same kind (re-create is CheckProjectConflicts' business; or modify is
// fine), and for a name the script frees first.
func TestProjectNameClash_NotRefused(t *testing.T) {
	cases := map[string]string{
		"constant beside microflow": `create constant M.Same type string default 'x';`,
		"page beside microflow":     `create page M.Same (Title: 'x', Layout: Atlas_Core.Atlas_Default) { };`,
		"microflow beside entity":   `create microflow M.A () begin end;`,
		"same kind, or modify":      `create or modify microflow M.Same () begin end;`,
		"same kind, plain":          `create microflow M.Same () begin end;`,
		"freed by a drop":           "drop microflow M.Same;\ncreate nanoflow M.Same () begin end;",
		"new name":                  `create nanoflow M.Other () begin end;`,
		"another module":            `create nanoflow N.Same () begin end;`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if got := projectNameClashes(t, setupNameClashCtx(t), src); len(got) != 0 {
				t.Fatalf("want no clash, got %v", got)
			}
		})
	}
}

// check --references reports it: CheckProjectConflicts is what `check` runs.
func TestProjectConflicts_IncludesNameClash(t *testing.T) {
	ctx := setupNameClashCtx(t)
	assertHasConflict(t, ctx, `create nanoflow M.Same () begin end;`, "microflow M.Same")
}

// ---------------------------------------------------------------------------
// Exec: the dispatch refuses before the handler writes anything
// ---------------------------------------------------------------------------

func dispatchRecorded(t *testing.T, ctx *ExecContext, src string) (bool, error) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	stmt := prog.Statements[0]
	ran := false
	r := NewRegistry()
	r.handlers[reflect.TypeOf(stmt)] = func(*ExecContext, ast.Statement) error {
		ran = true
		return nil
	}
	return ran, r.Dispatch(ctx, stmt)
}

func TestExecNameClash_Refused(t *testing.T) {
	cases := map[string]struct{ src, want, ce string }{
		"nanoflow":    {`create nanoflow M.Same () begin end;`, "microflow M.Same", "CE0122"},
		"snippet":     {`create snippet M.p { };`, "page M.P", "CE0122"},
		"entity":      {`create persistent entity M.Status (Name: String(20));`, "enumeration M.Status", "CE0065"},
		"association": {`create association M.Status from M.A to M.B;`, "enumeration M.Status", "CE0065"},
		"if not exists does not skip another kind": {`create nanoflow if not exists M.Same () begin end;`, "microflow M.Same", "CE0122"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ran, err := dispatchRecorded(t, setupNameClashCtx(t), c.src)
			if err == nil {
				t.Fatal("create was not refused")
			}
			if ran {
				t.Error("the handler ran: the refusal must come before anything is written")
			}
			for _, want := range []string{c.want, c.ce} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

// CONTROL: the same fixture lets these through to the handler.
func TestExecNameClash_NotRefused(t *testing.T) {
	for name, src := range map[string]string{
		"new name":              `create nanoflow M.Other () begin end;`,
		"other kind group":      `create constant M.Same type string default 'x';`,
		"same kind, or modify":  `create or modify microflow M.Same () begin end;`,
		"microflow over entity": `create microflow M.A () begin end;`,
	} {
		t.Run(name, func(t *testing.T) {
			ran, err := dispatchRecorded(t, setupNameClashCtx(t), src)
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !ran {
				t.Error("the handler did not run")
			}
		})
	}
}

// A project that already holds a clash — written by an older mxcli, say —
// can still have either element modified in place: `or modify` of a kind that
// has the name adds nothing. Only a create that ADDS an element is refused.
func TestNameClash_ModifyInsideAnExistingClashIsAllowed(t *testing.T) {
	ctx := setupNameClashCtx(t)
	var modID model.ID
	for id := range ctx.Cache.hierarchy.moduleIDs {
		modID = id
	}
	ctx.Backend.(*mock.MockBackend).ListNanoflowsFunc = func() ([]*microflows.Nanoflow, error) {
		return []*microflows.Nanoflow{{ContainerID: modID, Name: "Same"}}, nil
	}
	src := `create or modify microflow M.Same () begin end;`
	ran, err := dispatchRecorded(t, ctx, src)
	if err != nil || !ran {
		t.Errorf("exec refused a modify inside an existing clash: ran=%v err=%v", ran, err)
	}
	if got := projectNameClashes(t, ctx, src); len(got) != 0 {
		t.Errorf("check reported a modify inside an existing clash: %v", got)
	}
	// CONTROL: a third kind joining the clash is still refused.
	if _, err := dispatchRecorded(t, ctx, `create rule M.Same (A: Decimal) returns Boolean begin return $A > 1; end;`); err == nil {
		t.Error("a rule joining the clash was not refused")
	}
}
