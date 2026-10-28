// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A member name must be unique along an entity's generalization chain
// (ako/mxcli#1645). mxbuild reports a second one as
//
//	CE0069 "Duplicate member name 'X'."
//
// and that is the only place it was reported: check passed it and exec wrote
// it. What counts, measured on mx 11.13.0 in a blank app (every case below is
// one CE0069, every "no" builds clean):
//
//   - an attribute or association whose name an ANCESTOR's attribute or
//     association already has — attribute/attribute, association/association
//     (cross-module), attribute/association both ways, two generalization
//     levels up, and System entities' members too: `extends System.User`
//     with `Name` collides with System.User.Name, with `UserRoles` with the
//     association System.UserRoles; `extends System.FileDocument` with `Size`.
//   - the other direction: adding to a generalization a member one of its
//     SPECIALIZATIONS already declares.
//   - an attribute and an association of the SAME entity.
//   - a rename into such a name — of an attribute or of an association.
//   - an association is a member of its FROM entity; under owner Both it is a
//     member of its TO entity as well (reference and reference set alike).
//     Under owner Default, an attribute on the TO entity named like it: no.
//   - names that differ only in case: no. The comparison is exact.
//   - two siblings declaring the same name, or an unrelated entity: no.
//
// memberWorld is the slice of the domain model that decides it, and
// memberNameClash the one verdict. exec judges a statement against the project
// as it stands (projectMemberWorld); check judges each statement against the
// project with the script's earlier statements applied (validateMemberNameClashes),
// because the ordinary shape is one script that creates the generalization and
// the specialization both.

// memberWorld holds each entity's generalization and own attributes, and each
// association's ends. The members of an entity are derived from both.
type memberWorld struct {
	entities map[string]*worldEntity // keyed by exact qualified name
	assocs   map[string]*worldAssoc  // keyed by exact qualified name
	// uneditable are modules a bulk ALTER ENTITIES without IN skips (System,
	// Marketplace) — see resolveBulkEntityTargets.
	uneditable map[string]bool
}

type worldEntity struct {
	gen         string
	attrs       []string
	persistable bool
	// notBulk marks a view or external entity, which ALTER ENTITIES never
	// targets (entityMatchesFilter).
	notBulk bool
}

type worldAssoc struct {
	name string
	from string // entity qualified name
	to   string // entity qualified name
	both bool   // owner Both: a member of the TO entity too
}

// worldMember is one member of an entity: what it is called and what it is.
type worldMember struct {
	name  string
	owner string // the entity that has it as its own member
	ref   string // Module.Entity.Attribute, or Module.Association
	assoc bool
}

func (m worldMember) kind() string {
	if m.assoc {
		return "association"
	}
	return "attribute"
}

func newMemberWorld() *memberWorld {
	return &memberWorld{
		entities:   map[string]*worldEntity{},
		assocs:     map[string]*worldAssoc{},
		uneditable: map[string]bool{},
	}
}

// projectMemberWorld reads the member world from the connected project. It
// includes the System module, whose entities' members a specialization of one
// inherits like any other's. Returns nil when the domain model cannot be read,
// which every caller takes as "nothing to judge".
func projectMemberWorld(ctx *ExecContext) *memberWorld {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil
	}
	moduleNames := make(map[model.ID]string, len(modules))
	w := newMemberWorld()
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
		if isUneditableModule(m) {
			w.uneditable[m.Name] = true
		}
	}
	entityQN := map[model.ID]string{}
	for _, dm := range dms {
		mod := moduleNames[dm.ContainerID]
		if mod == "" {
			continue
		}
		for _, e := range dm.Entities {
			if e == nil {
				continue
			}
			qn := mod + "." + e.Name
			entityQN[e.ID] = qn
			we := &worldEntity{gen: e.GeneralizationRef, persistable: e.Persistable,
				notBulk: e.Source != "" || e.OqlQuery != ""}
			for _, a := range e.Attributes {
				if a != nil {
					we.attrs = append(we.attrs, a.Name)
				}
			}
			w.entities[qn] = we
		}
	}
	for _, dm := range dms {
		mod := moduleNames[dm.ContainerID]
		if mod == "" {
			continue
		}
		for _, a := range dm.Associations {
			if a == nil {
				continue
			}
			w.assocs[mod+"."+a.Name] = &worldAssoc{
				name: a.Name, from: entityQN[a.ParentID], to: entityQN[a.ChildID],
				both: a.Owner == domainmodel.AssociationOwnerBoth,
			}
		}
		for _, ca := range dm.CrossAssociations {
			if ca == nil {
				continue
			}
			w.assocs[mod+"."+ca.Name] = &worldAssoc{
				name: ca.Name, from: entityQN[ca.ParentID], to: ca.ChildRef,
				both: ca.Owner == domainmodel.AssociationOwnerBoth,
			}
		}
	}
	// A backend that lists no System domain model still has the System
	// entities: take them from modelsdk/meta, as a project-free check does.
	if _, ok := w.entities["System.User"]; !ok {
		sys := systemMemberWorld()
		for qn, e := range sys.entities {
			if _, have := w.entities[qn]; !have {
				w.entities[qn] = e
			}
		}
		for qn, a := range sys.assocs {
			if _, have := w.assocs[qn]; !have {
				w.assocs[qn] = a
			}
		}
		w.uneditable["System"] = true
	}
	return w
}

// members lists an entity's own members: its attributes, then the associations
// it is an end of as a member (FROM always, TO under owner Both), by name.
func (w *memberWorld) members(qn string) []worldMember {
	var out []worldMember
	if e := w.entities[qn]; e != nil {
		for _, a := range e.attrs {
			out = append(out, worldMember{name: a, owner: qn, ref: qn + "." + a})
		}
	}
	var assocs []worldMember
	for aqn, a := range w.assocs {
		if a.from == qn || (a.both && a.to == qn) {
			assocs = append(assocs, worldMember{name: a.name, owner: qn, ref: aqn, assoc: true})
		}
	}
	sort.Slice(assocs, func(i, j int) bool { return assocs[i].ref < assocs[j].ref })
	return append(out, assocs...)
}

// ancestors walks the generalization chain above qn, nearest first, as far as
// it resolves in the world.
func (w *memberWorld) ancestors(qn string) []string {
	var out []string
	seen := map[string]bool{qn: true}
	for e := w.entities[qn]; e != nil && e.gen != "" && !seen[e.gen]; e = w.entities[e.gen] {
		seen[e.gen] = true
		if w.entities[e.gen] == nil {
			break
		}
		out = append(out, e.gen)
	}
	return out
}

// descendants lists every entity whose chain passes through qn, nearest first.
func (w *memberWorld) descendants(qn string) []string {
	children := map[string][]string{}
	for cqn, e := range w.entities {
		if e.gen != "" {
			children[e.gen] = append(children[e.gen], cqn)
		}
	}
	var out []string
	seen := map[string]bool{qn: true}
	queue := []string{qn}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		kids := children[cur]
		sort.Strings(kids)
		for _, k := range kids {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
				queue = append(queue, k)
			}
		}
	}
	return out
}

// memberClash is a member already holding the name a statement gives another.
type memberClash struct {
	subject  string      // the entity gaining the new member
	existing worldMember // the member that already has the name
	relation string      // "own", "ancestor" or "descendant"
	// via names the entity whose new generalization brings the inherited
	// member, when that — not the member itself — is what the statement changes.
	via string
}

// findClash looks for a member called name anywhere on subject's chain — its
// own members, its ancestors', its specializations' — other than one skip
// accepts. The comparison is exact: mxbuild does not report two names that
// differ only in case.
func (w *memberWorld) findClash(subject, name string, skip func(worldMember) bool) *memberClash {
	look := func(qn, relation string) *memberClash {
		for _, m := range w.members(qn) {
			if m.name == name && (skip == nil || !skip(m)) {
				return &memberClash{subject: subject, existing: m, relation: relation}
			}
		}
		return nil
	}
	if c := look(subject, "own"); c != nil {
		return c
	}
	for _, a := range w.ancestors(subject) {
		if c := look(a, "ancestor"); c != nil {
			return c
		}
	}
	for _, d := range w.descendants(subject) {
		if c := look(d, "descendant"); c != nil {
			return c
		}
	}
	return nil
}

// clashError words a clash for the member about to be added: what it is, the
// member that already has its name and where that member comes from, and the
// way out.
func clashError(newKind, newRef, name string, c *memberClash) error {
	e := c.existing
	ce := fmt.Sprintf("mxbuild rejects a second one with CE0069 \"Duplicate member name '%s'.\"", name)
	var why, fix string
	switch c.relation {
	case "own":
		why = fmt.Sprintf("%s already has %s %s — an entity's attributes and associations share one name space, so %s",
			c.subject, e.kind(), e.ref, ce)
		fix = fmt.Sprintf("give the %s another name", newKind)
	case "ancestor":
		inherits := "inherits"
		if c.via != "" {
			inherits = "would inherit"
		}
		why = fmt.Sprintf("%s %s '%s' from %s (%s %s); a member name must be unique along the generalization chain, so %s",
			c.subject, inherits, name, e.owner, e.kind(), e.ref, ce)
		if c.via != "" {
			why = fmt.Sprintf("through the new generalization of %s, %s", c.via, why)
		}
		fix = fmt.Sprintf("give the %s another name", newKind)
		if !e.assoc && newKind == "attribute" {
			fix += ", or drop it and use the inherited one"
		}
	default:
		why = fmt.Sprintf("%s's specialization %s already has '%s' (%s %s) and would inherit this one too; a member name must be unique along the generalization chain, so %s",
			c.subject, e.owner, name, e.kind(), e.ref, ce)
		fix = fmt.Sprintf("give the %s another name, or rename or drop %s first", newKind, e.ref)
	}
	return mdlerrors.NewValidationf("%s %s: %s — %s", newKind, newRef, why, fix)
}

// memberNameClash is the verdict check and exec share: the error for a
// statement that would give some entity's generalization chain two members of
// one name, judged against w; nil when it adds no member, or none clashes, or
// w is nil. A statement exec refuses for another reason — the entity is
// missing, the attribute or association already exists — is left to that
// refusal rather than reported twice.
func memberNameClash(w *memberWorld, stmt ast.Statement) error {
	if w == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		return w.entityDeclClash(s)
	case *ast.AlterEntityStmt:
		return w.alterEntityClash(s)
	case *ast.CreateAssociationStmt:
		return w.createAssociationClash(s)
	case *ast.AlterAssociationStmt:
		return w.alterAssociationClash(s)
	case *ast.RenameStmt:
		if s.ObjectType == "association" {
			return w.renameAssociationClash(s)
		}
	}
	return nil
}

// declaredAttrNames are the attributes a declaration stores: its AutoX
// pseudo-types become entity flags, not attributes.
func declaredAttrNames(attrs []ast.Attribute) []string {
	var out []string
	for _, a := range attrs {
		if systemMemberOfPseudoType(a.Type.Kind) != "" {
			continue
		}
		out = append(out, a.Name)
	}
	return out
}

func (w *memberWorld) entityDeclClash(s *ast.CreateEntityStmt) error {
	qn := s.Name.String()
	old, exists := w.entities[qn]
	if exists && (s.IfNotExists || !s.CreateOrModify) {
		return nil // skipped, or refused as already existing
	}
	gen := ""
	if s.Generalization != nil {
		gen = s.Generalization.String()
	}
	declared := declaredAttrNames(s.Attributes)
	had := map[string]bool{}
	if exists {
		for _, a := range old.attrs {
			had[a] = true
		}
	}
	hyp := w.withEntity(qn, &worldEntity{gen: gen, attrs: declared, persistable: s.Kind != ast.EntityNonPersistent})

	// A declared attribute the entity did not have meets everything on its
	// chain. Another declared attribute of the same name is not this rule's —
	// the attribute list itself is.
	for _, name := range declared {
		if had[name] {
			continue
		}
		if c := hyp.findClash(qn, name, func(m worldMember) bool { return m.owner == qn && !m.assoc }); c != nil {
			return clashError("attribute", qn+"."+name, name, c)
		}
	}
	if exists && old.gen == gen {
		return nil // the rest of the chain is as it was
	}
	// A new generalization brings new inherited members, which every member
	// already below it meets: the entity's own, and its specializations'.
	above := hyp.ancestors(qn)
	meet := func(subject string, m worldMember) error {
		for _, a := range above {
			for _, o := range hyp.members(a) {
				if o.name == m.name {
					c := &memberClash{subject: subject, existing: o, relation: "ancestor"}
					if subject != qn {
						c.via = qn
					}
					return clashError(m.kind(), m.ref, m.name, c)
				}
			}
		}
		return nil
	}
	for _, d := range append([]string{qn}, hyp.descendants(qn)...) {
		for _, m := range hyp.members(d) {
			if err := meet(d, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *memberWorld) alterEntityClash(s *ast.AlterEntityStmt) error {
	qn := s.Name.String()
	e := w.entities[qn]
	if e == nil {
		return nil // refused as not found
	}
	switch s.Operation {
	case ast.AlterEntityAddAttribute:
		if s.Attribute == nil || systemMemberOfPseudoType(s.Attribute.Type.Kind) != "" {
			return nil
		}
		name := s.Attribute.Name
		if containsString(e.attrs, name) {
			return nil // skipped (IF NOT EXISTS) or refused as existing
		}
		if c := w.findClash(qn, name, nil); c != nil {
			return clashError("attribute", qn+"."+name, name, c)
		}
	case ast.AlterEntityRenameAttribute:
		if s.NewName == s.AttributeName || !containsString(e.attrs, s.AttributeName) || containsString(e.attrs, s.NewName) {
			return nil
		}
		self := qn + "." + s.AttributeName
		if c := w.findClash(qn, s.NewName, func(m worldMember) bool { return m.ref == self }); c != nil {
			return clashError("attribute", qn+"."+s.NewName, s.NewName, c)
		}
	}
	return nil
}

// createAssociationEnds resolves a CREATE ASSOCIATION's FROM and TO entities the way
// exec does: an unqualified end is in the association's module.
func createAssociationEnds(s *ast.CreateAssociationStmt) (from, to string) {
	pm, cm := s.Parent.Module, s.Child.Module
	if pm == "" {
		pm = s.Name.Module
	}
	if cm == "" {
		cm = s.Name.Module
	}
	return pm + "." + s.Parent.Name, cm + "." + s.Child.Name
}

// crossModuleEnds reports whether a statement's two ends are in different
// modules — exec's isCrossModule, which decides whether OR MODIFY moves the TO
// end.
func crossModuleEnds(from, to string) bool {
	fm, _, _ := strings.Cut(from, ".")
	tm, _, _ := strings.Cut(to, ".")
	return fm != tm
}

// owners are the entities an association is a member of.
func (a *worldAssoc) owners() []string {
	if a.both && a.to != "" && a.to != a.from {
		return []string{a.from, a.to}
	}
	return []string{a.from}
}

func (w *memberWorld) createAssociationClash(s *ast.CreateAssociationStmt) error {
	qn := s.Name.String()
	from, to := createAssociationEnds(s)
	both := s.Owner == ast.OwnerBoth
	before := map[string]bool{}
	after := &worldAssoc{name: s.Name.Name, from: from, to: to, both: both}
	if old := w.assocs[qn]; old != nil {
		if s.IfNotExists || !s.CreateOrModify {
			return nil // skipped, or refused as already existing
		}
		for _, o := range old.owners() {
			before[o] = true
		}
		// OR MODIFY keeps the FROM end; it moves the TO end of a cross-module
		// association only (execCreateAssociation).
		after.from = old.from
		if !crossModuleEnds(from, to) {
			after.to = old.to
		}
	}
	for _, o := range after.owners() {
		if o == "" || before[o] || w.entities[o] == nil {
			continue
		}
		if c := w.findClash(o, s.Name.Name, func(m worldMember) bool { return m.ref == qn }); c != nil {
			return clashError("association", qn, s.Name.Name, c)
		}
	}
	return nil
}

func (w *memberWorld) alterAssociationClash(s *ast.AlterAssociationStmt) error {
	if s.Operation != ast.AlterAssociationSetOwner || s.Owner != ast.OwnerBoth {
		return nil
	}
	qn := s.Name.String()
	a := w.assocs[qn]
	if a == nil || a.both || a.to == "" || a.to == a.from || w.entities[a.to] == nil {
		return nil
	}
	if c := w.findClash(a.to, a.name, func(m worldMember) bool { return m.ref == qn }); c != nil {
		return clashError("association", qn, a.name, c)
	}
	return nil
}

func (w *memberWorld) renameAssociationClash(s *ast.RenameStmt) error {
	oldQN := s.Name.String()
	a := w.assocs[oldQN]
	if a == nil || s.NewName == a.name || w.assocs[s.Name.Module+"."+s.NewName] != nil {
		return nil // refused as not found, or as a name the module already has
	}
	for _, o := range a.owners() {
		if w.entities[o] == nil {
			continue
		}
		if c := w.findClash(o, s.NewName, func(m worldMember) bool { return m.ref == oldQN }); c != nil {
			return clashError("association", s.Name.Module+"."+s.NewName, s.NewName, c)
		}
	}
	return nil
}

// withEntity is w with one entity replaced, sharing everything else.
func (w *memberWorld) withEntity(qn string, e *worldEntity) *memberWorld {
	entities := make(map[string]*worldEntity, len(w.entities)+1)
	for k, v := range w.entities {
		entities[k] = v
	}
	entities[qn] = e
	return &memberWorld{entities: entities, assocs: w.assocs, uneditable: w.uneditable}
}

// apply replays a statement's effect on the members, for check's ordered pass.
// It follows what exec does closely enough that a later statement is judged
// against the model it will meet; a statement it does not model leaves the
// world as it was, which can only make check quieter, never refuse more.
func (w *memberWorld) apply(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		qn := s.Name.String()
		if _, exists := w.entities[qn]; exists && (s.IfNotExists || !s.CreateOrModify) {
			return
		}
		gen := ""
		if s.Generalization != nil {
			gen = s.Generalization.String()
		}
		w.entities[qn] = &worldEntity{gen: gen, attrs: declaredAttrNames(s.Attributes), persistable: s.Kind != ast.EntityNonPersistent}
	case *ast.CreateViewEntityStmt:
		qn := s.Name.String()
		if _, exists := w.entities[qn]; exists && (s.IfNotExists || !(s.CreateOrModify || s.CreateOrReplace)) {
			return
		}
		e := &worldEntity{notBulk: true}
		for _, a := range s.Attributes {
			e.attrs = append(e.attrs, a.Name)
		}
		w.entities[qn] = e
	case *ast.CreateExternalEntityStmt:
		qn := s.Name.String()
		if _, exists := w.entities[qn]; exists && (s.IfNotExists || !s.CreateOrModify) {
			return
		}
		w.entities[qn] = &worldEntity{attrs: declaredAttrNames(s.Attributes), persistable: true, notBulk: true}
	case *ast.DropEntityStmt:
		// The backend's DeleteEntity removes every association with the
		// entity at either end, in every module, along with it.
		qn := s.Name.String()
		delete(w.entities, qn)
		for aqn, a := range w.assocs {
			if a.from == qn || a.to == qn {
				delete(w.assocs, aqn)
			}
		}
	case *ast.AlterEntityStmt:
		e := w.entities[s.Name.String()]
		if e == nil {
			return
		}
		switch s.Operation {
		case ast.AlterEntityAddAttribute:
			if s.Attribute != nil && systemMemberOfPseudoType(s.Attribute.Type.Kind) == "" && !containsString(e.attrs, s.Attribute.Name) {
				e.attrs = append(append([]string(nil), e.attrs...), s.Attribute.Name)
			}
		case ast.AlterEntityDropAttribute:
			var kept []string
			for _, a := range e.attrs {
				if a != s.AttributeName {
					kept = append(kept, a)
				}
			}
			e.attrs = kept
		case ast.AlterEntityRenameAttribute:
			renamed := append([]string(nil), e.attrs...)
			for i, a := range renamed {
				if a == s.AttributeName {
					renamed[i] = s.NewName
				}
			}
			e.attrs = renamed
		}
	case *ast.AlterEntitiesStmt:
		for _, qn := range w.bulkTargets(s) {
			mod, name, _ := strings.Cut(qn, ".")
			for _, action := range s.Actions {
				per := *action
				per.Name = ast.QualifiedName{Module: mod, Name: name}
				w.apply(&per)
			}
		}
	case *ast.CreateAssociationStmt:
		qn := s.Name.String()
		from, to := createAssociationEnds(s)
		if old := w.assocs[qn]; old != nil {
			if s.IfNotExists || !s.CreateOrModify {
				return
			}
			from = old.from
			if !crossModuleEnds(from, to) {
				to = old.to
			}
		}
		w.assocs[qn] = &worldAssoc{name: s.Name.Name, from: from, to: to, both: s.Owner == ast.OwnerBoth}
	case *ast.AlterAssociationStmt:
		if a := w.assocs[s.Name.String()]; a != nil && s.Operation == ast.AlterAssociationSetOwner {
			w.assocs[s.Name.String()] = &worldAssoc{name: a.name, from: a.from, to: a.to, both: s.Owner == ast.OwnerBoth}
		}
	case *ast.DropAssociationStmt:
		delete(w.assocs, s.Name.String())
	case *ast.RenameStmt:
		if s.DryRun {
			return
		}
		switch s.ObjectType {
		case "association":
			if a := w.assocs[s.Name.String()]; a != nil {
				delete(w.assocs, s.Name.String())
				w.assocs[s.Name.Module+"."+s.NewName] = &worldAssoc{name: s.NewName, from: a.from, to: a.to, both: a.both}
			}
		case "entity":
			w.renameEntity(s.Name.String(), s.Name.Module+"."+s.NewName)
		case "module":
			// RENAME MODULE names the module in Name.Name or Name.Module.
			oldMod := s.Name.Name
			if s.Name.Module != "" {
				oldMod = s.Name.Module
			}
			var entities, assocs []string
			for qn := range w.entities {
				if strings.HasPrefix(qn, oldMod+".") {
					entities = append(entities, qn)
				}
			}
			for qn := range w.assocs {
				if strings.HasPrefix(qn, oldMod+".") {
					assocs = append(assocs, qn)
				}
			}
			for _, qn := range entities {
				w.renameEntity(qn, s.NewName+qn[len(oldMod):])
			}
			for _, qn := range assocs {
				a := w.assocs[qn]
				delete(w.assocs, qn)
				w.assocs[s.NewName+qn[len(oldMod):]] = a
			}
		}
	case *ast.MoveStmt:
		if s.DocumentType == ast.DocumentTypeEntity && s.TargetModule != "" && s.TargetModule != s.Name.Module {
			w.renameEntity(s.Name.String(), s.TargetModule+"."+s.Name.Name)
		}
	}
}

// renameEntity moves an entity to a new qualified name and repoints what
// refers to it: its specializations and its associations' ends.
func (w *memberWorld) renameEntity(oldQN, newQN string) {
	e := w.entities[oldQN]
	if e == nil || oldQN == newQN {
		return
	}
	delete(w.entities, oldQN)
	w.entities[newQN] = e
	for _, x := range w.entities {
		if x.gen == oldQN {
			x.gen = newQN
		}
	}
	for _, a := range w.assocs {
		if a.from == oldQN {
			a.from = newQN
		}
		if a.to == oldQN {
			a.to = newQN
		}
	}
}

// bulkTargets is resolveBulkEntityTargets over the world: the entities an
// ALTER ENTITIES statement adds to, after dropping those whose ancestor is
// also a target.
func (w *memberWorld) bulkTargets(s *ast.AlterEntitiesStmt) []string {
	var targets []ast.QualifiedName
	parentOf := map[string]string{}
	for qn, e := range w.entities {
		mod, name, ok := strings.Cut(qn, ".")
		if !ok {
			continue
		}
		if s.Module != "" && !strings.EqualFold(mod, s.Module) {
			continue
		}
		if s.Module == "" && (w.uneditable[mod] || mod == "System") {
			continue
		}
		if e.notBulk {
			continue
		}
		switch s.Filter {
		case ast.EntityFilterPersistent:
			if !e.persistable {
				continue
			}
		case ast.EntityFilterNonPersistent:
			if e.persistable {
				continue
			}
		}
		targets = append(targets, ast.QualifiedName{Module: mod, Name: name})
		parentOf[qn] = e.gen
	}
	targets = dropInheritedTargets(targets, parentOf)
	out := make([]string, 0, len(targets))
	for _, t := range targets {
		out = append(out, t.String())
	}
	sort.Strings(out)
	return out
}

// validateMemberNameClashes is check's side of the rule against a project: the
// script replayed over the project's members in statement order.
func validateMemberNameClashes(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() || !programTouchesMembers(prog) {
		return nil
	}
	return replayMemberNameClashes(projectMemberWorld(ctx), prog)
}

// scriptMemberNameClashes is the same rule for `check` without a project
// (MDL-MEMBER01): the script replayed over the System module alone, whose
// members a specialization of a System entity inherits. A clash it reports is
// between members the script itself declares, or one of them and a System
// member, so it holds whatever the project turns out to contain; the clashes
// that need the project are left to `check -p` and exec.
func scriptMemberNameClashes(prog *ast.Program) []linter.Violation {
	if prog == nil || !programTouchesMembers(prog) {
		return nil
	}
	var out []linter.Violation
	w := systemMemberWorld()
	for i, stmt := range prog.Statements {
		err := w.judgeAndApply(stmt)
		if err == nil {
			continue
		}
		// The verdict's text ends with its way out; the report shows that as
		// the suggestion.
		msg, fix := err.Error(), ""
		if k := strings.LastIndex(msg, " — "); k >= 0 {
			msg, fix = msg[:k], msg[k+len(" — "):]
		}
		docType, name := memberStatementSubject(stmt)
		out = append(out, linter.Violation{
			RuleID:     "MDL-MEMBER01",
			Severity:   linter.SeverityError,
			Message:    fmt.Sprintf("statement %d: %s", i+1, msg),
			Suggestion: fix,
			Location:   linter.Location{Module: name.Module, DocumentType: docType, DocumentName: name.Name},
		})
	}
	return out
}

// replayMemberNameClashes judges each statement by the verdict exec gives, then
// applies it, in statement order.
func replayMemberNameClashes(w *memberWorld, prog *ast.Program) []error {
	if w == nil {
		return nil
	}
	var errs []error
	for i, stmt := range prog.Statements {
		if err := w.judgeAndApply(stmt); err != nil {
			errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
		}
	}
	return errs
}

// judgeAndApply judges one statement and, when it passes, applies it — a
// refused statement is not, since exec stops there. ALTER ENTITIES is judged
// per target, as exec runs it.
func (w *memberWorld) judgeAndApply(stmt ast.Statement) error {
	var err error
	if bulk, ok := stmt.(*ast.AlterEntitiesStmt); ok {
		err = w.bulkClash(w.bulkTargets(bulk), bulk.Actions)
	} else {
		err = memberNameClash(w, stmt)
	}
	if err == nil {
		w.apply(stmt)
	}
	return err
}

// memberStatementSubject names what a member-adding statement is about, for a
// report's location.
func memberStatementSubject(stmt ast.Statement) (string, ast.QualifiedName) {
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		return "entity", s.Name
	case *ast.AlterEntityStmt:
		return "entity", s.Name
	case *ast.AlterEntitiesStmt:
		return "module", ast.QualifiedName{Module: s.Module}
	case *ast.CreateAssociationStmt:
		return "association", s.Name
	case *ast.AlterAssociationStmt:
		return "association", s.Name
	case *ast.RenameStmt:
		return s.ObjectType, s.Name
	}
	return "", ast.QualifiedName{}
}

// systemMemberWorld is the System module's members as Studio Pro shows them,
// from modelsdk/meta — what a project-free check knows about the ancestors a
// script names.
func systemMemberWorld() *memberWorld {
	w := newMemberWorld()
	w.uneditable["System"] = true
	for _, e := range meta.ModelerSystemEntities() {
		we := &worldEntity{gen: e.Generalization, persistable: e.Persistable, notBulk: true}
		for _, a := range e.Attributes {
			we.attrs = append(we.attrs, a.Name)
		}
		w.entities["System."+e.Name] = we
	}
	for _, a := range meta.ModelerSystemAssociations() {
		w.assocs["System."+a.Name] = &worldAssoc{
			name: a.Name, from: "System." + a.Parent, to: "System." + a.Child, both: a.Owner == "Both",
		}
	}
	return w
}

// bulkClash judges ALTER ENTITIES target by target, applying each add before
// the next is judged — the order exec writes them in. It runs on a copy, so a
// refusal leaves w untouched. targets are exec's (resolveBulkEntityTargets) at
// exec time and the world's (bulkTargets) at check time.
func (w *memberWorld) bulkClash(targets []string, actions []*ast.AlterEntityStmt) error {
	if w == nil {
		return nil
	}
	sim := w.clone()
	for _, qn := range targets {
		mod, name, _ := strings.Cut(qn, ".")
		for _, action := range actions {
			per := *action
			per.Name = ast.QualifiedName{Module: mod, Name: name}
			if err := memberNameClash(sim, &per); err != nil {
				return fmt.Errorf("%s: %w", qn, err)
			}
			sim.apply(&per)
		}
	}
	return nil
}

// clone copies the world deeply enough that apply on the copy leaves w as it
// was.
func (w *memberWorld) clone() *memberWorld {
	c := newMemberWorld()
	for k, v := range w.entities {
		e := *v
		e.attrs = append([]string(nil), v.attrs...)
		c.entities[k] = &e
	}
	for k, v := range w.assocs {
		a := *v
		c.assocs[k] = &a
	}
	for k, v := range w.uneditable {
		c.uneditable[k] = v
	}
	return c
}

// programTouchesMembers reports whether a script has a statement that can add
// a member — so a script that does not pays nothing for reading the model.
func programTouchesMembers(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt, *ast.AlterEntityStmt, *ast.AlterEntitiesStmt,
			*ast.CreateAssociationStmt, *ast.AlterAssociationStmt:
			return true
		case *ast.RenameStmt:
			if s.ObjectType == "association" {
				return true
			}
		}
	}
	return false
}

// refuseMemberNameClash is exec's side: the same verdict, against the project
// as this statement finds it.
func refuseMemberNameClash(ctx *ExecContext, stmt ast.Statement) error {
	return memberNameClash(projectMemberWorld(ctx), stmt)
}
