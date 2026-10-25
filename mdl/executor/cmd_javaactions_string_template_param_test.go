// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// `create java action M.J (S: StringTemplate(Text))` — the type describe prints
// for a text-template parameter, BusinessEvents.PublishBusinessEvent_V2's
// Subject among them — had no case in the converter and was written as a
// String parameter. A describe -> exec round trip of such an action turned the
// template into a String, after which every call of it built a template value
// for a String parameter (found while fixing ako/mxcli#1451).
func TestParamType_StringTemplateIsATemplate(t *testing.T) {
	for _, c := range []struct{ context, grammar string }{
		{"Text", "Text"}, {"text", "Text"}, {"Sql", "Sql"}, {"SQL", "Sql"},
	} {
		pt := astDataTypeToJavaActionParamType(ast.DataType{Kind: ast.TypeStringTemplate, TemplateContext: c.context})
		st, ok := pt.(*javaactions.StringTemplateParameterType)
		if !ok {
			t.Errorf("StringTemplate(%s) converted to %T, want *StringTemplateParameterType", c.context, pt)
			continue
		}
		if st.Grammar != c.grammar {
			t.Errorf("StringTemplate(%s) grammar = %q, want %q", c.context, st.Grammar, c.grammar)
		}
	}
}
