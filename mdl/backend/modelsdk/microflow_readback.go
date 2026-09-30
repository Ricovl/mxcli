// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ReadBackMicroflow returns mf as the reader would return it once
// CreateMicroflow had stored it: encoded by the same writer, decoded by the
// same reader, and nothing written. What the writer defaults, and what the
// document has no property for, reads back as it would from the project, so
// the result compares with a stored microflow like for like (ako/mxcli#859).
func (b *Backend) ReadBackMicroflow(mf *microflows.Microflow) (*microflows.Microflow, error) {
	if mf == nil {
		return nil, fmt.Errorf("ReadBackMicroflow: nil microflow")
	}
	gm := microflowToGen(mf, b.majorVersion())
	gm.SetID(element.ID(readBackID(mf.ID)))
	assignMicroflowIDs(gm)
	el, err := readBack(gm)
	if err != nil {
		return nil, fmt.Errorf("ReadBackMicroflow: %w", err)
	}
	g, ok := el.(*genMf.Microflow)
	if !ok {
		return nil, fmt.Errorf("ReadBackMicroflow: decoded a %T", el)
	}
	return microflowFromGen(g, mf.ContainerID), nil
}

// ReadBackNanoflow is ReadBackMicroflow for a nanoflow (CreateNanoflow).
func (b *Backend) ReadBackNanoflow(nf *microflows.Nanoflow) (*microflows.Nanoflow, error) {
	if nf == nil {
		return nil, fmt.Errorf("ReadBackNanoflow: nil nanoflow")
	}
	g := nanoflowToGen(nf, b.majorVersion())
	g.SetID(element.ID(readBackID(nf.ID)))
	assignNanoflowIDs(g)
	el, err := readBack(g)
	if err != nil {
		return nil, fmt.Errorf("ReadBackNanoflow: %w", err)
	}
	gn, ok := el.(*genMf.Nanoflow)
	if !ok {
		return nil, fmt.Errorf("ReadBackNanoflow: decoded a %T", el)
	}
	return nanoflowFromGen(gn, nf.ContainerID), nil
}

func readBackID(id model.ID) model.ID {
	if id == "" {
		return model.ID(mmpr.GenerateID())
	}
	return id
}

func readBack(el element.Element) (element.Element, error) {
	contents, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return codec.NewDecoder(codec.DefaultRegistry).Decode(bsonv2.Raw(contents))
}
