// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ParseMicroflowFromRaw builds a Microflow from an already-unmarshalled BSON
// document that did NOT come from this project's storage — diff-local hands it
// a historical unit read back with `git show`, so there is no unit to look up.
//
// The codec decodes bytes, not maps, so the map is re-marshalled first. That is
// safe here because the result is read-only (it is rendered as MDL and thrown
// away): nothing this produces is written back, so the field reordering a map
// round-trip causes cannot reach storage.
func (b *Backend) ParseMicroflowFromRaw(raw map[string]any, unitID, containerID model.ID) *microflows.Microflow {
	data, err := bson.Marshal(raw)
	if err != nil {
		return nil
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(data))
	if err != nil {
		return nil
	}
	mf, ok := elem.(*genMf.Microflow)
	if !ok {
		return nil
	}
	out := microflowFromGen(mf, containerID)
	if out != nil && unitID != "" {
		out.ID = unitID
	}
	return out
}

// ParseNanoflowFromRaw is ParseMicroflowFromRaw for a nanoflow, under the same
// read-only reasoning. Without it diff-local rendered every nanoflow as a fixed
// header stub, so a change to its body counted as modified and diffed to
// nothing (mendixlabs/mxcli#1393).
func (b *Backend) ParseNanoflowFromRaw(raw map[string]any, unitID, containerID model.ID) *microflows.Nanoflow {
	data, err := bson.Marshal(raw)
	if err != nil {
		return nil
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(data))
	if err != nil {
		return nil
	}
	nf, ok := elem.(*genMf.Nanoflow)
	if !ok {
		return nil
	}
	out := nanoflowFromGen(nf, containerID)
	if out != nil && unitID != "" {
		out.ID = unitID
	}
	return out
}
