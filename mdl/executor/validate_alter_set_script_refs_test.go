// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// actionDeps serializes any action to a placeholder so the setter succeeds
// once the target resolves; the test is about resolution, not the BSON.
type actionDeps struct{ countingDeps }

func (d *actionDeps) SerializeClientAction(pages.ClientAction) bson.D {
	return bson.D{{Key: "$Type", Value: "Forms$NoAction"}}
}

func storedButtonPage() bson.D {
	btn := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnGo"},
		{Key: "Action", Value: bson.D{{Key: "$Type", Value: "Forms$NoAction"}}},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), btn}}},
			}},
		}},
	}
}

func buttonPageCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "P_Btn")
	deps := &actionDeps{}
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:    func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:      func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return nil, nil },
		ListNanoflowsFunc:  func() ([]*microflows.Nanoflow, error) { return nil, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedButtonPage(), unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

// TestAlterSet_ActionTargetsCreatedInScript is ako/mxcli#969 item 2: a SET
// naming a microflow, nanoflow or page the same script creates was "not found"
// in check, because the dry run resolves against the stored project and the
// session cache exec would use is never filled by check.
func TestAlterSet_ActionTargetsCreatedInScript(t *testing.T) {
	for name, src := range map[string]string{
		"microflow": `create microflow MyModule.ACT_New () begin end;
alter page MyModule.P_Btn { set Action = microflow MyModule.ACT_New on btnGo };`,
		"nanoflow": `create nanoflow MyModule.NAV_New () begin end;
alter page MyModule.P_Btn { set Action = nanoflow MyModule.NAV_New on btnGo };`,
		"page": `create page MyModule.P_New (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dynamictext t1 (Content: 'x') };
alter page MyModule.P_Btn { set Action = show page MyModule.P_New on btnGo };`,
	} {
		t.Run(name, func(t *testing.T) {
			if errs := checkAlterSet(t, buttonPageCtx(t), src); len(errs) != 0 {
				t.Errorf("script-created %s reported missing: %v", name, errs)
			}
		})
	}
}

// TestAlterSet_ActionTargetsMissing is the control: a target neither stored
// nor created by the script is still refused.
func TestAlterSet_ActionTargetsMissing(t *testing.T) {
	for name, src := range map[string]string{
		"microflow": `alter page MyModule.P_Btn { set Action = microflow MyModule.ACT_Missing on btnGo };`,
		"nanoflow":  `alter page MyModule.P_Btn { set Action = nanoflow MyModule.NAV_Missing on btnGo };`,
		"page":      `alter page MyModule.P_Btn { set Action = show page MyModule.P_Missing on btnGo };`,
	} {
		t.Run(name, func(t *testing.T) {
			errs := checkAlterSet(t, buttonPageCtx(t), src)
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "not found") {
				t.Errorf("missing %s: got %v, want one not-found error", name, errs)
			}
		})
	}
}
