// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// The domain-model write paths rebuild entities and associations from the
// semantic model (entityToGen / assocToGen), and a rebuild writes a CONSTANT for
// every property the semantic model does not carry. Carrying the stored raw bytes
// onto the rebuild does not rescue those: a property the converter SETS is dirty,
// and a dirty property is re-encoded from the rebuild over the raw bytes. So the
// carry of the raw bytes (#657, #1119, #1169) saved what the converters never
// touch — the GUID above all — and still reset what they do touch to a default:
//
//	ExportLevel              "Hidden" on the entity, each attribute, each association
//	XPathConstraintCaption   "" on every access rule
//	Documentation            "" on every access rule
//	element $IDs             fresh on every access rule, member access, delete behaviour
//
// UpdateDomainModel rebuilt EVERY entity and association in the unit that way, so
// one ALTER ASSOCIATION, RENAME or `mxcli layout` turned every API-exported entity
// in the module Hidden and rewrote every entity's access rules (ako/mxcli#801).
// Nothing reports it: a hidden entity is valid, the build is clean, and the loss
// only shows when the module is exported as a package and its public surface is
// gone.
//
// The fix is two-layered, and both layers are needed:
//
//  1. An element the statement did not change is not rebuilt at all. It passes
//     through as its stored element, which the codec re-emits byte-for-byte
//     (entityUnchanged / assocUnchanged).
//  2. An element the statement did change is rebuilt as before, and the stored
//     values of the properties the semantic model cannot express are written back
//     onto the rebuild (carryStoredEntity / carryStoredAssociation).

// carryStoredEntity makes a rebuilt entity (ge, from entityToGen) keep what the
// stored entity (orig) holds and the semantic model does not: its raw bytes and
// its children's identities (#657, #1119), its export level and its attributes'
// (#801), and its access rules where the statement left them alone (#801).
//
// Every write path that rebuilds an EXISTING entity goes through here —
// UpdateEntity (ALTER ENTITY, CREATE OR MODIFY ENTITY), UpdateDomainModel (ALTER /
// CREATE OR MODIFY ASSOCIATION, RENAME, layout) and MoveEntity — so a carry added
// for one statement cannot be missing from another.
func carryStoredEntity(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	if ge == nil || orig == nil || entity == nil {
		return
	}
	if raw := orig.Raw(); raw != nil {
		ge.SetRaw(raw)
	}
	carryChildIdentity(ge, orig, entity)
	if lvl := orig.ExportLevel(); lvl != "" {
		ge.SetExportLevel(lvl)
	}
	carryAccessRules(ge, orig, entity)
}

// carryAccessRules pairs each rebuilt access rule with the stored one it came from
// and keeps the stored rule where the statement left it unchanged.
//
// A rule is paired on the stored $ID the read path round-trips into the semantic
// rule (accessRuleFromGen), falling back to its position for a statement-declared
// rule that carries none — the same two-pass pairing the attribute carry uses.
//
//   - Unchanged (the semantic rule equals the stored one read back, and the member
//     sync in entityToGen added nothing): the stored element replaces the rebuild
//     and is re-emitted byte-for-byte — its $IDs, caption and documentation with it.
//   - Changed: the rebuild stands, and only what the semantic rule cannot carry —
//     its $ID, the XPath caption and the documentation — is written back onto it.
func carryAccessRules(ge, orig *genDm.Entity, entity *domainmodel.Entity) {
	var stored []*genDm.AccessRule
	storedByID := map[string]*genDm.AccessRule{}
	for _, el := range orig.AccessRulesItems() {
		if ar, ok := el.(*genDm.AccessRule); ok {
			stored = append(stored, ar)
			storedByID[string(ar.ID())] = ar
		}
	}
	rebuilt := ge.AccessRulesItems()
	if len(stored) == 0 || len(rebuilt) == 0 || len(rebuilt) != len(entity.AccessRules) {
		return
	}

	claimed := map[string]bool{}
	pair := make([]*genDm.AccessRule, len(rebuilt))
	for i, sem := range entity.AccessRules {
		if sem == nil || sem.ID == "" {
			continue
		}
		if sr := storedByID[string(sem.ID)]; sr != nil && !claimed[string(sr.ID())] {
			pair[i] = sr
			claimed[string(sr.ID())] = true
		}
	}
	for i := range rebuilt {
		if pair[i] != nil || i >= len(stored) || claimed[string(stored[i].ID())] {
			continue
		}
		if entity.AccessRules[i] != nil && entity.AccessRules[i].ID == "" {
			pair[i] = stored[i]
			claimed[string(stored[i].ID())] = true
		}
	}

	out := make([]element.Element, len(rebuilt))
	substituted := false
	for i, el := range rebuilt {
		out[i] = el
		gar, ok := el.(*genDm.AccessRule)
		sr := pair[i]
		if !ok || sr == nil {
			continue
		}
		if len(gar.MemberAccessesItems()) == len(sr.MemberAccessesItems()) &&
			semanticEqual(entity.AccessRules[i], accessRuleFromGen(sr)) {
			out[i] = sr
			substituted = true
			continue
		}
		gar.SetID(sr.ID())
		gar.SetXPathConstraintCaption(sr.XPathConstraintCaption())
		gar.SetDocumentation(sr.Documentation())
	}
	if !substituted {
		return
	}
	for i := len(rebuilt) - 1; i >= 0; i-- {
		ge.RemoveAccessRules(i)
	}
	for _, el := range out {
		ge.AddAccessRules(el)
	}
}

// entityUnchanged reports whether the semantic entity says exactly what the stored
// one does, in which case the write passes the stored element through instead of
// rebuilding it. A false "changed" costs only the rebuild the path always did; a
// false "unchanged" would discard the statement, so the comparison is over every
// field of the semantic entity rather than a list of fields thought to matter.
func entityUnchanged(e *domainmodel.Entity, orig *genDm.Entity) bool {
	if e == nil || orig == nil {
		return false
	}
	stored := entityFromGen(orig)
	// The reader fills OqlQuery from the view's source document on 11+, where the
	// entity itself stores none; it is not the entity's content either way.
	if stored.OqlQuery == "" {
		stored.OqlQuery = e.OqlQuery
	}
	return semanticEqual(e, stored)
}

// assocUnchanged is entityUnchanged for an association.
func assocUnchanged(a *domainmodel.Association, orig *genDm.Association) bool {
	if a == nil || orig == nil {
		return false
	}
	return semanticEqual(a, assocFromGen(orig))
}

// carryStoredAssociation is carryStoredEntity for a rebuilt association: the raw
// bytes (the GUID, #1169), the export level (#801), and the stored delete
// behaviour element where the statement left the behaviour alone, so its $ID and
// its refusal message's $IDs do not churn.
func carryStoredAssociation(ga, orig *genDm.Association, a *domainmodel.Association) {
	if ga == nil || orig == nil || a == nil {
		return
	}
	if raw := orig.Raw(); raw != nil {
		ga.SetRaw(raw)
	}
	if lvl := orig.ExportLevel(); lvl != "" {
		ga.SetExportLevel(lvl)
	}
	storedDB, ok := orig.DeleteBehavior().(*genDm.AssociationDeleteBehavior)
	if !ok || storedDB == nil {
		return
	}
	was := assocFromGen(orig)
	if semanticEqual(a.ParentDeleteBehavior, was.ParentDeleteBehavior) &&
		semanticEqual(a.ChildDeleteBehavior, was.ChildDeleteBehavior) {
		ga.SetDeleteBehavior(storedDB)
	}
}

// semanticEqual compares two semantic-model values field by field, the way
// reflect.DeepEqual does, except that it:
//
//   - skips each element's own identity (model.BaseElement: $ID and $Type) and its
//     ContainerID — the pairing is established by the caller, and a statement-
//     declared member carries no ID while its stored counterpart does;
//   - treats a nil slice or map as equal to an empty one, since the reader appends
//     (nil when empty) and a statement may build an empty literal.
//
// References to OTHER elements (a member access's attribute, an index segment's
// attribute, an association's endpoints) are ordinary fields and are compared.
func semanticEqual(a, b any) bool {
	return deepEqualSemantic(reflect.ValueOf(a), reflect.ValueOf(b))
}

var baseElementType = reflect.TypeOf(model.BaseElement{})

func deepEqualSemantic(a, b reflect.Value) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return deepEqualSemantic(a.Elem(), b.Elem())
	case reflect.Struct:
		t := a.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Type == baseElementType || f.Name == "ContainerID" {
				continue
			}
			if !f.IsExported() {
				// Unexported state is not part of the model; the semantic types
				// have none today, and comparing it would need unsafe access.
				continue
			}
			if !deepEqualSemantic(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !deepEqualSemantic(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if a.Len() != b.Len() {
			return false
		}
		for _, k := range a.MapKeys() {
			bv := b.MapIndex(k)
			if !bv.IsValid() || !deepEqualSemantic(a.MapIndex(k), bv) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a.Interface(), b.Interface())
	}
}
