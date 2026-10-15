// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"time"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Incremental refresh: when only existing flows changed since the catalog was
// built, their rows are deleted and rebuilt instead of the whole catalog.
// See docs/11-proposals/PROPOSAL_incremental_project_index.md, step 5.

// incrementalTypes are the unit types an incremental refresh adds, rebuilds
// and removes per unit; a domain model is rebuilt per module, and an added or
// removed folder holds no rows. Any other change needs a full build.
var incrementalTypes = map[string]bool{
	"Microflows$Microflow": true,
	"Microflows$Nanoflow":  true,
	"Microflows$Rule":      true,
	"Forms$Page":           true,
	"Forms$Snippet":        true,
}

// domainModelType is rebuilt per module: an edit, not an add or remove (which
// comes with a module added or removed).
const domainModelType = "DomainModels$DomainModel"

// createIncrementalTables makes the temp tables the builders' SQL filters on;
// empty in a full build.
func createIncrementalTables(tx CatalogTx) error {
	for _, q := range []string{
		`CREATE TEMP TABLE IF NOT EXISTS incremental_units (UnitId TEXT PRIMARY KEY)`,
		`CREATE TEMP TABLE IF NOT EXISTS incremental_modules (Name TEXT, Id TEXT)`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// UnitTables are the tables whose rows carry UnitId, the unit they were built
// from; an incremental refresh deletes a changed unit's rows from each.
var UnitTables = []string{
	"microflows_data", "microflow_parameters_data", "activities_data",
	"entities_data", "attributes_data", "associations_data", "entity_event_handlers_data",
	"pages_data", "snippets_data", "widgets_data",
	"xpath_expressions_data", "permissions", "refs", "strings",
}

// maxIncrementalUnits bounds an incremental refresh; past it a full build is
// as cheap and simpler.
const maxIncrementalUnits = 200

func (b *Builder) incremental() bool { return b.only != nil }

// incrementalFlag is "1" during an incremental refresh, for SQL that filters on it.
func (b *Builder) incrementalFlag() string {
	if b.incremental() {
		return "1"
	}
	return ""
}

// wants reports whether a unit's rows are being (re)built.
func (b *Builder) wants(unitID string) bool { return b.only == nil || b.only[unitID] }

// rawUnits is ListRawUnitsByType, limited to the changed units when incremental.
func (b *Builder) rawUnits(typeName string) ([]*types.RawUnit, error) {
	if !b.incremental() {
		return b.reader.ListRawUnitsByType(typeName)
	}
	r, ok := b.reader.(interface {
		GetRawUnitBytes(model.ID) ([]byte, error)
	})
	if !ok {
		return nil, fmt.Errorf("reader cannot read units by ID")
	}
	var out []*types.RawUnit
	for _, s := range b.onlyStamps {
		if typeName != "" && s.Type != typeName {
			continue
		}
		contents, err := r.GetRawUnitBytes(model.ID(s.ID))
		if err != nil {
			return nil, err
		}
		out = append(out, &types.RawUnit{ID: model.ID(s.ID), ContainerID: model.ID(s.ContainerID), Type: s.Type, Contents: contents})
	}
	return out, nil
}

// changedFlows decodes only the changed documents of one kind. Never nil: a nil
// cache means "not loaded" and would load every flow.
func changedFlows[T any](b *Builder, typeName string, get func(model.ID) (T, error)) ([]T, error) {
	out := []T{}
	for _, s := range b.onlyStamps {
		if s.Type != typeName {
			continue
		}
		v, err := get(model.ID(s.ID))
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// SetStamps gives a full build the unit stamps it is made from, taken before
// any unit is read, so a change during the build shows up next time.
func (b *Builder) SetStamps(stamps []types.UnitStamp) { b.stamps = stamps }

func (b *Builder) writeStamps(stamps []types.UnitStamp) error {
	stmt, err := b.tx.Prepare(`INSERT OR REPLACE INTO unit_stamps (UnitId, Type, ContainerId, Size, MtimeNs, Digest) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range stamps {
		digest := ""
		if s.MtimeNs == -1 {
			digest = b.unitDigest(s.ID)
		}
		if _, err := stmt.Exec(s.ID, s.Type, s.ContainerID, s.Size, s.MtimeNs, digest); err != nil {
			return err
		}
	}
	return nil
}

// unitDigest hashes a unit's file contents; "" when the reader cannot.
func (b *Builder) unitDigest(id string) string {
	if d, ok := b.reader.(interface{ UnitContentDigest([]string) string }); ok {
		return d.UnitContentDigest([]string{id})
	}
	return ""
}

// StampDiff is how a catalog's unit stamps differ from the current ones.
type StampDiff struct {
	Identical bool              // the catalog is current
	Changed   []types.UnitStamp // flows and domain models to rebuild
	Removed   []string          // flows whose rows to delete
	Reason    string            // set when only a full build will do
}

// DiffStamps compares the catalog's stored unit stamps with now. A racily
// clean stored stamp is compared by content, through digest. ok is false when
// the catalog has no stamps (MPR v1, or built before they existed).
func (c *Catalog) DiffStamps(now []types.UnitStamp, digest func(id string) string) (d StampDiff, ok bool) {
	type stored struct {
		types.UnitStamp
		digest string
	}
	old := map[string]stored{}
	rows, err := c.db.Query(`SELECT UnitId, Type, ContainerId, Size, MtimeNs, COALESCE(Digest, '') FROM unit_stamps`)
	if err != nil {
		return d, false
	}
	for rows.Next() {
		var st stored
		if rows.Scan(&st.ID, &st.Type, &st.ContainerID, &st.Size, &st.MtimeNs, &st.digest) == nil {
			old[st.ID] = st
		}
	}
	rows.Close()
	if len(old) == 0 || len(now) == 0 {
		return d, false
	}

	identical := len(old) == len(now)
	seen := make(map[string]bool, len(now))
	for _, n := range now {
		seen[n.ID] = true
		o, ok := old[n.ID]
		same := ok && o.Type == n.Type && o.ContainerID == n.ContainerID && o.Size == n.Size &&
			(o.MtimeNs != -1 && o.MtimeNs == n.MtimeNs || o.MtimeNs == -1 && o.digest != "" && o.digest == digest(n.ID))
		if same {
			continue
		}
		identical = false
		switch {
		case !ok && incrementalTypes[n.Type]:
			d.Changed = append(d.Changed, n)
		case !ok && n.Type == "Projects$Folder":
		case !ok:
			d.Reason = "a non-flow unit was added"
		case incrementalTypes[n.Type] && o.Type == n.Type:
			d.Changed = append(d.Changed, n)
		case n.Type == domainModelType && o.Type == n.Type && o.ContainerID == n.ContainerID:
			d.Changed = append(d.Changed, n) // rebuilt per module
		default:
			d.Reason = "a non-flow unit changed or moved"
		}
	}
	for id, o := range old {
		if seen[id] {
			continue
		}
		identical = false
		switch {
		case incrementalTypes[o.Type]:
			d.Removed = append(d.Removed, id)
		case o.Type == "Projects$Folder":
		default:
			d.Reason = "a non-flow unit was removed"
		}
	}
	d.Identical = identical
	return d, true
}

// PlanIncremental diffs the catalog's stamps against now and returns the
// units to rebuild and to delete, or the reason only a full build will do.
func (b *Builder) PlanIncremental(now []types.UnitStamp) (changed []types.UnitStamp, removed []string, reason string) {
	d, ok := b.catalog.DiffStamps(now, b.unitDigest)
	switch {
	case !ok:
		return nil, nil, "catalog has no unit stamps"
	case d.Reason != "":
		return nil, nil, d.Reason
	case len(d.Changed)+len(d.Removed) > maxIncrementalUnits:
		return nil, nil, "too many units changed"
	}
	var n int
	if b.catalog.db.QueryRow(`SELECT COUNT(*) FROM external_actions_data`).Scan(&n) == nil && n > 0 {
		return nil, nil, "project calls external actions (aggregated across flows)"
	}
	b.stamps = now
	b.removed = d.Removed
	return d.Changed, d.Removed, ""
}

// BuildIncremental rebuilds the rows of the changed flows in the catalog's own
// database, in one transaction.
func (b *Builder) BuildIncremental(changed []types.UnitStamp, progress ProgressFunc) error {
	start := time.Now()
	b.progress = progress
	b.only = map[string]bool{}
	b.onlyStamps = changed
	ids := make([]any, 0, len(changed)+len(b.removed))
	for _, s := range changed {
		b.only[s.ID] = true
		ids = append(ids, s.ID)
	}
	for _, id := range b.removed {
		ids = append(ids, id)
	}
	if err := b.loadActiveSnapshot(); err != nil {
		return err
	}
	if err := b.buildHierarchy(); err != nil {
		return fmt.Errorf("failed to build hierarchy: %w", err)
	}
	if r, ok := b.reader.(interface {
		GetMicroflow(model.ID) (*microflows.Microflow, error)
		GetNanoflow(model.ID) (*microflows.Nanoflow, error)
		GetRule(model.ID) (*microflows.Rule, error)
	}); ok {
		var err error
		if b.microflowCache, err = changedFlows(b, "Microflows$Microflow", r.GetMicroflow); err != nil {
			return err
		}
		if b.nanoflowCache, err = changedFlows(b, "Microflows$Nanoflow", r.GetNanoflow); err != nil {
			return err
		}
		if b.ruleCache, err = changedFlows(b, "Microflows$Rule", r.GetRule); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("reader cannot read flows by ID")
	}
	if r, ok := b.reader.(interface {
		GetPage(model.ID) (*pages.Page, error)
		GetSnippet(model.ID) (*pages.Snippet, error)
	}); ok {
		var err error
		if b.pageCache, err = changedFlows(b, "Forms$Page", r.GetPage); err != nil {
			return err
		}
		if b.snippetCache, err = changedFlows(b, "Forms$Snippet", r.GetSnippet); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("reader cannot read pages by ID")
	}

	tx, err := b.catalog.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()
	b.tx = tx

	if len(ids) == 0 {
		return b.commitStamps(tx)
	}
	if err := createIncrementalTables(tx); err != nil {
		return err
	}
	for _, q := range []string{`DELETE FROM incremental_units`, `DELETE FROM incremental_modules`} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if _, err := tx.Exec(`INSERT INTO incremental_units (UnitId) VALUES (?)`, id); err != nil {
			return err
		}
	}
	dmSteps, err := b.prepareDomainModels(tx, changed)
	if err != nil {
		return err
	}
	for _, table := range UnitTables {
		del := `DELETE FROM ` + table + ` WHERE UnitId IN (SELECT UnitId FROM incremental_units)`
		if _, err := tx.Exec(del); err != nil {
			return fmt.Errorf("%s: %w", del, err)
		}
	}

	for _, step := range append(dmSteps, []struct {
		name string
		run  func() error
	}{
		{"microflows", b.buildMicroflows},
		{"pages", b.buildPages},
		{"snippets", b.buildSnippets},
		{"xpath expressions", b.buildXPathExpressions},
		{"permissions", b.buildPermissions},
		{"references", b.buildReferences},
		{"strings", b.buildStrings},
	}...) {
		if err := step.run(); err != nil {
			return fmt.Errorf("failed to build %s: %w", step.name, err)
		}
	}
	if err := b.commitStamps(tx); err != nil {
		return err
	}
	tx = nil
	b.report(fmt.Sprintf("Incremental refresh of %d unit(s) (%.1fs)", len(ids), time.Since(start).Seconds()), len(ids))
	return nil
}

// commitStamps writes the current stamps and commits.
func (b *Builder) commitStamps(tx CatalogTx) error {
	if _, err := tx.Exec(`DELETE FROM unit_stamps`); err != nil {
		return err
	}
	if err := b.writeStamps(b.stamps); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.tx = nil
	return b.catalog.SetMeta(MetaBuildTime, time.Now().Format(time.RFC3339))
}

type incrementalStep = struct {
	name string
	run  func() error
}

// prepareDomainModels records the modules of the changed domain models and
// returns the builder steps that rebuild their rows, which run before the flow
// steps (the member-ref index reads their entities).
func (b *Builder) prepareDomainModels(tx CatalogTx, changed []types.UnitStamp) ([]incrementalStep, error) {
	var modules [][2]string
	for _, s := range changed {
		if s.Type == domainModelType {
			id := b.hierarchy.findModuleID(model.ID(s.ContainerID))
			modules = append(modules, [2]string{b.hierarchy.getModuleName(id), string(id)})
		}
	}
	if len(modules) == 0 {
		return nil, nil
	}
	for _, m := range modules {
		if _, err := tx.Exec(`INSERT INTO incremental_modules (Name, Id) VALUES (?, ?)`, m[0], m[1]); err != nil {
			return nil, err
		}
	}
	const inMods = `(SELECT Name FROM incremental_modules)`
	external := `SELECT COUNT(*) FROM entities_data WHERE IsExternal = 1 AND ModuleName IN ` + inMods
	var n int
	if err := tx.QueryRow(external).Scan(&n); err != nil || n > 0 {
		return nil, fmt.Errorf("a changed domain model has external entities")
	}
	// The module's domain-model documentation row belongs to the module unit;
	// every other row of the domain model goes with its UnitId.
	del := `DELETE FROM strings WHERE StringContext = 'domain_model_documentation' AND UnitId IN (SELECT Id FROM incremental_modules)`
	if _, err := tx.Exec(del); err != nil {
		return nil, fmt.Errorf("%s: %w", del, err)
	}
	return []incrementalStep{
		{"entities", b.buildEntities},
		{"external entities check", func() error {
			if err := tx.QueryRow(external).Scan(&n); err != nil || n > 0 {
				return fmt.Errorf("a changed domain model has external entities")
			}
			return nil
		}},
		{"associations", b.buildAssociations},
		{"entity event handlers", b.buildEntityEventHandlers},
		{"regular expression rules", b.collectRegexRuleRefs},
		{"module documentation", b.updateModuleDocumentation},
	}, nil
}

// updateModuleDocumentation sets modules_data's domain-model documentation for
// the changed domain models' modules.
func (b *Builder) updateModuleDocumentation() error {
	dms, err := b.cachedDomainModels()
	if err != nil {
		return err
	}
	for _, dm := range dms {
		if !b.wants(string(dm.ID)) {
			continue
		}
		if _, err := b.tx.Exec(`UPDATE modules_data SET DomainModelDocumentation = ? WHERE Id = ?`,
			dm.Documentation, string(b.hierarchy.findModuleID(dm.ContainerID))); err != nil {
			return err
		}
	}
	return nil
}

// loadActiveSnapshot points the builder at the catalog's existing snapshot,
// whose ID every row carries.
func (b *Builder) loadActiveSnapshot() error {
	var id string
	if err := b.catalog.db.QueryRow(`SELECT SnapshotId FROM snapshots WHERE IsActive = 1 LIMIT 1`).Scan(&id); err != nil {
		return fmt.Errorf("catalog has no active snapshot: %w", err)
	}
	b.snapshot = &Snapshot{ID: id}
	return nil
}
