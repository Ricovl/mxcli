// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// validateBusinessEventInheritedMembers reports a business event message
// attribute named like a member the message's entity inherits — from
// BusinessEvents.PublishedBusinessEvent or anything else up its generalization
// chain (mendixlabs/mxcli#1525, the reporter's `EventId`). The entity must
// declare one attribute per message attribute, an inherited attribute does not
// count, and it cannot redeclare one it inherits — so such a message can never be
// satisfied. Measured on mx 11.6.6 and 11.15.0 (blank apps, a stub
// PublishedBusinessEvent declaring EventId): mxbuild reports "The Business Events
// attribute 'EventId' is not present on the entity." on the service, after exec
// has written it; the same service with the attribute dropped is clean of it.
//
// The inherited members are read from the project (and from entities the script
// itself creates), never from a list kept here: the installed marketplace
// module decides what PublishedBusinessEvent declares. Where the chain cannot be
// read — the BusinessEvents module is not in the project, an ancestor does not
// resolve — nothing is reported for the part that could not be read.
//
// Matching ignores case: two members differing only in case are one member to
// Mendix.
func validateBusinessEventInheritedMembers(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	b, _ := ctx.Backend.(entityLookupBackend)

	var errs []error
	for i, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateBusinessEventServiceStmt)
		if !ok {
			continue
		}
		for _, msg := range s.Messages {
			if msg.Entity == "" || len(msg.Attributes) == 0 {
				continue
			}
			inherited := inheritedAttributeOwners(b, sc, msg.Entity)
			if len(inherited) == 0 {
				continue
			}
			for _, a := range msg.Attributes {
				owner, clash := inherited[strings.ToLower(a.Name)]
				if !clash {
					continue
				}
				errs = append(errs, mdlerrors.NewValidationf(
					"statement %d: business event service %s, message %s: attribute '%s' collides with "+
						"the member %s.%s that %s inherits — an inherited attribute does not count as the "+
						"entity's own, so mxbuild reports \"The Business Events attribute '%[4]s' is not present "+
						"on the entity\". Rename the message attribute and add it to the entity under that name",
					i+1, s.Name.String(), msg.MessageName, a.Name, owner, a.Name, msg.Entity))
			}
		}
	}
	return errs
}

// inheritedAttributeOwners maps each attribute an entity inherits (lower-cased)
// to the qualified name of the ancestor declaring it. The entity's own
// attributes are not included. Each ancestor is read from the script when the
// script creates it, otherwise from the project; the walk stops at the first
// one that resolves in neither.
func inheritedAttributeOwners(b entityLookupBackend, sc *scriptContext, entityQN string) map[string]string {
	out := map[string]string{}
	seen := map[string]bool{strings.ToLower(entityQN): true}
	parent, ok := entityParent(b, sc, entityQN)
	for ok && parent != "" && !seen[strings.ToLower(parent)] {
		seen[strings.ToLower(parent)] = true
		attrs, found := entityOwnAttributeNames(b, sc, parent)
		if !found {
			break
		}
		for _, name := range attrs {
			if _, dup := out[strings.ToLower(name)]; !dup {
				out[strings.ToLower(name)] = parent
			}
		}
		parent, ok = entityParent(b, sc, parent)
	}
	return out
}

// entityParent is the generalization of entityQN ("" for none); ok is false
// when the entity resolves in neither the script nor the project.
func entityParent(b entityLookupBackend, sc *scriptContext, entityQN string) (string, bool) {
	if sc != nil {
		if gen, declared := sc.entityGeneralizations[strings.ToLower(entityQN)]; declared {
			return gen, true
		}
	}
	if e, found := findEntityByQN(b, entityQN); found {
		return e.GeneralizationRef, true
	}
	return "", false
}

// entityOwnAttributeNames lists the attributes entityQN itself declares.
func entityOwnAttributeNames(b entityLookupBackend, sc *scriptContext, entityQN string) ([]string, bool) {
	if sc != nil {
		for qn, decl := range sc.entityDecls {
			if strings.EqualFold(qn, entityQN) {
				names := make([]string, 0, len(decl.Attributes))
				for _, a := range decl.Attributes {
					names = append(names, a.Name)
				}
				return names, true
			}
		}
	}
	e, found := findEntityByQN(b, entityQN)
	if !found {
		return nil, false
	}
	names := make([]string, 0, len(e.Attributes))
	for _, a := range e.Attributes {
		names = append(names, a.Name)
	}
	return names, true
}
