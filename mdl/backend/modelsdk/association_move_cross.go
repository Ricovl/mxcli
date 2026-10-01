// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A MOVE ENTITY has to look at the cross-associations that already exist, not
// only at the plain associations it converts (ako/mxcli#628). A cross-association
// lives in the unit of its FROM entity, holds that entity by id (ParentPointer)
// and the TO entity by qualified name (Child). Moving entity E from S to T leaves
// three shapes that are not the same case:
//
//  1. Both endpoints end up in one module. The cross-association becomes a plain
//     DomainModels$Association again in that module — the inverse of
//     crossAssocRawFromAssoc. This is what moving the second endpoint of an
//     association across, one entity at a time, produces.
//  2. E is the FROM entity and the TO entity is elsewhere. The cross-association
//     travels with E to T (its ParentPointer would not resolve in S), so its
//     qualified name changes S.X -> T.X.
//  3. E is the TO entity of a cross-association in ANOTHER module. That module's
//     unit names `S.E`; it is re-pointed to `T.E`.
//
// Left alone, shape 1 and 2 leave a ParentPointer naming an element absent from
// its unit — Studio Pro fails to load the project ("The given key was not present
// in the dictionary") — and shape 3 leaves a dangling qualified name. Every
// conversion carries the stored document, GUID included: the runtime keys the
// association's data on that GUID (CLAUDE.md, "A GUID Is the Database's Identity").

// moveOwnCrossAssociations handles shapes 1 and 2: the cross-associations in the
// source unit whose FROM entity is the moved one. It removes them from sourceDM
// and adds them to targetDM, as plain associations when their TO entity lives in
// the target module.
func moveOwnCrossAssociations(sourceDM, targetDM *genDm.DomainModel, entityID model.ID, sourceModuleName, targetModuleName string) []types.MovedAssociation {
	targetEntities := entityIDsByName(targetDM)
	var out []types.MovedAssociation
	var removeIdx []int
	for i, el := range sourceDM.CrossAssociationsItems() {
		ca, ok := el.(*genDm.CrossAssociation)
		if !ok || string(ca.ParentRefID()) != string(entityID) {
			continue
		}
		moved := types.MovedAssociation{
			Name:             ca.Name(),
			OldQualifiedName: sourceModuleName + "." + ca.Name(),
			NewQualifiedName: targetModuleName + "." + ca.Name(),
		}
		mod, ent, _ := strings.Cut(ca.ChildQualifiedName(), ".")
		if toID, ok := targetEntities[ent]; ok && mod == targetModuleName {
			targetDM.AddAssociations(assocFromGenCrossAssoc(ca, toID))
			moved.SameModule = true
		} else {
			targetDM.AddCrossAssociations(cloneCrossAssoc(ca))
		}
		removeIdx = append(removeIdx, i)
		out = append(out, moved)
	}
	for i := len(removeIdx) - 1; i >= 0; i-- {
		sourceDM.RemoveCrossAssociations(removeIdx[i])
	}
	return out
}

// convertIncomingCrossAssociations handles shape 1 seen from the other side: a
// cross-association in the TARGET unit whose TO entity is the moved one. Both
// endpoints are now in the target module, so it becomes a plain association
// there. Its qualified name does not change.
func convertIncomingCrossAssociations(targetDM *genDm.DomainModel, entityID model.ID, oldEntityQN, targetModuleName string) []types.MovedAssociation {
	var out []types.MovedAssociation
	var removeIdx []int
	for i, el := range targetDM.CrossAssociationsItems() {
		ca, ok := el.(*genDm.CrossAssociation)
		if !ok || ca.ChildQualifiedName() != oldEntityQN {
			continue
		}
		targetDM.AddAssociations(assocFromGenCrossAssoc(ca, string(entityID)))
		removeIdx = append(removeIdx, i)
		qn := targetModuleName + "." + ca.Name()
		out = append(out, types.MovedAssociation{Name: ca.Name(), OldQualifiedName: qn, NewQualifiedName: qn, SameModule: true})
	}
	for i := len(removeIdx) - 1; i >= 0; i-- {
		targetDM.RemoveCrossAssociations(removeIdx[i])
	}
	return out
}

// repointCrossAssociationsElsewhere handles shape 3: every cross-association in a
// domain model other than the source and target units that names the moved entity
// as its TO entity is re-pointed to the entity's new qualified name.
func (b *Backend) repointCrossAssociationsElsewhere(sourceDMID, targetDMID model.ID, oldEntityQN, newEntityQN string) error {
	dms, err := b.ListDomainModels()
	if err != nil {
		return fmt.Errorf("list domain models: %w", err)
	}
	for _, info := range dms {
		if info.ID == sourceDMID || info.ID == targetDMID || string(info.ID) == meta.SystemDomainModelID {
			continue
		}
		gdm, err := b.loadDomainModelGen(info.ID)
		if err != nil {
			return err
		}
		changed := false
		for _, el := range gdm.CrossAssociationsItems() {
			if ca, ok := el.(*genDm.CrossAssociation); ok && ca.ChildQualifiedName() == oldEntityQN {
				ca.SetChildQualifiedName(newEntityQN)
				changed = true
			}
		}
		if changed {
			if err := b.persistDM(info.ID, gdm); err != nil {
				return err
			}
		}
	}
	return nil
}

func entityIDsByName(dm *genDm.DomainModel) map[string]string {
	out := map[string]string{}
	for _, el := range dm.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok {
			out[e.Name()] = string(e.ID())
		}
	}
	return out
}

// cloneCrossAssoc re-homes a stored cross-association in another unit unchanged:
// a clean element over the stored bytes, so the encoder passes the whole document
// — GUID included — through verbatim.
func cloneCrossAssoc(ca *genDm.CrossAssociation) *genDm.CrossAssociation {
	if raw := ca.Raw(); raw != nil {
		out := genDm.NewCrossAssociation()
		out.SetRaw(raw)
		out.InitFromRaw(raw)
		out.SetID(ca.ID())
		return out
	}
	return ca
}

// assocFromGenCrossAssoc converts a cross-association back into the plain
// association both of whose endpoints now share its unit; childID is the TO
// entity's element id. Like the forward conversion it prefers a raw transform of
// the stored document, so the GUID and every untouched property survive; the
// property build is the fallback for a cross-association never persisted.
func assocFromGenCrossAssoc(ca *genDm.CrossAssociation, childID string) *genDm.Association {
	if raw, ok := assocRawFromCrossAssoc(ca, childID); ok {
		out := genDm.NewAssociation()
		out.SetRaw(raw)
		out.InitFromRaw(raw)
		out.SetID(ca.ID())
		return out
	}

	out := genDm.NewAssociation()
	out.SetID(ca.ID())
	out.SetName(ca.Name())
	out.SetDocumentation(ca.Documentation())
	out.SetExportLevel(orDefault(ca.ExportLevel(), "Hidden"))
	out.SetParentID(ca.ParentRefID())
	out.SetChildID(element.ID(childID))
	out.SetType(ca.Type())
	out.SetOwner(ca.Owner())
	out.SetStorageFormat(orDefault(ca.StorageFormat(), "Column"))
	out.SetParentConnection(domainmodel.DefaultParentConnection)
	out.SetChildConnection(domainmodel.DefaultChildConnection)
	pdb, cdb := "DeleteMeButKeepReferences", "DeleteMeButKeepReferences"
	if odb, ok := ca.DeleteBehavior().(*genDm.AssociationDeleteBehavior); ok {
		pdb, cdb = orDefault(odb.ParentDeleteBehavior(), pdb), orDefault(odb.ChildDeleteBehavior(), cdb)
	}
	out.SetDeleteBehavior(deleteBehaviorToGen(pdb, cdb))
	if src, ok := ca.Source().(*genDm.OqlViewAssociationSource); ok && src != nil {
		out.SetSource(oqlViewAssociationSourceToGen(src.Reference()))
	}
	assignID(out.DeleteBehavior())
	return out
}

// assocRawFromCrossAssoc is the inverse of crossAssocRawFromAssoc: $Type becomes
// DomainModels$Association, Child (a qualified name) becomes ChildPointer (the TO
// entity's 16-byte id, resolvable now that it shares the unit), and the two
// connection points a plain association declares — the line's on-canvas anchors —
// are added with the defaults mxcli and Studio Pro write for a new association.
// Everything else, the GUID above all, passes through. The key set is the one
// `generated/metamodel` declares for DomainModelsAssociation.
func assocRawFromCrossAssoc(ca *genDm.CrossAssociation, childID string) (bson.Raw, bool) {
	raw := ca.Raw()
	if raw == nil {
		return nil, false
	}
	elems, err := bsoncore.Document(raw).Elements()
	if err != nil {
		return nil, false
	}
	out := make(bson.D, 0, len(elems)+2)
	child, parent := false, false
	for _, e := range elems {
		switch e.Key() {
		case "$Type":
			out = append(out, bson.E{Key: "$Type", Value: "DomainModels$Association"})
		case "Child":
			out = append(out,
				bson.E{Key: "ChildConnection", Value: domainmodel.DefaultChildConnection},
				bson.E{Key: "ChildPointer", Value: mmpr.IDToBsonBinary(childID)})
			child = true
		case "ParentPointer":
			v := e.Value()
			out = append(out,
				bson.E{Key: "ParentConnection", Value: domainmodel.DefaultParentConnection},
				bson.E{Key: "ParentPointer", Value: bson.RawValue{Type: bson.Type(v.Type), Value: v.Data}})
			parent = true
		case "ChildConnection", "ParentConnection":
			// Not declared on a cross-association; never carry a stray one twice.
		default:
			v := e.Value()
			out = append(out, bson.E{Key: e.Key(), Value: bson.RawValue{Type: bson.Type(v.Type), Value: v.Data}})
		}
	}
	if !child || !parent {
		return nil, false
	}
	b, err := bson.Marshal(out)
	if err != nil {
		return nil, false
	}
	return bson.Raw(b), true
}
