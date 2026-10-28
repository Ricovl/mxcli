// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
)

// EditConsumedRestService is `alter consumed rest service`: a splice of the
// stored unit, not a rebuild (mendixlabs/mxcli#1608).
//
// UpdateConsumedRestService rebuilds the whole document from the semantic
// model, so everything the reader does not carry — a query parameter's
// optional usage, a parameter's test value, a response's status code — comes
// back as the writer's constant, on every operation, whichever one the
// statement was about. Adding one operation that way would reset all the
// others. Here an operation the edit does not name is copied as its stored
// bytes; a modified one has only the keys the statement wrote re-encoded; and
// a new one is encoded exactly as CREATE encodes it.
func (b *Backend) EditConsumedRestService(id model.ID, edit *backend.ConsumedRestServiceEdit) error {
	if edit == nil {
		return fmt.Errorf("EditConsumedRestService: nil edit")
	}
	if b.writer == nil {
		return fmt.Errorf("EditConsumedRestService: not connected for writing")
	}
	doc, err := b.rawUnitDoc(id)
	if err != nil {
		return fmt.Errorf("EditConsumedRestService: %w", err)
	}
	forms := b.restBodyForms()

	if edit.BaseUrl != nil {
		v, err := encodeFlowElementToD(valueTemplateElem(*edit.BaseUrl))
		if err != nil {
			return fmt.Errorf("EditConsumedRestService: encode BaseUrl: %w", err)
		}
		doc = setDocKey(doc, "BaseUrl", v)
	}
	if edit.SetAuthentication {
		var v any // BSON null: no authentication, as CREATE writes it
		if edit.Authentication != nil {
			if v, err = encodeFlowElementToD(restAuthSchemeToGen(edit.Authentication)); err != nil {
				return fmt.Errorf("EditConsumedRestService: encode AuthenticationScheme: %w", err)
			}
		}
		doc = setDocKey(doc, "AuthenticationScheme", v)
	}

	drop := map[string]bool{}
	for _, n := range edit.Drop {
		drop[strings.ToLower(n)] = true
	}
	modify := map[string]backend.ConsumedRestOperationEdit{}
	for _, m := range edit.Modify {
		modify[strings.ToLower(m.Name)] = m
	}

	var ops []any
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(doc, "Operations")) {
		od, ok := el.(bson.D)
		if !ok {
			ops = append(ops, el)
			continue
		}
		name := strings.ToLower(bsonnav.DGetString(od, "Name"))
		if drop[name] {
			delete(drop, name)
			continue
		}
		if m, ok := modify[name]; ok {
			delete(modify, name)
			if od, err = editRestOperationDoc(od, m, forms); err != nil {
				return fmt.Errorf("EditConsumedRestService: operation %s: %w", m.Name, err)
			}
		}
		ops = append(ops, od)
	}
	// The executor checks the names first; this is the backstop that keeps a
	// misnamed edit from writing a document it did not describe.
	for _, n := range edit.Drop {
		if drop[strings.ToLower(n)] {
			return fmt.Errorf("EditConsumedRestService: no operation %q to drop", n)
		}
	}
	for _, m := range edit.Modify {
		if _, ok := modify[strings.ToLower(m.Name)]; ok {
			return fmt.Errorf("EditConsumedRestService: no operation %q to modify", m.Name)
		}
	}
	for _, op := range edit.Add {
		od, err := encodeFlowElementToD(restOperationToGen(op, forms))
		if err != nil {
			return fmt.Errorf("EditConsumedRestService: encode operation %s: %w", op.Name, err)
		}
		ops = append(ops, od)
	}
	// The list keeps the marker it was stored with; one written without a list
	// gets Studio Pro's.
	marker := any(int32(3))
	if stored, ok := bsonnav.DGet(doc, "Operations").(bson.A); ok && len(stored) > 0 {
		marker = stored[0]
	}
	doc = setDocKey(doc, "Operations", append(bson.A{marker}, ops...))

	contents, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("EditConsumedRestService: marshal: %w", err)
	}
	return b.writer.UpdateRawUnitPatch(string(id), contents)
}

// restOperationKeys maps an MDL operation property to the stored keys it is
// written to.
var restOperationKeys = map[string][]string{
	"Method":     {"Method"},
	"Body":       {"Method"}, // the body lives in the method element
	"Path":       {"Path"},
	"Parameters": {"Parameters"},
	"Query":      {"QueryParameters"},
	"Headers":    {"Headers"},
	"Timeout":    {"Timeout"},
	"Response":   {"ResponseHandling"},
}

// editRestOperationDoc rewrites the stored keys behind the MDL properties in
// m.Keys, each taken from the operation encoded as CREATE would encode it, and
// copies every other key of the stored operation unchanged.
//
// A method change that keeps the stored method element's shape — with a body
// to with a body, or without to without — sets only its HttpMethod, so the
// stored body survives a `Method:` that does not mention it.
func editRestOperationDoc(stored bson.D, m backend.ConsumedRestOperationEdit, forms restBodyForms) (bson.D, error) {
	encoded, err := encodeFlowElementToD(restOperationToGen(m.Operation, forms))
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	bodyWritten := false
	for _, k := range m.Keys {
		for _, sk := range restOperationKeys[k] {
			keys[sk] = true
		}
		bodyWritten = bodyWritten || k == "Body"
	}
	out := stored
	for key := range keys {
		v := bsonnav.DGet(encoded, key)
		if key == "Method" && !bodyWritten {
			if kept, ok := withHttpMethodOnly(bsonnav.DGetDoc(stored, "Method"), v); ok {
				v = kept
			}
		}
		out = setDocKey(out, key, v)
	}
	return out, nil
}

// withHttpMethodOnly returns the stored method element with the encoded one's
// HttpMethod, when both have the same $Type (so the stored body, if any, is
// still the right shape).
func withHttpMethodOnly(stored bson.D, encoded any) (bson.D, bool) {
	enc, ok := encoded.(bson.D)
	if !ok || stored == nil || bsonnav.DGetString(stored, "$Type") != bsonnav.DGetString(enc, "$Type") {
		return nil, false
	}
	out := append(bson.D(nil), stored...)
	bsonnav.DSet(out, "HttpMethod", bsonnav.DGet(enc, "HttpMethod"))
	return out, true
}

// setDocKey returns doc with key set to v, appended when doc lacks it.
func setDocKey(doc bson.D, key string, v any) bson.D {
	if bsonnav.DSet(doc, key, v) {
		return doc
	}
	return append(doc, bson.E{Key: key, Value: v})
}
