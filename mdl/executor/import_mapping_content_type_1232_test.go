// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1232 (ako/mxcli#1290): `import from mapping` over a mapping
// sourced from a web service operation was written with ContentType Json, and
// mx check refused the replayed microflow: "The selected import mapping cannot
// handle content type JSON." Studio Pro fixes it on opening the activity —
// "This action now imports XML" — so the content type follows the mapping's
// source: XML for an XML schema or a web service, JSON otherwise.
func TestImportFromMapping_ContentTypeFollowsTheMappingSource(t *testing.T) {
	ws := model.WebServiceMappingSource{ImportedWebService: "ExamplesModule.WS", OperationName: "Op"}
	for _, tc := range []struct {
		name string
		im   model.ImportMapping
		want string
	}{
		{"web service operation (the reported shape)", model.ImportMapping{WebServiceSource: ws}, "Xml"},
		{"XML schema", model.ImportMapping{XmlSchema: "ExamplesModule.Schema"}, "Xml"},
		{"JSON structure (control)", model.ImportMapping{JsonStructure: "ExamplesModule.Json"}, "Json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im := tc.im
			im.Elements = []*model.ImportMappingElement{{Kind: "Object", Entity: "ExamplesModule.Root", MaxOccurs: 1}}
			fb := &flowBuilder{
				varTypes: map[string]string{},
				backend: &mock.MockBackend{
					GetImportMappingByQualifiedNameFunc: func(string, string) (*model.ImportMapping, error) { return &im, nil },
				},
			}
			fb.addImportFromMappingAction(&ast.ImportFromMappingStmt{
				OutputVariable: "Variable_2",
				SourceVariable: "latestHttpResponse",
				Mapping:        ast.QualifiedName{Module: "ExamplesModule", Name: "Import_mapping"},
			})
			act := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity).Action.(*microflows.ImportXmlAction)
			if got := act.ResultHandling.ContentType; got != tc.want {
				t.Errorf("ContentType = %q, want %q", got, tc.want)
			}
		})
	}
}
