// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// eventHandlerTestCtx builds an ExecContext over M.E carrying the given stored
// handlers, with microflows M.First, M.Second and M.Third resolvable. It returns
// the handlers the last UpdateEntity wrote (nil when nothing was written).
func eventHandlerTestCtx(t *testing.T, stored ...*domainmodel.EventHandler) (*ExecContext, *[]*domainmodel.EventHandler) {
	t.Helper()
	mod := mkModule("M")
	ent := &domainmodel.Entity{
		BaseElement:   model.BaseElement{ID: nextID("ent")},
		ContainerID:   nextID("dm"),
		Name:          "E",
		Persistable:   true,
		EventHandlers: stored,
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{ent},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, ent.ContainerID, dm.ID)
	mfs := []*microflows.Microflow{
		mkMicroflow(mod.ID, "First"),
		mkMicroflow(mod.ID, "Second"),
		mkMicroflow(mod.ID, "Third"),
	}

	written := new([]*domainmodel.EventHandler)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return mfs, nil },
		UpdateEntityFunc: func(dmID model.ID, e *domainmodel.Entity) error {
			*written = append([]*domainmodel.EventHandler{}, e.EventHandlers...)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, written
}

func storedHandler(moment, event, mf string) *domainmodel.EventHandler {
	return &domainmodel.EventHandler{
		BaseElement:     model.BaseElement{ID: nextID("eh")},
		Moment:          domainmodel.EventMoment(moment),
		Event:           domainmodel.EventType(event),
		MicroflowName:   mf,
		PassEventObject: true,
	}
}

func handlerMicroflows(ehs []*domainmodel.EventHandler) []string {
	out := make([]string, 0, len(ehs))
	for _, eh := range ehs {
		out = append(out, eh.MicroflowName)
	}
	return out
}

func addHandlerStmt(mf string, ifNotExists bool) *ast.AlterEntityStmt {
	return &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "M", Name: "E"},
		Operation: ast.AlterEntityAddEventHandler,
		EventHandler: &ast.EventHandlerDef{
			Moment:          "Before",
			Event:           "Commit",
			Microflow:       ast.QualifiedName{Module: "M", Name: mf},
			PassEventObject: true,
		},
		IfNotExists: ifNotExists,
	}
}

func dropHandlerStmt(mf string, ifExists bool) *ast.AlterEntityStmt {
	eh := &ast.EventHandlerDef{Moment: "Before", Event: "Commit"}
	if mf != "" {
		eh.Microflow = ast.QualifiedName{Module: "M", Name: mf}
	}
	return &ast.AlterEntityStmt{
		Name:         ast.QualifiedName{Module: "M", Name: "E"},
		Operation:    ast.AlterEntityDropEventHandler,
		EventHandler: eh,
		IfExists:     ifExists,
	}
}

func outputOf(ctx *ExecContext) string {
	return ctx.Output.(interface{ String() string }).String()
}

// mendixlabs/mxcli#1540: "ALTER ENTITY M.E ADD EVENT HANDLER ON BEFORE COMMIT
// CALL M.Second($currentObject) fails with "event handler already exists for
// Before Commit" when the entity already has one." Studio Pro allows several
// handlers for one event, and CREATE OR MODIFY already stores both.
func TestAlterEntityAddSecondHandlerForSameEvent(t *testing.T) {
	ctx, written := eventHandlerTestCtx(t, storedHandler("Before", "Commit", "M.First"))
	err := execAlterEntity(ctx, addHandlerStmt("Second", false))
	assertNoError(t, err)
	got := handlerMicroflows(*written)
	if strings.Join(got, ",") != "M.First,M.Second" {
		t.Fatalf("handlers written = %v, want [M.First M.Second] (the stored one kept, the new one appended)", got)
	}
}

// The identity of a handler is its moment, event AND microflow: the same
// microflow on the same event twice is still a duplicate, and still refused.
func TestAlterEntityAddSameHandlerTwiceStillErrors(t *testing.T) {
	ctx, written := eventHandlerTestCtx(t, storedHandler("Before", "Commit", "M.First"))
	err := execAlterEntity(ctx, addHandlerStmt("First", false))
	if err == nil {
		t.Fatal("expected an already-exists error for the same microflow on the same event")
	}
	if !strings.Contains(err.Error(), "M.First") || !strings.Contains(err.Error(), "IF NOT EXISTS") {
		t.Errorf("error should name the microflow and the guard, got: %v", err)
	}
	if *written != nil {
		t.Error("expected no UpdateEntity for a refused duplicate")
	}
}

// IF NOT EXISTS skips only when THIS handler is present. It used to skip when
// any handler for the event was present — reporting "already exists" for a
// microflow that was never wired.
func TestAlterEntityAddHandlerIfNotExistsKeysOnMicroflow(t *testing.T) {
	ctx, written := eventHandlerTestCtx(t, storedHandler("Before", "Commit", "M.First"))
	assertNoError(t, execAlterEntity(ctx, addHandlerStmt("Second", true)))
	if got := handlerMicroflows(*written); strings.Join(got, ",") != "M.First,M.Second" {
		t.Fatalf("handlers written = %v, want [M.First M.Second]", got)
	}

	ctx2, written2 := eventHandlerTestCtx(t, storedHandler("Before", "Commit", "M.First"))
	assertNoError(t, execAlterEntity(ctx2, addHandlerStmt("First", true)))
	if *written2 != nil {
		t.Error("expected no UpdateEntity when the same handler is already present")
	}
	if out := outputOf(ctx2); !strings.Contains(out, "already exists") || !strings.Contains(out, "M.First") {
		t.Errorf("expected a skip notice naming M.First, got: %q", out)
	}
}

// With several handlers on one event, DROP … CALL M.X removes that one only.
func TestAlterEntityDropHandlerByMicroflow(t *testing.T) {
	ctx, written := eventHandlerTestCtx(t,
		storedHandler("Before", "Commit", "M.First"),
		storedHandler("Before", "Commit", "M.Second"),
		storedHandler("Before", "Commit", "M.Third"))
	assertNoError(t, execAlterEntity(ctx, dropHandlerStmt("Second", false)))
	if got := handlerMicroflows(*written); strings.Join(got, ",") != "M.First,M.Third" {
		t.Fatalf("handlers written = %v, want [M.First M.Third]", got)
	}
}

// A bare DROP that matches several handlers cannot know which one is meant.
// Taking the first one would drop a handler the author may have wanted kept,
// so it is refused, and the error names the candidates and the CALL form.
func TestAlterEntityBareDropAmbiguousIsRefused(t *testing.T) {
	ctx, written := eventHandlerTestCtx(t,
		storedHandler("Before", "Commit", "M.First"),
		storedHandler("Before", "Commit", "M.Second"))
	err := execAlterEntity(ctx, dropHandlerStmt("", false))
	if err == nil {
		t.Fatal("expected a bare DROP over two Before Commit handlers to be refused")
	}
	for _, want := range []string{"M.First", "M.Second", "call"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
	if *written != nil {
		t.Error("expected no UpdateEntity for an ambiguous drop")
	}

	// IF EXISTS answers "is it absent?", not "which one?" — still refused.
	ctx2, _ := eventHandlerTestCtx(t,
		storedHandler("Before", "Commit", "M.First"),
		storedHandler("Before", "Commit", "M.Second"))
	if err := execAlterEntity(ctx2, dropHandlerStmt("", true)); err == nil {
		t.Fatal("expected an ambiguous DROP IF EXISTS to be refused too")
	}
}

// The single-handler case keeps working exactly as before, including the
// handler on another event being left alone.
func TestAlterEntityBareDropSingleHandler(t *testing.T) {
	ctx, written := eventHandlerTestCtx(t,
		storedHandler("After", "Commit", "M.Third"),
		storedHandler("Before", "Commit", "M.First"))
	assertNoError(t, execAlterEntity(ctx, dropHandlerStmt("", false)))
	if got := handlerMicroflows(*written); strings.Join(got, ",") != "M.Third" {
		t.Fatalf("handlers written = %v, want [M.Third]", got)
	}
}

// DROP … CALL M.X IF EXISTS on an absent handler skips; without the guard it
// errors — even when another handler for the same event is present.
func TestAlterEntityDropHandlerByMicroflowMissing(t *testing.T) {
	ctx, written := eventHandlerTestCtx(t, storedHandler("Before", "Commit", "M.First"))
	assertNoError(t, execAlterEntity(ctx, dropHandlerStmt("Second", true)))
	if *written != nil {
		t.Error("expected no UpdateEntity when the named handler is absent")
	}
	if out := outputOf(ctx); !strings.Contains(out, "M.Second") || !strings.Contains(out, "skipping") {
		t.Errorf("expected a skip notice naming M.Second, got: %q", out)
	}

	ctx2, _ := eventHandlerTestCtx(t, storedHandler("Before", "Commit", "M.First"))
	err := execAlterEntity(ctx2, dropHandlerStmt("Second", false))
	if err == nil {
		t.Fatal("expected a not-found error for an absent named handler")
	}
	if !strings.Contains(err.Error(), "M.Second") {
		t.Errorf("error should name M.Second, got: %v", err)
	}
}
