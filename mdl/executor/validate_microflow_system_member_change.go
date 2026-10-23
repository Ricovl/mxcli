// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// systemMemberChangeRule is the rule ID for "a create or change object that
// sets System.owner / System.changedBy".
const systemMemberChangeRule = "MDL-CHANGE01"

// A create or change object member naming System.owner or System.changedBy
// was refused at exec with advice to `create or modify association
// System.owner from …` — which no project can do — while `check -p` passed the
// same script (mendixlabs/mxcli#1612, ako/mxcli#1540).
//
// The issue expected the member to be written as Studio Pro would. Measured
// with mx check on 11.12.5, writing it the only way a member change can hold
// an association (Association "System.owner") is
//
//	CE1050 "Unknown association 'System.owner'."
//
// for an entity storing its owner and one that does not, a System.FileDocument
// specialization, a specialization of an owning root, System.changedBy alike,
// in a microflow and in a nanoflow. The name itself resolves — `System.Owner`
// is CE1613 instead — but it is not a member a create or change can set. So
// there is nothing to write; this is the refusal, worded so it can be acted on.
//
// Caution when re-measuring: a CE1613 anywhere in the project hid every CE1050
// from the same `mx check` run, which first read as "builds clean". Measure
// each variant in a project with no other error.
//
// A list operation is not covered: `find $l by System.owner = $u` builds clean.

// builtinSystemMemberAssociation reports the spelling of the System
// association member names, in any case, when it is System.owner or
// System.changedBy.
func builtinSystemMemberAssociation(member string) (string, bool) {
	module, name, ok := strings.Cut(member, ".")
	if !ok || !strings.EqualFold(module, "System") {
		return "", false
	}
	switch strings.ToLower(name) {
	case "owner":
		return "System.owner", true
	case "changedby":
		return "System.changedBy", true
	}
	return "", false
}

// systemMemberChangeRefusal is the message and suggestion for a create or
// change object member naming System.owner / System.changedBy. The check rule
// and the flow builder both word it from here, so check and exec say the same.
// entityQN is "" when the variable's entity is not known.
func systemMemberChangeRefusal(member, canonical, entityQN string) (message, suggestion string) {
	on, target := "the object", "<Module>.<Entity>"
	if entityQN != "" {
		on, target = entityQN, entityQN
	}
	message = fmt.Sprintf("member %q on %s: %s is kept by the runtime, not set by a microflow — a create or change "+
		"object cannot set it, and mxbuild rejects the member with CE1050 \"Unknown association '%s'.\" "+
		"(whether or not the entity stores it)", member, on, canonical, canonical)
	suggestion = fmt.Sprintf("To record a user on the object, model the association: `create association %s_User "+
		"from %s to System.User;` — set that, and constrain on it instead of %s", target, target, canonical)
	return message, suggestion
}

// checkSystemMemberChange flags each create object, change object and member
// `set` that names System.owner or System.changedBy. It needs no project: no
// module but System declares these names, and the entity's flags do not change
// the verdict.
func (v *microflowValidator) checkSystemMemberChange(body []ast.MicroflowStatement) {
	types := newFlowObjectTypes(v.params)
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		defer types.observe(s)
		var entity string
		var members []string
		switch st := s.(type) {
		case *ast.CreateObjectStmt:
			if st.EntityType.Module != "" {
				entity = st.EntityType.String()
			}
			for _, c := range st.Changes {
				members = append(members, c.Attribute)
			}
		case *ast.ChangeObjectStmt:
			entity = types.objects[strings.TrimPrefix(st.Variable, "$")]
			for _, c := range st.Changes {
				members = append(members, c.Attribute)
			}
		case *ast.MfSetStmt:
			if i := strings.LastIndex(st.Target, "/"); i >= 0 {
				entity = types.objects[strings.TrimPrefix(st.Target[:i], "$")]
				members = append(members, st.Target[i+1:])
			}
		}
		for _, m := range members {
			canonical, ok := builtinSystemMemberAssociation(m)
			if !ok {
				continue
			}
			msg, suggestion := systemMemberChangeRefusal(m, canonical, entity)
			v.addViolation(systemMemberChangeRule, linter.SeverityError, msg, suggestion)
		}
	})
}

// refuseSystemMemberChange is checkSystemMemberChange's twin in the flow
// builder, for the paths that build without the rule having run (an ALTER that
// inserts statements, --no-check). It reports whether the member was refused.
func (fb *flowBuilder) refuseSystemMemberChange(member, entityQN string) bool {
	canonical, ok := builtinSystemMemberAssociation(member)
	if !ok {
		return false
	}
	msg, suggestion := systemMemberChangeRefusal(member, canonical, entityQN)
	fb.addError("%s. %s", msg, suggestion)
	return true
}
