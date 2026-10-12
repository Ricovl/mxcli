// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// showMessageTypeRule is the rule ID for "show message with a type Mendix does not have".
const showMessageTypeRule = "MDL-SHOWMSG01"

// checkShowMessageType refuses a `show message … type X` whose X is not a
// ShowMessageType value. Such a value is written as-is, `mx check` reports
// nothing, and the runtime shows an Information dialog in its place — the
// silent half of mendixlabs/mxcli#1280, whose reporter wrote `type ok`. The
// case of a valid value is not this rule's business: the visitor stores
// `type error` as "Error".
func (v *microflowValidator) checkShowMessageType(body []ast.MicroflowStatement) {
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		sm, ok := s.(*ast.ShowMessageStmt)
		if !ok || sm.Type == "" {
			return
		}
		if _, ok := microflows.CanonicalMessageType(sm.Type); ok {
			return
		}
		valid := make([]string, len(microflows.MessageTypes))
		for i, t := range microflows.MessageTypes {
			valid[i] = string(t)
		}
		v.addViolation(showMessageTypeRule, linter.SeverityError,
			fmt.Sprintf("show message type '%s' is not a Mendix message type; the runtime would show it "+
				"as an Information dialog. Valid types: %s.", sm.Type, strings.Join(valid, ", ")),
			"Use one of "+strings.Join(valid, ", ")+".")
	})
}
