// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// R10 (ako/mxcli#755): Studio Pro's consumed and published web services and XML
// schemas are not supported yet, and their names are reserved now: a statement
// naming one is an error that says so, never a generic parse error or a
// different statement that happens to parse.
func TestReservedDocumentTypesAreRefusedByName(t *testing.T) {
	for _, src := range []string{
		"create consumed web service M.Soap (Wsdl: 'https://x/?wsdl');",
		"create or modify published web service M.Soap (Path: 'ws');",
		"drop consumed web service M.Soap;",
		"describe published web service M.Soap;",
		"list consumed web services;",
		"list published web services in M;",
		"create xml schema M.Order (File: 'order.xsd');",
		"describe xml schema M.Order;",
		"list xml schemas;",
	} {
		t.Run(src, func(t *testing.T) {
			_, errs := Build(src)
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), "reserved") {
				t.Errorf("errs = %v, want one error saying the name is reserved", errs)
			}
		})
	}
	// Controls: the supported services and the words used elsewhere.
	for _, src := range []string{
		"list consumed rest services;",
		"list published rest services;",
		"list consumed mcp services;",
		"create persistent entity M.Site (Web: String(200), Xml: String(200), Schema: String(20));",
	} {
		if _, errs := Build(src); len(errs) > 0 {
			t.Errorf("%q: %v", src, errs)
		}
	}
}
