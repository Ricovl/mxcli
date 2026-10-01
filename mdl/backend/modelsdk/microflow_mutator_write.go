// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/modelsdk/version"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// OpenMicroflowForMutation loads a microflow or nanoflow unit and returns the
// shared graph splice (mfmutator) over its stored bytes. New objects and flows
// are encoded with the same converters `create microflow` uses, so a fragment
// is written exactly as the same statements would be in a create; the patched
// unit is written through the reconciling writer as a patch.
func (b *Backend) OpenMicroflowForMutation(unitID model.ID) (backend.MicroflowMutator, error) {
	if b.writer == nil {
		return nil, fmt.Errorf("OpenMicroflowForMutation: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return nil, fmt.Errorf("OpenMicroflowForMutation: load unit: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("OpenMicroflowForMutation: unmarshal: %w", err)
	}
	return mfmutator.New(d, unitID, codecMicroflowDeps{b: b})
}

// codecMicroflowDeps implements mfmutator.Deps for the modelsdk (codec) backend.
type codecMicroflowDeps struct{ b *Backend }

var _ mfmutator.Deps = codecMicroflowDeps{}
var _ mfmutator.PropertyDeclarer = codecMicroflowDeps{}

// DeclaresProperty reports whether the project's Mendix version declares key
// on a flow document of docType, from the metamodel's version data: the
// property's own type first, then Microflows$MicroflowBase, which both flow
// kinds extend (ReturnVariableName, introduced in 10.12, lives there). A
// property with no version data is declared at every version. An unknown
// project version declares nothing, so SetHeader keeps refusing (#843).
func (d codecMicroflowDeps) DeclaresProperty(docType, key string) bool {
	pv := d.b.ProjectVersion()
	if pv == nil || pv.ProductVersion == "" {
		return false
	}
	v := version.Parse(pv.ProductVersion)
	if v.IsZero() {
		return false
	}
	prop := strings.ToLower(key[:1]) + key[1:]
	for _, typ := range []string{docType, "Microflows$MicroflowBase"} {
		if info, ok := genMf.VersionInfos[typ].Properties[prop]; ok {
			return info.IsAvailableIn(v)
		}
	}
	return true
}

func (d codecMicroflowDeps) SerializeObject(obj microflows.MicroflowObject) (bson.D, error) {
	el := microflowObjectToGen(obj)
	if el == nil {
		return nil, nil
	}
	assignFlowObjectIDs(el)
	return encodeFlowElementToD(el)
}

func (d codecMicroflowDeps) SerializeSequenceFlow(f *microflows.SequenceFlow) (bson.D, error) {
	el := sequenceFlowToGen(f, d.b.majorVersion())
	assignID(el)
	if sf, ok := el.(*genMf.SequenceFlow); ok {
		for _, cv := range sf.CaseValuesItems() {
			assignID(cv)
		}
		assignID(sf.Line())
	}
	return encodeFlowElementToD(el)
}

func (d codecMicroflowDeps) SerializeAnnotationFlow(f *microflows.AnnotationFlow) (bson.D, error) {
	el := annotationFlowToGen(f, d.b.majorVersion())
	assignID(el)
	if af, ok := el.(*genMf.AnnotationFlow); ok {
		assignID(af.Line())
	}
	return encodeFlowElementToD(el)
}

// SerializeDocument encodes a declared microflow or nanoflow exactly as
// UpdateMicroflow / UpdateNanoflow would, for SetHeader to copy its header
// from (ako/mxcli#818).
func (d codecMicroflowDeps) SerializeDocument(declared any) (bson.D, error) {
	var el element.Element
	switch doc := declared.(type) {
	case *microflows.Microflow:
		gm := microflowToGen(doc, d.b.majorVersion())
		gm.SetID(element.ID(doc.ID))
		assignMicroflowIDs(gm)
		contents, err := (&codec.Encoder{}).Encode(gm)
		if err != nil {
			return nil, err
		}
		if contents, err = patchMicroflowToolboxEntries(contents, doc); err != nil {
			return nil, err
		}
		var out bson.D
		if err := bson.Unmarshal(contents, &out); err != nil {
			return nil, err
		}
		return out, nil
	case *microflows.Nanoflow:
		g := nanoflowToGen(doc, d.b.majorVersion())
		g.SetID(element.ID(doc.ID))
		assignNanoflowIDs(g)
		d.b.carryStoredNanoflowHeader(doc, g)
		el = g
	default:
		return nil, fmt.Errorf("cannot encode a %T as a flow document", declared)
	}
	return encodeFlowElementToD(el)
}

// SaveUnit writes the spliced unit as a patch: the reconciling writer elides
// an unchanged unit and guards storage GUIDs as for every write, but does not
// re-pair element $IDs the splice deliberately kept (canon.ContentsOwnElementIDs).
func (d codecMicroflowDeps) SaveUnit(unitID string, contents []byte) error {
	return d.b.writer.UpdateRawUnitPatch(unitID, contents)
}

func encodeFlowElementToD(el element.Element) (bson.D, error) {
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return nil, err
	}
	var out bson.D
	if err := bson.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
