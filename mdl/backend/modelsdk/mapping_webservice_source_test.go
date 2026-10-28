// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// withStoredKeys re-encodes doc with the given top-level keys set to the given
// values, keeping key order — the shape a Studio Pro document has and mxcli's
// own writer never produces for these keys.
func withStoredKeys(t *testing.T, doc []byte, set map[string]any) []byte {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	for i := range d {
		if v, ok := set[d[i].Key]; ok {
			d[i].Value = v
			delete(set, d[i].Key)
		}
	}
	for k := range set {
		t.Fatalf("key %q is not in the encoded document", k)
	}
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ako/mxcli#1631: a mapping sourced from an imported web service stores the
// service as WsdlFile, and an export mapping's header flag as
// IsHeaderParameter — mxbuild's own metadata pairs each storage name with the
// SDK names importedWebService and isHeader. gen bound the SDK names, so a
// stored SOAP mapping read back with no source at all, and the refusal that
// keeps a rewrite from deleting the binding (checkNoWebServiceSource, #365)
// could not fire on one. Its tests built the model by hand, never from a
// stored document.
func TestMappingWebServiceSource_ReadsTheStoredKeys(t *testing.T) {
	b := connectedBackend(t)
	mods, err := b.ListModules()
	if err != nil || len(mods) == 0 {
		t.Fatalf("list modules: %v", err)
	}
	var mod *model.Module
	for _, m := range mods {
		if m.Name == "MyFirstModule" {
			mod = m
		}
	}
	if mod == nil {
		t.Fatal("fixture has no MyFirstModule")
	}

	insert := func(g element.Element, unitType string, set map[string]any) string {
		raw, err := (&codec.Encoder{}).Encode(g)
		if err != nil {
			t.Fatal(err)
		}
		raw = withStoredKeys(t, raw, set)
		if err := b.writer.InsertUnit(string(g.ID()), string(mod.ID), "Documents", unitType, raw); err != nil {
			t.Fatal(err)
		}
		return string(g.ID())
	}

	soap := map[string]any{
		"WsdlFile":           "MyFirstModule.WS_Orders",
		"ServiceName":        "OrderService",
		"OperationName":      "GetOrder",
		"XsdRootElementName": "GetOrderResponse",
	}
	im := &model.ImportMapping{BaseElement: model.BaseElement{ID: model.ID(mmpr.GenerateID())}, Name: "IMM_Soap"}
	insert(importMappingToGen(im), "ImportMappings$ImportMapping", copyMap(soap))
	em := &model.ExportMapping{BaseElement: model.BaseElement{ID: model.ID(mmpr.GenerateID())}, Name: "EXM_Soap"}
	exportSet := copyMap(soap)
	exportSet["IsHeaderParameter"] = true
	insert(exportMappingToGen(em), "ExportMappings$ExportMapping", exportSet)

	gotIm, err := b.GetImportMappingByQualifiedName("MyFirstModule", "IMM_Soap")
	if err != nil {
		t.Fatal(err)
	}
	gotEm, err := b.GetExportMappingByQualifiedName("MyFirstModule", "EXM_Soap")
	if err != nil {
		t.Fatal(err)
	}
	for kind, src := range map[string]model.WebServiceMappingSource{
		"import": gotIm.WebServiceSource, "export": gotEm.WebServiceSource,
	} {
		if src.ImportedWebService != "MyFirstModule.WS_Orders" || !src.IsSet() {
			t.Errorf("%s mapping: stored WsdlFile reads back as ImportedWebService %q (IsSet %v), want %q",
				kind, src.ImportedWebService, src.IsSet(), "MyFirstModule.WS_Orders")
		}
		// The control: these keys were always read correctly.
		if src.ServiceName != "OrderService" || src.OperationName != "GetOrder" {
			t.Errorf("%s mapping: service/operation read back as %q/%q", kind, src.ServiceName, src.OperationName)
		}
	}
	if !gotEm.WebServiceSource.IsHeader {
		t.Error("export mapping: stored IsHeaderParameter true reads back as IsHeader false")
	}
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
