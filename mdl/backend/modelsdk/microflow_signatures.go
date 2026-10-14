// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ListMicroflowSignatures lists every microflow decoded without its body: name,
// container, flags, allowed roles, parameters and return type are as
// ListMicroflows reports them, but ObjectCollection holds no activities and
// there are no flows. For callers that need what a flow accepts and returns,
// not what it does — check resolves call arguments against every stored flow,
// and decoding each one whole was most of its time on a large project.
func (b *Backend) ListMicroflowSignatures() ([]*microflows.Microflow, error) {
	units, err := mprread.ListUnitsTransformed[*genMf.Microflow](b.reader, stripFlowBody)
	if err != nil {
		return nil, err
	}
	out := make([]*microflows.Microflow, 0, len(units))
	for _, u := range units {
		out = append(out, microflowFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

// ListNanoflowSignatures is ListMicroflowSignatures for nanoflows.
func (b *Backend) ListNanoflowSignatures() ([]*microflows.Nanoflow, error) {
	units, err := mprread.ListUnitsTransformed[*genMf.Nanoflow](b.reader, stripFlowBody)
	if err != nil {
		return nil, err
	}
	out := make([]*microflows.Nanoflow, 0, len(units))
	for _, u := range units {
		out = append(out, nanoflowFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

var flowParameterType, _ = codec.DefaultRegistry.TypeNameOf(reflect.TypeOf((*genMf.MicroflowParameter)(nil)))

// stripFlowBody drops a flow document's body: the top-level Flows, and every
// object in ObjectCollection.Objects except the parameters. Arrays keep their
// leading non-document version marker, so the result decodes like the stored
// document minus those elements.
func stripFlowBody(doc []byte) ([]byte, error) {
	elems, err := bsoncore.Document(doc).Elements()
	if err != nil {
		return nil, err
	}
	idx, out := bsoncore.AppendDocumentStart(nil)
	for _, e := range elems {
		switch e.Key() {
		case "Flows":
			if out, err = appendFiltered(out, e, func(bsoncore.Document) bool { return false }); err != nil {
				return nil, err
			}
		case "ObjectCollection":
			coll, ok := e.Value().DocumentOK()
			if !ok {
				out = append(out, e...)
				continue
			}
			celems, err := coll.Elements()
			if err != nil {
				return nil, err
			}
			cidx, cout := bsoncore.AppendDocumentElementStart(out, "ObjectCollection")
			for _, ce := range celems {
				if ce.Key() != "Objects" {
					cout = append(cout, ce...)
					continue
				}
				if cout, err = appendFiltered(cout, ce, isFlowParameter); err != nil {
					return nil, err
				}
			}
			if out, err = bsoncore.AppendDocumentEnd(cout, cidx); err != nil {
				return nil, err
			}
		default:
			out = append(out, e...)
		}
	}
	return bsoncore.AppendDocumentEnd(out, idx)
}

func isFlowParameter(d bsoncore.Document) bool {
	t, _ := d.Lookup("$Type").StringValueOK()
	return t == flowParameterType
}

// appendFiltered appends array element e keeping its non-document values and
// the documents keep accepts. A value that is not an array is kept as stored.
func appendFiltered(dst []byte, e bsoncore.Element, keep func(bsoncore.Document) bool) ([]byte, error) {
	arr, ok := e.Value().ArrayOK()
	if !ok {
		return append(dst, e...), nil
	}
	vals, err := arr.Values()
	if err != nil {
		return nil, err
	}
	idx, dst := bsoncore.AppendArrayElementStart(dst, e.Key())
	i := 0
	for _, v := range vals {
		if d, ok := v.DocumentOK(); ok && !keep(d) {
			continue
		}
		dst = bsoncore.AppendValueElement(dst, strconv.Itoa(i), v)
		i++
	}
	return bsoncore.AppendArrayEnd(dst, idx)
}
