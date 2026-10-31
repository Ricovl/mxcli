// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
)

// deserializeConn replaces conn's main database with data.
func deserializeConn(conn *sql.Conn, data []byte) error {
	return conn.Raw(func(dc any) error {
		d, ok := dc.(interface{ Deserialize([]byte) error })
		if !ok {
			return fmt.Errorf("the SQLite driver cannot deserialize")
		}
		return d.Deserialize(data)
	})
}

// Incremental catalog builds (ako/mxcli#1804).
//
// A refresh rebuilt every table from every unit after any save, which on a
// large project is seconds of decoding and inserting rows that are exactly
// the rows already in the cache. A build now records which unit each row came
// from (builderTx), and the next build rebuilds only:
//
//   - the units whose contents or container changed, were added or removed;
//   - the units holding a document whose refs name one of them, before or
//     after the change: a pass resolves names against other documents (an
//     association's ends, a called flow, an attribute's type), so a document
//     that names a changed one may write different rows. A specialization
//     counts as naming whatever its generalization holds, transitively,
//     because member names resolve through the inheritance chain.
//
// The build starts from the cached catalog itself, loaded into memory, and
// drops the rows of those units and the rows written by no unit (project-wide
// passes, projections over other tables), which it produces again. Every
// other unit's rows stay where they are: nothing is copied, decoded or
// re-indexed for them.
//
// Anything the plan cannot vouch for makes it a full build, as before: a
// change to a unit no per-unit pass reads (project settings, security, a
// module or folder), another build mode or mxcli build, a schema change, a
// row written for a document the pass was skipping. Reuse is only ever an
// optimisation of a full build, never a different answer — the tests compare
// every table of an incremental build with a forced full one.

// MetaCatalogFingerprint records which mxcli build wrote the catalog's rows.
const MetaCatalogFingerprint = "catalog_fingerprint"

// SetPreviousCatalog gives the build the cached catalog it may carry rows
// over from.
func (b *Builder) SetPreviousCatalog(path string) {
	b.prevCatalogPath = path
}

// SetScopedReader supplies the view an incremental build lists documents
// from: one holding only the units keep accepts, which reads and decodes no
// other. Without it the build lists every document and skips the ones whose
// rows it carries.
func (b *Builder) SetScopedReader(f func(keep func(unitID string) bool) CatalogReader) {
	b.scopedReader = f
}

// SetFingerprint identifies the mxcli build writing the catalog: rows written
// by another build are never carried over, since it may write them
// differently.
func (b *Builder) SetFingerprint(fp string) {
	b.fingerprint = fp
}

// BuildStats says how the last Build went.
type BuildStats struct {
	Incremental bool   // rows of unchanged units were carried over
	Reason      string // why the build was full, when it was
	Rebuilt     int    // units whose rows were produced again
	Carried     int    // units whose rows were copied
	Rounds      int    // incremental passes run (a second one widens the scope)
}

// Stats reports how the last Build went.
func (b *Builder) Stats() BuildStats {
	return b.stats
}

// own marks the rows written from here on as coming from unit, and reports
// whether this build writes them. It is false for a unit whose rows were
// carried over: the pass skips the document.
func (b *Builder) own(unit string) bool {
	if b.rows == nil {
		b.rows = newRowOwners()
	}
	b.rows.owned[unit] = true
	if b.scope != nil && !b.scope[unit] {
		b.rows.owner = skippedOwner
		return false
	}
	b.rows.owner = unit
	return true
}

// currentOwner is the unit the rows being written now come from, for a pass
// that collects rows a later one writes.
func (b *Builder) currentOwner() string {
	if b.rows == nil {
		return ""
	}
	return b.rows.owner
}

// as marks the rows written from here on as coming from owner, which a pass
// recorded when it collected them: a unit in scope, or "" for project-wide.
func (b *Builder) as(owner string) {
	if b.rows != nil {
		b.rows.owner = owner
	}
}

// ownRows is own for a pass that walks units of every type (the strings
// index): it attributes the unit's rows to it without counting the unit as
// one a per-unit pass reads. A unit only such a walk visits — the project
// settings, a module, a folder — is read by project-wide passes too, and a
// change to it still makes the build a full one.
func (b *Builder) ownRows(unit string) bool {
	if b.rows == nil {
		b.rows = newRowOwners()
	}
	if b.scope != nil && !b.scope[unit] {
		b.rows.owner = skippedOwner
		return false
	}
	b.rows.owner = unit
	return true
}

// disown marks the rows written from here on as project-wide.
func (b *Builder) disown() {
	if b.rows != nil {
		b.rows.owner = ""
	}
}

// buildMode is the mode name the cache records for this build.
func (b *Builder) buildMode() string {
	switch {
	case b.sourceMode:
		return "source"
	case b.fullMode:
		return "full"
	}
	return "fast"
}

// unitBasis is what a unit's rows depend on of the unit itself: its content
// hash and its container. The container is a column of the Unit table, not
// part of the bytes, and rows print the folder and module. Nil when the
// reader cannot hash units, which carries and records nothing. It also
// returns each unit's type and container.
func (b *Builder) unitBasis() (basis, types, parents map[string]string) {
	h, ok := b.reader.(unitHasher)
	if !ok {
		return nil, nil, nil
	}
	hashes, err := h.UnitContentHashes()
	if err != nil {
		return nil, nil, nil
	}
	units, err := b.reader.ListUnits()
	if err != nil {
		return nil, nil, nil
	}
	types, parents = map[string]string{}, map[string]string{}
	for _, u := range units {
		id := string(u.ID)
		types[id] = u.Type
		parents[id] = string(u.ContainerID)
		if hash, ok := hashes[id]; ok {
			hashes[id] = hash + "|" + string(u.ContainerID)
		}
	}
	return hashes, types, parents
}

const folderType = "Projects$Folder"

// folderDescendants is every unit below the given folders, nested folders
// included: a folder's name and place are in the folder path, module and
// qualified name of every document under it.
func (b *Builder) folderDescendants(folders map[string]bool) map[string]bool {
	out := map[string]bool{}
	if len(folders) == 0 {
		return out
	}
	for id := range b.unitParents {
		seen := 0
		for p := b.unitParents[id]; p != "" && seen < 64; p = b.unitParents[p] {
			if folders[p] {
				out[id] = true
				break
			}
			seen++
		}
	}
	return out
}

// incrementalPlan is what an incremental build rebuilds.
type incrementalPlan struct {
	scope   map[string]bool // units whose rows are produced again
	changed map[string]bool // units changed, added or removed
	added   map[string]bool // units the previous build did not know
}

// prevCatalog is the previous catalog's basis.
type prevCatalog struct {
	basis map[string]string
	owned map[string]bool
	types map[string]string
}

// loadPrevious reads the cached catalog and plans the build from it, on a
// scratch in-memory copy; it returns the bytes the build loads to build on,
// or says why the build has to be a full one. Planning from the bytes the
// build then keeps means a cache another process replaces meanwhile cannot
// be planned from one version and built on another.
func (b *Builder) loadPrevious(ctx context.Context) ([]byte, *incrementalPlan, string) {
	data, err := os.ReadFile(b.prevCatalogPath)
	if err != nil {
		return nil, nil, "no cache to carry from"
	}
	scratch, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, nil, err.Error()
	}
	defer scratch.Close()
	conn, err := scratch.Conn(ctx)
	if err != nil {
		return nil, nil, err.Error()
	}
	defer conn.Close()
	if err := deserializeConn(conn, data); err != nil {
		return nil, nil, "cannot load the cache: " + err.Error()
	}
	plan, reason := b.planFrom(ctx, conn)
	return data, plan, reason
}

// planFrom plans the build from the previous catalog on conn.
func (b *Builder) planFrom(ctx context.Context, conn *sql.Conn) (*incrementalPlan, string) {
	meta := func(key string) string {
		var v string
		_ = conn.QueryRowContext(ctx, `SELECT Value FROM catalog_meta WHERE Key = ?`, key).Scan(&v)
		return v
	}
	switch {
	case meta(MetaSchemaVersion) != CatalogSchemaVersion:
		return nil, "the cache has another schema version"
	case meta(MetaCatalogFingerprint) != b.fingerprint:
		return nil, "the cache was written by another mxcli build"
	case meta(MetaBuildMode) != b.buildMode():
		return nil, "the cache was built in another mode"
	}
	var snap string
	_ = conn.QueryRowContext(ctx, `SELECT SnapshotId FROM snapshots`).Scan(&snap)
	if b.snapshot == nil || snap != b.snapshot.ID {
		return nil, "the cache records another snapshot"
	}

	p := &prevCatalog{basis: map[string]string{}, owned: map[string]bool{}, types: map[string]string{}}
	rows, err := conn.QueryContext(ctx, `SELECT UnitId, Basis, Owned, Type FROM catalog_units`)
	if err != nil {
		return nil, "the cache records no unit basis"
	}
	for rows.Next() {
		var id, basis, typ string
		var owned int
		if err := rows.Scan(&id, &basis, &owned, &typ); err != nil {
			rows.Close()
			return nil, err.Error()
		}
		p.basis[id] = basis
		p.owned[id] = owned != 0
		p.types[id] = typ
	}
	rows.Close()
	if len(p.basis) == 0 {
		return nil, "the cache records no unit basis"
	}
	b.prevOwned = p.owned
	return b.planIncremental(querier(ctx, conn.QueryContext), p)
}

// planIncremental decides which units to rebuild, or says why the build has
// to be full. q reads the previous catalog.
func (b *Builder) planIncremental(q queryFunc, prev *prevCatalog) (*incrementalPlan, string) {
	plan := &incrementalPlan{scope: map[string]bool{}, changed: map[string]bool{}, added: map[string]bool{}}
	for id, basis := range b.basis {
		old, ok := prev.basis[id]
		if !ok {
			plan.changed[id] = true
			plan.added[id] = true
		} else if old != basis {
			plan.changed[id] = true
		}
	}
	for id := range prev.basis {
		if _, ok := b.basis[id]; !ok {
			plan.changed[id] = true
		}
	}
	// A folder has no rows of its own; a renamed or moved one changes what
	// every unit below it prints, so those are rebuilt as changed.
	folders := map[string]bool{}
	for id := range plan.changed {
		if b.unitTypes[id] == folderType || prev.types[id] == folderType {
			folders[id] = true
		}
	}
	for id := range b.folderDescendants(folders) {
		plan.changed[id] = true
	}
	for id := range plan.changed {
		if !plan.added[id] && !prev.owned[id] && !folders[id] && b.unitTypes[id] != folderType {
			return nil, fmt.Sprintf("a %s changed", b.unitKind(id))
		}
		plan.scope[id] = true
	}
	if len(plan.changed) == 0 {
		return plan, ""
	}
	names, err := unitNames(q, plan.changed)
	if err != nil {
		return nil, "the cache records no unit names: " + err.Error()
	}
	deps, err := dependentUnits(q, names)
	if err != nil {
		return nil, "the cache's references are unreadable: " + err.Error()
	}
	for id := range deps {
		plan.scope[id] = true
	}
	return plan, ""
}

// queryFunc runs a query on the connection or transaction a step has.
type queryFunc func(query string, args ...any) (*sql.Rows, error)

func querier(ctx context.Context, f func(context.Context, string, ...any) (*sql.Rows, error)) queryFunc {
	return func(query string, args ...any) (*sql.Rows, error) { return f(ctx, query, args...) }
}

// verifyIncremental checks an incremental attempt before it commits. It
// returns why the build has to be a full one, or the units the plan missed:
// those holding a document that names what a changed unit is called now (a
// rename, or a document added under a name others already used).
func (b *Builder) verifyIncremental(plan *incrementalPlan) (map[string]bool, string) {
	for id := range plan.changed {
		if _, exists := b.basis[id]; exists && !b.rows.owned[id] && b.unitTypes[id] != folderType {
			return nil, fmt.Sprintf("a %s changed", b.unitKind(id))
		}
	}
	q := queryFunc(b.tx.CatalogTx.Query)
	names, err := unitNames(q, plan.changed)
	if err != nil {
		return nil, err.Error()
	}
	deps, err := dependentUnits(q, names)
	if err != nil {
		return nil, err.Error()
	}
	more := map[string]bool{}
	for id := range deps {
		if !plan.scope[id] {
			more[id] = true
		}
	}
	return more, ""
}

// incrementalSummary is the progress line for a build that carried rows.
func incrementalSummary(s BuildStats) string {
	return fmt.Sprintf("incremental: %d units rebuilt, %d carried over", s.Rebuilt, s.Carried)
}

// unitKind names a unit's type for a progress line.
func (b *Builder) unitKind(id string) string {
	if t := b.unitTypes[id]; t != "" {
		return t
	}
	return "unit"
}

// unitNames is the qualified names the given units' rows carry (an
// entity's, an association's, a microflow's).
func unitNames(q queryFunc, units map[string]bool) ([]string, error) {
	rows, err := q(`SELECT UnitId, QualifiedName FROM catalog_unit_names`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, qn string
		if err := rows.Scan(&id, &qn); err != nil {
			return nil, err
		}
		if units[id] {
			out = append(out, qn)
		}
	}
	return out, rows.Err()
}

// dependentUnits is the units holding a document whose refs name one of
// names or a member of one (`M.E.Attr` under `M.E`). Specializations are
// followed first: a document naming `M.Sub.Attr` may resolve it on Sub's
// generalization.
func dependentUnits(q queryFunc, names []string) (map[string]bool, error) {
	g, err := loadRefGraph(q)
	if err != nil {
		return nil, err
	}
	return g.dependents(names), nil
}

// refGraph is the refs and unit names the plan follows, read once.
type refGraph struct {
	refs    []graphRef
	unitOf  map[string][]string // qualified name -> units whose rows carry it
	namesOf map[string][]string // unit -> qualified names its rows carry
}

type graphRef struct{ source, target, kind string }

func loadRefGraph(q queryFunc) (*refGraph, error) {
	g := &refGraph{unitOf: map[string][]string{}, namesOf: map[string][]string{}}
	rows, err := q(`SELECT SourceName, TargetName, RefKind FROM refs`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r graphRef
		if err := rows.Scan(&r.source, &r.target, &r.kind); err != nil {
			rows.Close()
			return nil, err
		}
		g.refs = append(g.refs, r)
	}
	rows.Close()

	owners, err := q(`SELECT UnitId, QualifiedName FROM catalog_unit_names`)
	if err != nil {
		return nil, err
	}
	defer owners.Close()
	for owners.Next() {
		var id, qn string
		if err := owners.Scan(&id, &qn); err != nil {
			return nil, err
		}
		g.unitOf[qn] = append(g.unitOf[qn], id)
		g.namesOf[id] = append(g.namesOf[id], qn)
	}
	return g, owners.Err()
}

// dependents is dependentUnits on a loaded graph.
func (g *refGraph) dependents(names []string) map[string]bool {
	named := map[string]bool{}
	for _, n := range names {
		if n != "" {
			named[n] = true
		}
	}
	for grew := true; grew; {
		grew = false
		for _, r := range g.refs {
			if r.kind == RefKindGeneralize && named[r.target] && !named[r.source] {
				named[r.source] = true
				grew = true
			}
		}
	}
	isNamed := func(target string) bool {
		for {
			if named[target] {
				return true
			}
			i := strings.LastIndexByte(target, '.')
			if i < 0 {
				return false
			}
			target = target[:i]
		}
	}
	sources := map[string]bool{}
	for _, r := range g.refs {
		if isNamed(r.target) {
			sources[r.source] = true
		}
	}
	for n := range named {
		sources[n] = true // a specialization is rebuilt itself
	}
	out := map[string]bool{}
	for qn := range sources {
		for _, id := range g.unitOf[qn] {
			out[id] = true
		}
	}
	return out
}

// clearCatalog empties every table of a catalog loaded from the cache, so a
// full build starts from what a fresh one would: no rows, no AUTOINCREMENT
// counters, and only the schema version in catalog_meta.
func clearCatalog(tx CatalogTx) error {
	rows, err := tx.Query(`SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return err
	}
	type table struct {
		name    string
		virtual bool
	}
	var tables []table
	for rows.Next() {
		var name string
		var ddl sql.NullString
		if err := rows.Scan(&name, &ddl); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, table{name, strings.HasPrefix(strings.ToUpper(ddl.String), "CREATE VIRTUAL TABLE")})
	}
	rows.Close()
	// A full-text table's shadow tables are emptied through the table.
	shadow := map[string]bool{}
	for _, t := range tables {
		if t.virtual {
			for _, suffix := range []string{"_data", "_idx", "_content", "_docsize", "_config"} {
				shadow[t.name+suffix] = true
			}
		}
	}
	for _, t := range tables {
		if shadow[t.name] {
			continue
		}
		stmt := `DELETE FROM "` + t.name + `"`
		if t.name == "catalog_meta" {
			stmt += ` WHERE Key != '` + MetaSchemaVersion + `'`
		}
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("empty %s: %w", t.name, err)
		}
	}
	var seq int
	if tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'sqlite_sequence'`).Scan(&seq) == nil && seq > 0 {
		if _, err := tx.Exec(`DELETE FROM sqlite_sequence`); err != nil {
			return err
		}
	}
	return nil
}

// dropRebuiltRows deletes, from the previous catalog the build starts from,
// the rows of the units in scope and the rows no unit wrote, and records the
// ranges of every row it keeps under its unit. It returns how many units'
// rows were kept.
func (b *Builder) dropRebuiltRows() (int, error) {
	type rng struct {
		table, owner string
		first, last  int64
	}
	rows, err := b.txQuery(`SELECT TableName, UnitId, FirstRow, LastRow FROM catalog_row_owners ORDER BY TableName, FirstRow`)
	if err != nil {
		return 0, err
	}
	var all []rng
	for rows.Next() {
		var r rng
		if err := rows.Scan(&r.table, &r.owner, &r.first, &r.last); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, r)
	}
	rows.Close()

	kept := map[string]bool{}
	for _, r := range all {
		if r.owner != "" && !b.scope[r.owner] {
			b.rows.add(r.table, r.owner, r.first, r.last)
			kept[r.owner] = true
			continue
		}
		if _, err := b.tx.CatalogTx.Exec(`DELETE FROM "`+r.table+`" WHERE rowid BETWEEN ? AND ?`, r.first, r.last); err != nil {
			return 0, fmt.Errorf("drop rows of %s: %w", r.table, err)
		}
	}
	// What records the build rather than the project is written again, and
	// the graph analysis, which AddGraphAnalysis writes outside a build.
	for _, stmt := range []string{
		`DELETE FROM communities_data`,
		`DELETE FROM graph_cycles_data`,
		`DELETE FROM graph_module_cycles_data`,
		`DELETE FROM graph_layers_data`,
		`DELETE FROM graph_centrality_data`,
		`DELETE FROM projects`,
		`DELETE FROM snapshots`,
		`DELETE FROM catalog_row_owners`,
		`DELETE FROM catalog_units`,
		`DELETE FROM catalog_unit_names`,
		`DELETE FROM catalog_meta WHERE Key IN ('` + MetaGraphAnalysis + `', '` + MetaGraphResolution + `', '` + MetaSourceFingerprint + `', '` + MetaCatalogFingerprint + `')`,
	} {
		if _, err := b.tx.CatalogTx.Exec(stmt); err != nil {
			return 0, err
		}
	}
	return len(kept), nil
}

func (b *Builder) txQuery(query string, args ...any) (*sql.Rows, error) {
	return b.tx.CatalogTx.Query(query, args...)
}

// persistOwners records what the next build needs to carry from this one:
// the unit each row range came from, every unit's basis and whether a
// per-unit pass reads it, the names each unit's rows carry, and which mxcli
// build wrote them.
func (b *Builder) persistOwners() error {
	if b.basis == nil || b.fingerprint == "" {
		return nil
	}
	exec := b.tx.CatalogTx.Exec
	ins, err := b.tx.CatalogTx.Prepare(`INSERT INTO catalog_row_owners (TableName, UnitId, FirstRow, LastRow) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	tables := make([]string, 0, len(b.rows.ranges))
	for t := range b.rows.ranges {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, t := range tables {
		for _, r := range b.rows.ranges[t] {
			// A project-wide range ("") is recorded too: the next build
			// drops those rows and writes them again.
			if _, err := ins.Exec(t, r.owner, r.first, r.last); err != nil {
				return err
			}
		}
	}

	units, err := b.tx.CatalogTx.Prepare(`INSERT INTO catalog_units (UnitId, Basis, Owned, Type) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer units.Close()
	for id, basis := range b.basis {
		owned := 0
		// A unit outside the scope was not listed, so this build did not
		// visit it; it is unchanged, and so is whether a pass reads it.
		if b.rows.owned[id] || (b.scope != nil && !b.scope[id] && b.prevOwned[id]) {
			owned = 1
		}
		if _, err := units.Exec(id, basis, owned, b.unitTypes[id]); err != nil {
			return err
		}
	}

	// The names each unit's rows carry, from every table with a
	// QualifiedName column.
	for _, t := range tables {
		var has int
		if err := b.tx.CatalogTx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?, 'main') WHERE name = 'QualifiedName'`, t).Scan(&has); err != nil {
			return err
		}
		if has == 0 {
			continue
		}
		if _, err := exec(`INSERT INTO catalog_unit_names (UnitId, QualifiedName)
			SELECT DISTINCT o.UnitId, t.QualifiedName FROM catalog_row_owners o CROSS JOIN main.` + t + ` t
			WHERE o.TableName = '` + t + `' AND o.UnitId != '' AND t.rowid BETWEEN o.FirstRow AND o.LastRow AND t.QualifiedName IS NOT NULL`); err != nil {
			return err
		}
	}
	_, err = exec(`INSERT OR REPLACE INTO catalog_meta (Key, Value) VALUES (?, ?)`, MetaCatalogFingerprint, b.fingerprint)
	return err
}
