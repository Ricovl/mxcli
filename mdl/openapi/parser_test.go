// SPDX-License-Identifier: Apache-2.0

package openapi

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#280 (ako/mxcli#1234) used the Swagger 2.0 Petstore. The
// parser reads only OpenAPI 3 fields, so a 2.0 document parsed "successfully"
// into an empty contract — BaseUrl ”, request bodies dropped, every response
// `none` — and nothing said so. Refusing is strictly better than that.
const swagger2Petstore = `{
  "swagger": "2.0",
  "info": {"title": "Swagger Petstore", "version": "1.0.7"},
  "host": "petstore.swagger.io",
  "basePath": "/v2",
  "schemes": ["https"],
  "paths": {
    "/pet/{petId}": {
      "get": {
        "operationId": "getPetById",
        "produces": ["application/json"],
        "parameters": [{"name": "petId", "in": "path", "required": true, "type": "integer", "format": "int64"}],
        "responses": {"200": {"description": "ok", "schema": {"$ref": "#/definitions/Pet"}}}
      }
    }
  },
  "definitions": {"Pet": {"type": "object", "properties": {"id": {"type": "integer"}}}}
}`

func TestParseSpec_RefusesSwagger2(t *testing.T) {
	for _, tc := range []struct{ name, ext, doc string }{
		{"json", ".json", swagger2Petstore},
		{"yaml", ".yaml", "swagger: '2.0'\ninfo:\n  title: T\n  version: '1'\npaths: {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := ParseSpec([]byte(tc.doc), tc.ext)
			if err == nil {
				t.Fatalf("a Swagger 2.0 document was accepted (%d paths) — it converts into an empty contract with no warning", len(spec.Paths))
			}
			for _, want := range []string{"Swagger 2.0", "OpenAPI 3"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should name %q so the user knows what to do: %v", want, err)
				}
			}
		})
	}
}

// CONTROL: OpenAPI 3.0 and 3.1 still parse.
func TestParseSpec_AcceptsOpenAPI3(t *testing.T) {
	for _, v := range []string{"3.0.2", "3.1.0"} {
		doc := `{"openapi": "` + v + `", "info": {"title": "T", "version": "1"}, "servers": [{"url": "https://x.example/api"}],
		  "paths": {"/a": {"get": {"operationId": "getA", "responses": {"200": {"description": "ok", "content": {"application/json": {}}}}}}}}`
		spec, err := ParseSpec([]byte(doc), ".json")
		if err != nil {
			t.Fatalf("OpenAPI %s refused: %v", v, err)
		}
		svc, _, err := ToRestClientModel(spec, "Svc", "")
		if err != nil || len(svc.Operations) != 1 || svc.Operations[0].ResponseType != "JSON" {
			t.Errorf("OpenAPI %s did not convert to one JSON operation: %+v, %v", v, svc, err)
		}
	}
}
