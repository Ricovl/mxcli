// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A system member (`owner: AutoOwner` and the three others) declared on a
// SPECIALIZATION was dropped without a word: exec reported "Created" (or
// "Unchanged" on a re-run) and wrote nothing, because Mendix keeps those flags
// on the NoGeneralization of the root of the chain and a specialization has no
// place to store them. Two different outcomes hide behind the silence:
//
//   - the root already stores the member (System.FileDocument stores all four):
//     the specialization HAS it, inherited, and the declaration is redundant —
//     worth a warning, not a refusal;
//   - the root does not: the user asked for a member the entity will not have,
//     and every `[System.owner = …]` against it then fails the build with
//     CE0161. That is refused, naming the root to change instead.

// systemMemberOfPseudoType maps the Auto* pseudo-types to the system member
// they store, in XPath spelling.
func systemMemberOfPseudoType(k ast.DataTypeKind) string {
	switch k {
	case ast.TypeAutoOwner:
		return "owner"
	case ast.TypeAutoChangedBy:
		return "changedBy"
	case ast.TypeAutoCreatedDate:
		return "createdDate"
	case ast.TypeAutoChangedDate:
		return "changedDate"
	}
	return ""
}

// checkSpecializationSystemMembers judges the Auto* attributes declared on an
// entity that extends generalizationQN. It returns a warning per member that is
// inherited anyway, and an error for a member the chain's root does not store.
func checkSpecializationSystemMembers(ctx *ExecContext, entityQN, generalizationQN string, attrs []ast.Attribute, quietUnknown bool) ([]string, error) {
	if generalizationQN == "" {
		return nil, nil
	}
	var warnings []string
	var entities map[string]*domainmodel.Entity // loaded on first use
	for _, a := range attrs {
		member := systemMemberOfPseudoType(a.Type.Kind)
		if member == "" {
			continue
		}
		if entities == nil {
			entities = buildEntityIndex(ctx)
		}
		stores, root, known := systemMemberRoot(entities, generalizationQN, member)
		switch {
		case !known && quietUnknown:
			continue
		case !known:
			warnings = append(warnings, fmt.Sprintf(
				"%s extends %s: `%s` is ignored — Mendix stores system members on the root of the generalization chain, "+
					"not on a specialization, and mxcli could not resolve the chain to say whether %s has it",
				entityQN, generalizationQN, a.Name, member))
		case stores:
			warnings = append(warnings, fmt.Sprintf(
				"%s extends %s: `%s` is inherited — %s already stores %s, so the declaration changes nothing and is ignored",
				entityQN, generalizationQN, a.Name, root, member))
		default:
			fix := fmt.Sprintf("Store it on the root instead: alter entity %s add attribute %s: %s", root, a.Name, autoTypeName(member))
			if strings.HasPrefix(root, "System.") {
				fix = "System entities cannot be changed, so no specialization of " + root + " can store it"
			}
			return warnings, mdlerrors.NewValidationf(
				"%s extends %s, whose root %s does not store %s — Mendix keeps system members on the root of the "+
					"generalization chain, so `%s` cannot be stored on a specialization (it would be dropped, and "+
					"a constraint on System.%s / %s against %s fails the build with CE0161). %s",
				entityQN, generalizationQN, root, member, a.Name, member, member, entityQN, fix)
		}
	}
	return warnings, nil
}

func autoTypeName(member string) string {
	switch member {
	case "owner":
		return "AutoOwner"
	case "changedBy":
		return "AutoChangedBy"
	case "createdDate":
		return "AutoCreatedDate"
	case "changedDate":
		return "AutoChangedDate"
	}
	return ""
}
