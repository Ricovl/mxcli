// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Incremental CATALOG.SOURCE (ako/mxcli#1790).
//
// A project with a source index rebuilds it whenever the catalog goes stale,
// which is every save, and every rebuild described every document again. Most
// of them have not changed. A source build now records what it described from
// (each unit's content hash, and which document came from which unit), and the
// next build carries a document's row over when nothing its text depends on
// has changed:
//
//   - its own unit's bytes are unchanged. An entity is not a unit: its basis
//     is its own subtree of the domain model (#1803), and the rest of the
//     domain model is a unit of its own that, when it changes, re-describes
//     every entity in it;
//   - no document it references changed: a describe resolves names against
//     other documents (a called flow's return type, a data source's entity),
//     so a document whose refs name a changed one is described again;
//   - nothing else in the project changed. A change to any unit that is not
//     a described document — a folder, a module, the project settings, a
//     Java action — can move names or change what describes print, and is
//     not followed through the refs, so it makes the build describe
//     everything, as it always did;
//   - the same mxcli wrote the previous rows (the fingerprint): another
//     version may describe differently.
//
// Anything the plan cannot vouch for is described again; reuse is only ever
// an optimisation of a full rebuild, never a different answer.

// MetaSourceFingerprint records which mxcli build wrote the source rows.
const MetaSourceFingerprint = "source_fingerprint"

// unitHasher is the optional reader capability incremental source needs: a
// hash of every unit's current contents, keyed by unit ID.
type unitHasher interface {
	UnitContentHashes() (map[string]string, error)
}

// PreviousSource is what the last source build recorded.
type PreviousSource struct {
	rows        map[string]previousRow // by ElementId
	unitHashes  map[string]string      // unit ID -> content hash, every unit
	described   map[string]bool        // unit IDs that held a described document
	unitMembers map[string][]string    // unit ID -> qualified names described from it
}

type previousRow struct {
	qn, objType, text, module string
}

// LoadPreviousSource reads the basis of the source index cached at path, or
// returns nil and why it cannot be reused.
func LoadPreviousSource(path, fingerprint string) (*PreviousSource, string) {
	if path == "" || fingerprint == "" {
		return nil, "no cache to reuse"
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err.Error()
	}
	defer db.Close()

	var mode, fp string
	_ = db.QueryRow(`SELECT Value FROM catalog_meta WHERE Key = ?`, MetaBuildMode).Scan(&mode)
	_ = db.QueryRow(`SELECT Value FROM catalog_meta WHERE Key = ?`, MetaSourceFingerprint).Scan(&fp)
	if mode != "source" {
		return nil, "the cache has no source index"
	}
	if fp != fingerprint {
		return nil, "the source index was written by another mxcli build"
	}

	p := &PreviousSource{
		rows:        map[string]previousRow{},
		unitHashes:  map[string]string{},
		described:   map[string]bool{},
		unitMembers: map[string][]string{},
	}
	units, err := db.Query(`SELECT UnitId, Hash, Described FROM source_units`)
	if err != nil {
		return nil, "the cache records no source basis"
	}
	for units.Next() {
		var id, hash string
		var described int
		if err := units.Scan(&id, &hash, &described); err != nil {
			units.Close()
			return nil, err.Error()
		}
		p.unitHashes[id] = hash
		p.described[id] = described != 0
	}
	units.Close()
	if len(p.unitHashes) == 0 {
		return nil, "the cache records no source basis"
	}

	elems, err := db.Query(`SELECT UnitId, QualifiedName FROM source_elements`)
	if err != nil {
		return nil, err.Error()
	}
	for elems.Next() {
		var unit, qn string
		if err := elems.Scan(&unit, &qn); err != nil {
			elems.Close()
			return nil, err.Error()
		}
		p.unitMembers[unit] = append(p.unitMembers[unit], qn)
	}
	elems.Close()

	rows, err := db.Query(`SELECT ElementId, QualifiedName, ObjectType, SourceText, ModuleName FROM source`)
	if err != nil {
		return nil, err.Error()
	}
	defer rows.Close()
	for rows.Next() {
		var id, qn, objType, text, module sql.NullString
		if err := rows.Scan(&id, &qn, &objType, &text, &module); err != nil {
			return nil, err.Error()
		}
		if id.String == "" {
			continue
		}
		p.rows[id.String] = previousRow{qn.String, objType.String, text.String, module.String}
	}
	return p, ""
}

// SetPreviousSource gives the build the previous source index to reuse from.
func (b *Builder) SetPreviousSource(p *PreviousSource) {
	b.prevSource = p
}

// SetSourceFingerprint identifies the mxcli build that writes the source rows.
func (b *Builder) SetSourceFingerprint(fp string) {
	b.sourceFingerprint = fp
}

// sourceBasis is what each unit's describe depends on of the unit itself:
// its content hash and its container. The container is a column of the Unit
// table, not part of the bytes, so moving a document into another folder
// leaves its content hash as it was — and a describe prints the folder. Nil
// when the reader cannot hash units, which reuses and records nothing.
func (b *Builder) sourceBasis() map[string]string {
	h, ok := b.reader.(unitHasher)
	if !ok {
		return nil
	}
	hashes, err := h.UnitContentHashes()
	if err != nil {
		return nil
	}
	units, err := b.reader.ListUnits()
	if err != nil {
		return nil
	}
	for _, u := range units {
		id := string(u.ID)
		if hash, ok := hashes[id]; ok {
			hashes[id] = hash + "|" + string(u.ContainerID)
		}
	}
	b.entityBasis(hashes)
	return hashes
}

// entityBasis splits each domain model's basis per entity (#1803). Adding an
// attribute to one entity changes its domain model's unit, and with the unit
// as the basis every entity of the module was described again, along with
// everything that references any of them. An entity's describe reads only
// its own subtree — attributes, rules, indexes, handlers and access rules all
// live in it, and associations are not printed — so the entity is keyed on a
// hash of that subtree and its container, and the domain model's own entry
// covers the rest of it. A change there (an association, an annotation)
// still re-describes every entity of the module: planSourceReuse treats the
// domain model as each entity's parent.
//
// The entities are found in the stored bytes, not a decoded list, so an
// incremental build that rebuilds one module does not decode every domain
// model. A domain model whose contents do not parse keeps its whole-unit
// basis, and its entities have none of their own: each is then described
// again whenever the domain model changes, through its parent.
func (b *Builder) entityBasis(hashes map[string]string) {
	b.entityDM = map[string]string{}
	units, err := b.reader.ListRawUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return
	}
	for _, u := range units {
		dmID := string(u.ID)
		if _, ok := hashes[dmID]; !ok {
			continue
		}
		residual, entities, ok := splitDomainModel(u.Contents)
		if !ok {
			continue
		}
		for id, h := range entities {
			hashes[id] = h + "|" + dmID
			b.entityDM[id] = dmID
		}
		hashes[dmID] = residual + "|" + string(u.ContainerID)
	}
}

// splitDomainModel hashes a domain model's entities by ID, and everything
// else in it as one residual. ok is false when the contents do not parse.
func splitDomainModel(contents []byte) (residual string, entities map[string]string, ok bool) {
	elems, err := bson.Raw(contents).Elements()
	if err != nil {
		return "", nil, false
	}
	rest := sha256.New()
	entities = map[string]string{}
	for _, e := range elems {
		if e.Key() != "Entities" {
			rest.Write(e)
			continue
		}
		arr, isArr := e.Value().ArrayOK()
		if !isArr {
			return "", nil, false
		}
		vals, err := arr.Values()
		if err != nil {
			return "", nil, false
		}
		for _, v := range vals {
			doc, isDoc := v.DocumentOK()
			if !isDoc {
				continue // the array's leading version marker
			}
			_, id, isBin := doc.Lookup("$ID").BinaryOK()
			if !isBin {
				return "", nil, false
			}
			sum := sha256.Sum256(doc)
			entities[formatGUID(id)] = hex.EncodeToString(sum[:])
		}
	}
	return hex.EncodeToString(rest.Sum(nil)), entities, true
}

// reasonOtherUnitChanged is why a source build describes every document: a
// unit changed that no describe covers.
const reasonOtherUnitChanged = "a unit other than a described document changed"

// sourceReusePlan says, per item, whether its previous row can be carried
// over; reused[i] is that row's text.
type sourceReusePlan struct {
	reused map[int]previousRow
	reason string // why nothing was reused, when nothing was
}

// planSourceReuse decides which items keep their previous row. units maps
// each item to the basis it is described from (its unit, or for an entity the
// entity itself), parents to the unit that holds it when that is another one
// ("" otherwise): a change to the parent changes the item. hashes is every
// basis now.
func (b *Builder) planSourceReuse(items []sourceItem, units, parents []string, hashes map[string]string) sourceReusePlan {
	prev := b.prevSource
	if prev == nil {
		return sourceReusePlan{reason: "no previous source index"}
	}
	if hashes == nil {
		return sourceReusePlan{reason: "unit hashes unavailable"}
	}

	itemUnit := map[string]bool{}
	for i, u := range units {
		itemUnit[u] = true
		if parents[i] != "" {
			itemUnit[parents[i]] = true
		}
	}
	changed := map[string]bool{}
	for id, h := range hashes {
		if prev.unitHashes[id] != h {
			changed[id] = true
		}
	}
	for id := range prev.unitHashes {
		if _, ok := hashes[id]; !ok {
			changed[id] = true
		}
	}
	changedItem := func(i int) bool {
		return changed[units[i]] || (parents[i] != "" && changed[parents[i]])
	}
	for id := range changed {
		if !itemUnit[id] && !prev.described[id] {
			return sourceReusePlan{reason: reasonOtherUnitChanged}
		}
	}

	// Names a describe may have resolved against: every document described
	// from a changed unit, now and before (a rename or a delete).
	var names []string
	for i, it := range items {
		if changedItem(i) {
			names = append(names, it.qn)
		}
	}
	for id := range changed {
		names = append(names, prev.unitMembers[id]...)
	}
	dependents, err := b.sourceDependents(names)
	if err != nil {
		return sourceReusePlan{reason: "references unavailable: " + err.Error()}
	}

	plan := sourceReusePlan{reused: map[int]previousRow{}}
	for i, it := range items {
		if changedItem(i) || dependents[it.qn] {
			continue
		}
		row, ok := prev.rows[it.id]
		if !ok || row.qn != it.qn || row.objType != it.objType || row.module != it.moduleName {
			continue
		}
		plan.reused[i] = row
	}
	return plan
}

// sourceDependents is the qualified names of the documents whose refs name
// one of names, or a member of one (`M.Entity.Attribute` under `M.Entity`).
// The refs are this build's, written by buildReferences before buildSource.
func (b *Builder) sourceDependents(names []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(names) == 0 {
		return out, nil
	}
	stmt, err := b.tx.Prepare(`SELECT DISTINCT SourceName FROM refs
		WHERE TargetName = ? OR substr(TargetName, 1, length(?) + 1) = ? || '.'`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		rows, err := stmt.Query(n, n, n)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var src string
			if err := rows.Scan(&src); err != nil {
				rows.Close()
				return nil, err
			}
			out[src] = true
		}
		rows.Close()
	}
	return out, nil
}

// persistSourceBasis records what this build described from, so the next one
// can tell what changed.
func (b *Builder) persistSourceBasis(items []sourceItem, units, parents []string, hashes map[string]string) error {
	if hashes == nil || b.sourceFingerprint == "" {
		return nil
	}
	described := map[string]bool{}
	for i, u := range units {
		described[u] = true
		if parents[i] != "" {
			described[parents[i]] = true
		}
	}
	ustmt, err := b.tx.Prepare(`INSERT OR REPLACE INTO source_units (UnitId, Hash, Described) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ustmt.Close()
	// Each row belongs to the unit that holds it — an entity's to its domain
	// model — and an incremental build writes only the rebuilt units' rows.
	for id, h := range hashes {
		owner := id
		if dm := b.entityDM[id]; dm != "" {
			owner = dm
		}
		if !b.ownRows(owner) {
			continue
		}
		d := 0
		if described[id] {
			d = 1
		}
		if _, err := ustmt.Exec(id, h, d); err != nil {
			return err
		}
	}
	b.disown()
	estmt, err := b.tx.Prepare(`INSERT INTO source_elements (ElementId, UnitId, QualifiedName, ObjectType) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer estmt.Close()
	for i, it := range items {
		b.own(sourceOwner(it, parents[i]))
		if _, err := estmt.Exec(it.id, units[i], it.qn, it.objType); err != nil {
			return err
		}
	}
	b.disown()
	_, err = b.tx.Exec(`INSERT OR REPLACE INTO catalog_meta (Key, Value) VALUES (?, ?)`, MetaSourceFingerprint, b.sourceFingerprint)
	return err
}

// sourceReuseSummary is the progress line for a build that reused rows.
func sourceReuseSummary(reused, described int) string {
	return fmt.Sprintf("source: %d reused, %d described", reused, described)
}

// sourceOwner is the unit an item's rows come from: the unit that holds it
// (an entity's domain model) when that is another one, else the item.
func sourceOwner(it sourceItem, parent string) string {
	if parent != "" {
		return parent
	}
	return it.id
}

// sourceParentOf is the unit that holds an item when that is not the item
// itself: an entity's domain model. Every other source document is its own
// unit, and its basis is that unit.
func sourceParentOf(entityUnit map[string]string, it sourceItem) string {
	if it.objType == SourceEntity {
		return entityUnit[it.id]
	}
	return ""
}
