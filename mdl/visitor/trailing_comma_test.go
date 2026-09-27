// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"
)

// trailingCommaCases pairs a statement with a trailing comma in one of its
// lists (marked `,@`) against the same statement without it. ADR-0010 R11:
// trailing commas are allowed in every bracketed list, under every language
// version — it is additive, so no header is needed (#732).
//
// The `,@` marker is replaced by `,` for the trailing form and removed for the
// plain form, so the two sources differ in exactly that one comma.
var trailingCommaCases = map[string]string{
	"entity attributes":      "create persistent entity M.Note (Text: String(200),@);",
	"entity index":           "create persistent entity M.Note (A: String(20), B: String(20)) index (A, B,@);",
	"enumeration values":     "create enumeration M.Color (Red 'Red', Green 'Green',@);",
	"microflow parameters":   "create microflow M.F ($A: String, $B: Integer,@) begin end;",
	"java action parameters": "create java action M.J (A: String,@) returns Boolean as $$return true;$$;",
	"call arguments":         "create microflow M.F () begin call microflow M.G(A = 1, B = 2,@); end;",
	"page parameters":        "create page M.P (Params: { $A: M.E,@ }, Title: 'P', Layout: Atlas_Core.Atlas_Default) {};",
	"rest client properties": "create rest client M.Api (BaseUrl: 'https://x', Authentication: none,@) { operation Get { Method: get, Path: '/x', Response: none,@ } };",
	"rest client headers":    "create rest client M.Api (BaseUrl: 'https://x', Authentication: none) { operation Get { Method: get, Path: '/x', Headers: ('A' = 'b',@), Response: none } };",
	"published rest":         "create published rest service M.S (Path: 'rest/s', Version: '1.0',@) { };",
	"odata client":           "create odata client M.C (ODataVersion: OData4, MetadataUrl: 'https://x/$metadata',@);",
	"business events":        "create business event service M.BE (ServiceName: 'S', EventNamePrefix: '',@) { message Changed (Id: Long,@) publish; };",
	"agent model":            "create model M.GPT (Provider: MxCloudGenAI, Key: M.Key,@);",
	"agent":                  "create agent M.A (UsageType: Task, Model: M.GPT,@) { tool T { Enabled: true,@ } };",
	"module role grant":      "grant M.User on M.E (read *, write *,@);",
	"line comment after it":  "create enumeration M.Color (\n  Red 'Red',\n  Green 'Green',@ -- the last one\n);",
	"block comment after it": "create enumeration M.Color (Red 'Red', Green 'Green',@ /* last */ );",
}

func TestTrailingCommaInEveryList(t *testing.T) {
	for name, src := range trailingCommaCases {
		for _, header := range []string{"", "mdl 1;\n"} {
			t.Run(name+"/"+strings.TrimSpace(header), func(t *testing.T) {
				plain := header + strings.ReplaceAll(src, ",@", "")
				trailing := header + strings.ReplaceAll(src, ",@", ",")
				want, errs := Build(plain)
				if len(errs) > 0 {
					t.Fatalf("the plain form must parse: %v\n%s", errs, plain)
				}
				got, errs := Build(trailing)
				if len(errs) > 0 {
					t.Fatalf("a trailing comma must be accepted: %v\n%s", errs, trailing)
				}
				if !reflect.DeepEqual(got.Statements, want.Statements) {
					t.Errorf("a trailing comma changed the statement:\n got  %#v\n want %#v", got.Statements, want.Statements)
				}
			})
		}
	}
}

// A comma needs an item before it: `()` stays the only empty list, and `(,)`
// and `(a,,)` stay errors.
func TestTrailingCommaNeedsAnItem(t *testing.T) {
	for _, src := range []string{
		"create persistent entity M.Note (,);",
		"create enumeration M.Color (,);",
		"create microflow M.F (,) begin end;",
		"create enumeration M.Color (Red 'Red',,);",
		"create enumeration M.Color (Red 'Red', , );",
	} {
		if _, errs := Build(src); len(errs) == 0 {
			t.Errorf("a list holding only a comma was accepted: %s", src)
		}
	}
}
