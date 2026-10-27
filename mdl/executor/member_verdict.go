// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"
)

// Whether an entity has a member of a given name, for a member a flow names
// bare — a list operation's, an aggregate's — answered once for the flow
// builder and check --references alike (ako/mxcli#1646). The builder fell back
// to writing `<entity>.<name>` when its own resolution found nothing, which is
// the attribute mxbuild rejects with CE1613; it now asks this verdict first and
// refuses a name established as missing, with the message check reports.

// memberKind says what a resolved member name is.
type memberKind int

const (
	memberIsAttribute memberKind = iota + 1
	memberIsAssociation
	// memberIsSystemDate is createdDate / changedDate: not an attribute in the
	// model but a flag on the root of the generalization chain, read and sorted
	// on by that lower-camel name (measured clean on 11.13.0 for a sort, a
	// maximum and an expression, on an entity whose root stores it).
	memberIsSystemDate
)

// memberLinkFacts is what the member verdict needs to know about one link of
// an entity's generalization chain.
type memberLinkFacts struct {
	attribute      bool   // the entity declares an attribute of the name
	association    bool   // its module has an association of the name
	generalization string // the next link; "" at the root
	storesDate     bool   // at the root: it stores the system date the name spells
}

// memberModel reads one link. ok is false when the link cannot be read — a
// module or domain model the backend cannot load, an entity it does not have —
// which makes the verdict memberUnknown: silence, never a refusal.
type memberModel interface {
	memberLink(entityQN, name string) (facts memberLinkFacts, ok bool)
}

// memberVerdictOver is THE walk: entityQN and its generalizations, nearest
// first, for a member called name — an attribute of a link, or an association
// of that name in a link's module (an association is named on the module that
// owns it, not on either end). With systemDates, createdDate / changedDate are
// members too when the chain's root stores them: they are flags on the root,
// not attributes, and a flow reads and sorts on them by that lower-camel name
// (measured clean on 11.13.0 for a sort, a maximum and an expression; reading
// one the root does not store is CE0117).
//
// Matching is case-sensitive, as Mendix's own is: mxbuild rejects `orderno`
// for `OrderNo` with CE1613 in a change and CE0117 in an expression.
func memberVerdictOver(m memberModel, entityQN, name string, systemDates bool) (memberResolution, memberKind) {
	if m == nil || entityQN == "" || name == "" {
		return memberUnknown, 0
	}
	date := systemDates && (name == "createdDate" || name == "changedDate")
	seen := map[string]bool{}
	for qn := entityQN; qn != ""; {
		if seen[qn] {
			// A generalization cycle is not a model mxcli should reason about.
			return memberUnknown, 0
		}
		seen[qn] = true
		link, ok := m.memberLink(qn, name)
		if !ok {
			return memberUnknown, 0
		}
		switch {
		case link.attribute:
			return memberFound, memberIsAttribute
		case link.association:
			return memberFound, memberIsAssociation
		case link.generalization == "":
			if date && link.storesDate {
				return memberFound, memberIsSystemDate
			}
			return memberMissing, 0
		}
		qn = link.generalization
	}
	return memberUnknown, 0
}

// backendMemberModel reads links from a backend: the project as the statements
// before have left it, which is what the flow builder sees.
type backendMemberModel struct{ b entityLookupBackend }

func (m backendMemberModel) memberLink(entityQN, name string) (memberLinkFacts, bool) {
	if m.b == nil {
		return memberLinkFacts{}, false
	}
	module, entityName, ok := strings.Cut(entityQN, ".")
	if !ok {
		return memberLinkFacts{}, false
	}
	mod, err := m.b.GetModuleByName(module)
	if err != nil || mod == nil {
		return memberLinkFacts{}, false
	}
	dm, err := m.b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return memberLinkFacts{}, false
	}
	entity := dm.FindEntityByName(entityName)
	if entity == nil {
		return memberLinkFacts{}, false
	}
	facts := memberLinkFacts{generalization: entity.GeneralizationRef, association: associationNamedInModule(dm, name)}
	for _, attr := range entity.Attributes {
		if attr != nil && attr.Name == name {
			facts.attribute = true
		}
	}
	if entity.GeneralizationRef == "" && (name == "createdDate" || name == "changedDate") {
		facts.storesDate = entityStoresSystemMember(entity, name)
	}
	return facts, true
}

// entityMemberKind is the verdict on an attribute or association name against
// a backend — resolveMemberOnEntity's question, for a CREATE / CHANGE member.
func entityMemberKind(b entityLookupBackend, entityQN, name string) (memberResolution, memberKind) {
	return memberVerdictOver(backendMemberModel{b}, entityQN, name, false)
}

// flowMemberVerdict is the verdict on a member a flow names bare — a list
// operation's, an aggregate's — against a backend; systemDates says whether
// the statement can read createdDate / changedDate (a sort can, an aggregate
// cannot). exec's builder asks this; check asks the same walk through the
// script (flowMemberRefs.member).
func flowMemberVerdict(b entityLookupBackend, entityQN, name string, systemDates bool) (memberResolution, memberKind) {
	return memberVerdictOver(backendMemberModel{b}, entityQN, name, systemDates)
}

// listMemberRefusal is the message for a list operation or aggregate naming a
// member its list's entity does not have, shared by the flow builder and
// check --references so both say the same thing (ako/mxcli#1646). where is the
// statement as written ("find $l by Numbr"); near is nearMembersFor's list.
//
// An aggregate over a system date gets its own: a sort and an expression read
// createdDate / changedDate where the root stores them, an aggregate never does
// — measured on 11.13.0, `maximum $l by createdDate` is CE0226 "Entity 'M.E'
// does not contain attribute 'M.E.createdDate'." even where the root stores it.
func listMemberRefusal(where, entityQN, name, near string) string {
	if name == "createdDate" || name == "changedDate" {
		return fmt.Sprintf(
			"%s: an aggregate cannot read the system member %s — it is not an attribute of %s, and mxbuild reports "+
				"CE0226 \"Entity '%s' does not contain attribute '%s.%s'.\"",
			where, name, entityQN, entityQN, entityQN, name)
	}
	return fmt.Sprintf(
		"%s: %s has no member %q%s%s — mxbuild reports this as CE1613 "+
			"\"The selected attribute '%s.%s' no longer exists\"",
		where, entityQN, name, near, flowSystemMemberHint(name), entityQN, name)
}

// listMemberWhere spells the list statement a member belongs to, as written:
// "find $l by Numbr", "sum $l by Totl".
func listMemberWhere(verb, list, member string) string {
	return fmt.Sprintf("%s $%s by %s", verb, list, member)
}

// flowSystemMemberHint names the spelling of a system member written another
// way in a flow: `owner` / `changedBy` are associations to System.User written
// qualified, and the dates are lower camel case.
func flowSystemMemberHint(name string) string {
	for _, spelling := range xpathSystemMemberSpellings {
		bare := strings.TrimPrefix(spelling, "System.")
		if spelling != name && strings.EqualFold(bare, strings.TrimPrefix(name, "System.")) {
			return fmt.Sprintf(" (the system member is spelled `%s`)", spelling)
		}
	}
	return ""
}

// nearMembersFor lists what the entity does have, capped so a wide entity does
// not bury the error. A typo is cheap to fix only when you can see what is
// there.
func nearMembersFor(b entityLookupBackend, entityQN string) string {
	entity, ok := findEntityByQN(b, entityQN)
	if !ok {
		return ""
	}
	var names []string
	for _, a := range entity.Attributes {
		if a != nil {
			names = append(names, a.Name)
		}
	}
	return nearMemberList(names)
}

// nearMemberList is the " — it has A, B" suffix for a list of attribute names.
func nearMemberList(names []string) string {
	const max = 10
	if len(names) == 0 {
		return ""
	}
	names = append([]string(nil), names...)
	sort.Strings(names)
	if len(names) > max {
		return fmt.Sprintf(" — it has %s and %d more",
			strings.Join(names[:max], ", "), len(names)-max)
	}
	return fmt.Sprintf(" — it has %s", strings.Join(names, ", "))
}
