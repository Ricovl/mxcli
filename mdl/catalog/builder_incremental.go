// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Incremental refresh: when only existing flows changed since the catalog was
// built, their rows are deleted and rebuilt instead of the whole catalog.
// See docs/11-proposals/PROPOSAL_incremental_project_index.md, step 5.

// unitStamper is a reader that can stamp MPR v2 unit files.
type unitStamper interface {
	UnitStamps() ([]types.UnitStamp, error)
}

// incrementalTypes are the unit types an incremental refresh rebuilds; any
// other change needs a full build.
var incrementalTypes = map[string]bool{
	"Microflows$Microflow": true,
	"Microflows$Nanoflow":  true,
	"Microflows$Rule":      true,
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

// changedFlows decodes only the changed flows of one kind. Never nil: a nil
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

// captureStamps records the unit stamps a full build is made from; taken before
// any unit is read, so a change during the build shows up next time.
func (b *Builder) captureStamps() {
	if s, ok := b.reader.(unitStamper); ok {
		b.stamps, _ = s.UnitStamps()
	}
}

func (b *Builder) writeStamps(stamps []types.UnitStamp) error {
	stmt, err := b.tx.Prepare(`INSERT OR REPLACE INTO unit_stamps (UnitId, Type, ContainerId, Size, MtimeNs) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range stamps {
		if _, err := stmt.Exec(s.ID, s.Type, s.ContainerID, s.Size, s.MtimeNs); err != nil {
			return err
		}
	}
	return nil
}

// PlanIncremental compares the reader's unit stamps with the catalog's and
// returns the changed units when an incremental refresh can rebuild them, or
// the reason it cannot.
func (b *Builder) PlanIncremental() (changed []types.UnitStamp, reason string) {
	s, ok := b.reader.(unitStamper)
	if !ok {
		return nil, "reader has no unit stamps"
	}
	now, err := s.UnitStamps()
	if err != nil || len(now) == 0 {
		return nil, "no unit stamps"
	}
	stored := map[string]types.UnitStamp{}
	rows, err := b.catalog.db.Query(`SELECT UnitId, Type, ContainerId, Size, MtimeNs FROM unit_stamps`)
	if err != nil {
		return nil, "catalog has no unit stamps"
	}
	for rows.Next() {
		var st types.UnitStamp
		if rows.Scan(&st.ID, &st.Type, &st.ContainerID, &st.Size, &st.MtimeNs) == nil {
			stored[st.ID] = st
		}
	}
	rows.Close()
	if len(stored) != len(now) {
		return nil, "units added or removed"
	}
	for _, n := range now {
		o, ok := stored[n.ID]
		if !ok {
			return nil, "units added or removed"
		}
		if o.Size == n.Size && o.MtimeNs == n.MtimeNs && o.MtimeNs != -1 && o.ContainerID == n.ContainerID && o.Type == n.Type {
			continue
		}
		if !incrementalTypes[n.Type] || o.Type != n.Type || o.ContainerID != n.ContainerID {
			return nil, "a non-flow unit changed or moved"
		}
		changed = append(changed, n)
		if len(changed) > maxIncrementalUnits {
			return nil, "too many units changed"
		}
	}
	var n int
	if b.catalog.db.QueryRow(`SELECT COUNT(*) FROM external_actions_data`).Scan(&n) == nil && n > 0 {
		return nil, "project calls external actions (aggregated across flows)"
	}
	b.stamps = now
	return changed, ""
}

// BuildIncremental rebuilds the rows of the changed flows in the catalog's own
// database, in one transaction.
func (b *Builder) BuildIncremental(changed []types.UnitStamp, progress ProgressFunc) error {
	start := time.Now()
	b.progress = progress
	b.only = map[string]bool{}
	b.onlyStamps = changed
	ids := make([]any, len(changed))
	for i, s := range changed {
		b.only[s.ID] = true
		ids[i] = s.ID
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
	in := "(" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS incremental_units (UnitId TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM incremental_units`); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`INSERT INTO incremental_units (UnitId) VALUES (?)`, id); err != nil {
			return err
		}
	}
	// Permissions are keyed by name, so by the stored (pre-rename) name.
	if _, err := tx.Exec(`DELETE FROM permissions WHERE ElementType = ? AND ElementName IN
		(SELECT QualifiedName FROM microflows_data WHERE Id IN `+in+`)`,
		append([]any{PermissionElementMicroflow}, ids...)...); err != nil {
		return err
	}
	for _, del := range []string{
		`DELETE FROM microflows_data WHERE Id IN ` + in,
		`DELETE FROM microflow_parameters_data WHERE MicroflowId IN ` + in,
		`DELETE FROM activities_data WHERE MicroflowId IN ` + in,
		`DELETE FROM refs WHERE SourceId IN ` + in,
		`DELETE FROM xpath_expressions_data WHERE DocumentId IN ` + in,
		`DELETE FROM strings WHERE UnitId IN ` + in,
	} {
		if _, err := tx.Exec(del, ids...); err != nil {
			return fmt.Errorf("%s: %w", del, err)
		}
	}

	for _, step := range []struct {
		name string
		run  func() error
	}{
		{"microflows", b.buildMicroflows},
		{"xpath expressions", b.buildXPathExpressions},
		{"permissions", b.buildPermissions},
		{"references", b.buildReferences},
		{"strings", b.buildStrings},
	} {
		if err := step.run(); err != nil {
			return fmt.Errorf("failed to build %s: %w", step.name, err)
		}
	}
	if err := b.commitStamps(tx); err != nil {
		return err
	}
	tx = nil
	b.report(fmt.Sprintf("Incremental refresh of %d flow(s) (%.1fs)", len(changed), time.Since(start).Seconds()), len(changed))
	return nil
}

// commitStamps writes the current stamps and commits.
func (b *Builder) commitStamps(tx CatalogTx) error {
	if err := b.writeStamps(b.stamps); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.tx = nil
	return b.catalog.SetMeta(MetaBuildTime, time.Now().Format(time.RFC3339))
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
