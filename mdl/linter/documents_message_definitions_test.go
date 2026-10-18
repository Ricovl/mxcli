// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// "A message definition collection created with MDL does not appear in
// documents(), so a rule on its name (for example a required MD_ prefix) can
// never fire" (ako/mxcli#1433). Both storage shapes are covered: the
// collection (Mendix < 11.15) and the per-definition MessageDefinition2
// document that replaced it. The data transformer is the control — it was
// already listed.
func TestDocumentsIncludeMessageDefinitions(t *testing.T) {
	const mod = model.ID("mod-md")
	raw := func(id, typ, name string) *types.RawUnit {
		contents, err := bson.Marshal(bson.M{"$Type": typ, "Name": name, "Documentation": ""})
		if err != nil {
			t.Fatal(err)
		}
		return &types.RawUnit{ID: model.ID(id), ContainerID: mod, Type: typ, Contents: contents}
	}
	units := []*types.RawUnit{
		raw("u-coll", "MessageDefinitions$MessageDefinitionCollection", "Orders"),
		raw("u-md2", "MessageDefinitions$MessageDefinition2", "OrderMessage"),
		raw("u-dt", "DataTransformers$DataTransformer", "DT_Order"),
	}
	be := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: mod}, Name: "MD"}}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
		ListRulesFunc:        func() ([]*microflows.Rule, error) { return nil, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return nil, nil },
		ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return nil, nil },
		GetNavigationFunc:    func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		ListRawUnitsByTypeFunc: func(typ string) ([]*types.RawUnit, error) {
			var out []*types.RawUnit
			for _, u := range units {
				if u.Type == typ {
					out = append(out, u)
				}
			}
			return out, nil
		},
	}
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := catalog.NewBuilder(cat, be)
	b.SetFullMode(true)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}

	got := map[string]string{}
	for d := range linter.NewLintContext(cat, nil).Documents() {
		got[d.QualifiedName] = d.Kind
	}
	for qn, kind := range map[string]string{
		"MD.DT_Order":     "DATA_TRANSFORMER", // control
		"MD.Orders":       "MESSAGE_DEFINITION_COLLECTION",
		"MD.OrderMessage": "MESSAGE_DEFINITION",
	} {
		if got[qn] != kind {
			t.Errorf("documents() kind of %s = %q, want %q (all: %v)", qn, got[qn], kind, got)
		}
	}
}
