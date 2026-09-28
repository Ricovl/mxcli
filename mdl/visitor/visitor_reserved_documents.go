// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitReservedDocumentStatement refuses a statement naming a Studio Pro
// document type MDL does not support yet (R10, ako/mxcli#755).
func (b *Builder) ExitReservedDocumentStatement(ctx *parser.ReservedDocumentStatementContext) {
	name := ctx.ReservedDocumentName()
	if name == nil {
		return
	}
	nc := name.(*parser.ReservedDocumentNameContext)
	if nc.IDENTIFIER() != nil && !strings.EqualFold(nc.IDENTIFIER().GetText(), "schemas") {
		b.addError(fmt.Errorf("line %d: unexpected %q after `xml`", ctx.GetStart().GetLine(), nc.IDENTIFIER().GetText()))
		return
	}
	var words []string
	for i := 0; i < nc.GetChildCount(); i++ {
		words = append(words, strings.ToLower(nc.GetChild(i).(interface{ GetText() string }).GetText()))
	}
	kind := strings.TrimSuffix(strings.Join(words, " "), "s")
	b.addError(fmt.Errorf("line %d: `%s` is a Studio Pro document type MDL does not support yet; "+
		"the name is reserved for it (R10)", ctx.GetStart().GetLine(), kind))
}
