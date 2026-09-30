// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// CreateNanoflow inserts a new Microflows$Nanoflow document unit. A nanoflow
// shares the microflow flow model (parameters + object collection + sequence
// flows), so it reuses the microflow object/flow converters; only the top-level
// field set differs (no ExportLevel / concurrency / URL fields).
func (b *Backend) CreateNanoflow(nf *microflows.Nanoflow) error {
	if nf == nil {
		return fmt.Errorf("CreateNanoflow: nil nanoflow")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateNanoflow: not connected for writing")
	}
	if nf.ID == "" {
		nf.ID = model.ID(mmpr.GenerateID())
	}
	g := nanoflowToGen(nf, b.majorVersion())
	g.SetID(element.ID(nf.ID))
	assignNanoflowIDs(g)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateNanoflow: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(nf.ID), string(nf.ContainerID), "Documents", "Microflows$Nanoflow", contents); err != nil {
		return fmt.Errorf("CreateNanoflow: insert: %w", err)
	}
	return nil
}

// UpdateNanoflow rebuilds a nanoflow document (the CREATE OR REPLACE path).
func (b *Backend) UpdateNanoflow(nf *microflows.Nanoflow) error {
	if nf == nil {
		return fmt.Errorf("UpdateNanoflow: nil nanoflow")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateNanoflow: not connected for writing")
	}
	g := nanoflowToGen(nf, b.majorVersion())
	g.SetID(element.ID(nf.ID))
	assignNanoflowIDs(g)
	b.carryStoredNanoflowHeader(nf, g)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateNanoflow: encode: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(nf.ID), contents); err != nil {
		return fmt.Errorf("UpdateNanoflow: update: %w", err)
	}
	return nil
}

// DeleteNanoflow removes the nanoflow unit.
func (b *Backend) DeleteNanoflow(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteNanoflow: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// nanoflowToGen builds a gen Nanoflow from the model, mirroring the legacy
// serializeNanoflow field set (AllowedModuleRoles, Documentation, Excluded,
// Flows, MarkAsUsed, MicroflowReturnType [only when non-void], Name,
// ObjectCollection with parameters merged in first).
func nanoflowToGen(nf *microflows.Nanoflow, major int) *genMf.Nanoflow {
	out := genMf.NewNanoflow()
	out.SetName(nf.Name)
	out.SetDocumentation(nf.Documentation)
	out.SetExcluded(nf.Excluded)
	out.SetMarkAsUsed(nf.MarkAsUsed)
	out.SetAllowedModuleRolesQualifiedNames(moduleRoleNames(nf.AllowedModuleRoles))
	if nf.ReturnType != nil {
		out.SetMicroflowReturnType(microflowDataTypeToGen(nf.ReturnType))
	}
	// Only when authored; UpdateNanoflow carries a stored one otherwise.
	if nf.ReturnVariableName != "" {
		out.SetReturnVariableName(nf.ReturnVariableName)
	}

	oc := genMf.NewMicroflowObjectCollection()
	for i, p := range nf.Parameters {
		oc.AddObjects(microflowParameterToGen(p, i, major))
	}
	if nf.ObjectCollection != nil {
		for _, obj := range nf.ObjectCollection.Objects {
			if g := microflowObjectToGen(obj); g != nil {
				oc.AddObjects(g)
			}
		}
	}
	out.SetObjectCollection(oc)

	// Sequence flows and annotation flows share the Flows list, as on a
	// microflow. The annotation flows were not written at all, so every
	// rewrite detached each note from the activity it documents
	// (ako/mxcli#705).
	if nf.ObjectCollection != nil {
		for _, f := range nf.ObjectCollection.Flows {
			out.AddFlows(sequenceFlowToGen(f, major))
		}
		for _, af := range nf.ObjectCollection.AnnotationFlows {
			out.AddFlows(annotationFlowToGen(af, major))
		}
	}
	return out
}

// carryStoredNanoflowHeader copies onto a rebuilt nanoflow the header keys the
// statement did not author: ExportLevel and UseListParameterByReference, which
// MDL has no spelling for, and ReturnVariableName unless the statement gave
// `returns T as $Var`. A rebuild used to omit all three, so describe -> exec
// deleted them (ako/mxcli#705).
//
// Only a key the stored document carries is written. The nanoflow writer has
// never emitted these on a new document, and Studio Pro fills an absent one on
// load — while a key the project's metamodel does not declare makes the
// document unopenable. Carrying what is there is safe at every version.
func (b *Backend) carryStoredNanoflowHeader(nf *microflows.Nanoflow, g *genMf.Nanoflow) {
	if b.reader == nil || nf.ID == "" {
		return
	}
	raw, err := b.reader.GetRawUnitBytes(string(nf.ID))
	if err != nil {
		return
	}
	var stored bson.D
	if err := bson.Unmarshal(raw, &stored); err != nil {
		return
	}
	for _, e := range stored {
		switch e.Key {
		case "ExportLevel":
			if v, ok := e.Value.(string); ok && v != "" {
				g.SetExportLevel(v)
			}
		case "UseListParameterByReference":
			if v, ok := e.Value.(bool); ok {
				g.SetUseListParameterByReference(v)
			}
		case "ReturnVariableName":
			if v, ok := e.Value.(string); ok && nf.ReturnVariableName == "" {
				g.SetReturnVariableName(v)
			}
		}
	}
}

// assignNanoflowIDs assigns fresh IDs to the nanoflow's return type, object
// collection, and sequence flows (parallels assignMicroflowIDs minus the
// microflow-only concurrency-error message).
func assignNanoflowIDs(n *genMf.Nanoflow) {
	if rt := n.MicroflowReturnType(); rt != nil {
		assignID(rt)
	}
	if oc, ok := n.ObjectCollection().(*genMf.MicroflowObjectCollection); ok {
		assignObjectCollectionIDs(oc)
	}
	for _, el := range n.FlowsItems() {
		assignID(el)
		if sf, ok := el.(*genMf.SequenceFlow); ok {
			for _, cv := range sf.CaseValuesItems() {
				assignID(cv)
			}
			assignID(sf.Line())
		}
	}
}
