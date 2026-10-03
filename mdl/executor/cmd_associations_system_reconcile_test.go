// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `create association M.Thing_Task from M.Thing to System.WorkflowUserTask`
// wrote the association, then reconciled the TO module's access rules. System's
// domain model is virtual — no unit is stored for it — so the reconcile failed
// with "no such file" and aborted the rest of the script. System's rules cannot
// be changed by a project anyway; there is nothing to reconcile.
func TestReconcileModuleAccess_SkipsSystem(t *testing.T) {
	ctx, reconciled := reconcileRecordingAssocFixture(t, false)
	mb := ctx.Backend.(*mock.MockBackend)

	mods, _ := mb.ListModulesFunc()
	system := mkModule("System")
	systemDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: system.ID,
		Entities:    []*domainmodel.Entity{mkEntity(system.ID, "WorkflowUserTask")},
	}
	all := append(append([]*model.Module(nil), mods...), system)
	mb.ListModulesFunc = func() ([]*model.Module, error) { return all, nil }
	inner := mb.GetDomainModelFunc
	mb.GetDomainModelFunc = func(id model.ID) (*domainmodel.DomainModel, error) {
		if id == system.ID {
			return systemDM, nil
		}
		return inner(id)
	}
	ctx.Cache = nil
	withHierarchy(mkHierarchy(all...))(ctx)
	mb.ReconcileMemberAccessesFunc = func(_ model.ID, moduleName string) (int, error) {
		*reconciled = append(*reconciled, moduleName)
		if moduleName == "System" {
			return 0, fmt.Errorf("open mprcontents/…/00000000-0000-0000-0000-000000000002.mxunit: no such file or directory")
		}
		return 0, nil
	}

	assertNoError(t, reconcileModuleAccess(ctx, "System", "for new association"))
	if len(*reconciled) != 0 {
		t.Fatalf("reconciled %v; System's domain model is not stored and must be skipped", *reconciled)
	}

	// Control: an ordinary module is still reconciled.
	assertNoError(t, reconcileModuleAccess(ctx, "M", "for new association"))
	if len(*reconciled) != 1 || (*reconciled)[0] != "M" {
		t.Fatalf("reconciled %v, want [M]", *reconciled)
	}
}
