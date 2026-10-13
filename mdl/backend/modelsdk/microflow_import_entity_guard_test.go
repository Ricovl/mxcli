// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1418: an import-from-mapping activity stored with an empty
// VariableType.Entity makes the whole project unloadable:
//
//	System.InvalidOperationException: An error occurred when trying to set the
//	'Entity' property of a Object type in a Microflow with ID ...
//	---> System.ArgumentNullException: Value cannot be null. (Parameter 'value')
//	  at Mendix.Modeler.DataTypes.EntityType.set_EntityId(EntityIdentifier value)
//
// (mx check, 11.6.6). Whatever the caller hands it, the writer refuses rather
// than store one — including inside a loop body, and on the splice path.
func importActivity(entity model.ID) *microflows.ActionActivity {
	return &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "act-1"},
		}},
		Action: &microflows.ImportXmlAction{
			BaseElement:         model.BaseElement{ID: "imp-1"},
			XmlDocumentVariable: "P",
			ResultHandling: &microflows.ResultHandlingMapping{
				BaseElement:    model.BaseElement{ID: "rh-1"},
				MappingID:      "MyFirstModule.IMM_D",
				ResultEntityID: entity,
				ResultVariable: "R",
				SingleObject:   true,
			},
		},
	}
}

func TestWriterRefusesImportMappingWithoutResultEntity(t *testing.T) {
	inLoop := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: "loop-1"}},
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{importActivity("")},
		},
	}
	for name, obj := range map[string]microflows.MicroflowObject{
		"top level": importActivity(""),
		"in a loop": inLoop,
	} {
		t.Run(name, func(t *testing.T) {
			mf := &microflows.Microflow{
				BaseElement:      model.BaseElement{ID: "mf-1"},
				Name:             "MF_D",
				ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{obj}},
			}
			b := &Backend{}
			for op, err := range map[string]error{
				"CreateMicroflow": b.CreateMicroflow(mf),
				"UpdateMicroflow": b.UpdateMicroflow(mf),
			} {
				if err == nil || !strings.Contains(err.Error(), "MyFirstModule.IMM_D") || !strings.Contains(err.Error(), "result entity") {
					t.Errorf("%s: err = %v, want a refusal naming the mapping's missing result entity", op, err)
				}
			}
			if _, err := (codecMicroflowDeps{b: b}).SerializeObject(obj); err == nil {
				t.Errorf("SerializeObject: no error for an import activity without a result entity")
			}
		})
	}

	// Control: with the entity the guard stays out of the way.
	if err := importMappingEntityError(importActivity("MyFirstModule.CountryD")); err != nil {
		t.Fatalf("activity with a result entity refused: %v", err)
	}
}
