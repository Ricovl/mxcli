// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
)

// DefaultBatchSize is the number of rows inserted per transaction if not specified.
const DefaultBatchSize = 1000

// MaxPgParams is the PostgreSQL parameter limit per query.
const MaxPgParams = 65535

// ColumnMapping maps a source query column to a target table column.
type ColumnMapping struct {
	SourceName string
	TargetName string
}

// ImportConfig holds the configuration for an IMPORT operation.
type ImportConfig struct {
	SourceConn  *Connection
	TargetConn  *Connection
	SourceQuery string
	TargetTable string
	EntityName  string // qualified name for sequence lookup (e.g., "MyModule.Customer")
	ColumnMap   []ColumnMapping
	Assocs      []*AssocInfo // resolved association LINK mappings
	BatchSize   int
	Limit       int
}

// ImportResult holds the outcome of an IMPORT operation.
type ImportResult struct {
	TotalRows      int
	BatchesWritten int
	LinksCreated   map[string]int // association name → count of linked rows
	LinksMissed    map[string]int // association name → count of NULL lookups
}

// MendixIDInfo holds entity identifier information from the Mendix system tables.
type MendixIDInfo struct {
	ShortID        int64
	ObjectSequence int64
}

// LookupEntityID queries mendixsystem$entityidentifier joined with mendixsystem$entity
// to get the short_id and object_sequence for the given entity.
func LookupEntityID(ctx context.Context, conn *Connection, entityName string) (*MendixIDInfo, error) {
	query := `
		SELECT ei.short_id, ei.object_sequence
		FROM mendixsystem$entityidentifier ei
		JOIN mendixsystem$entity e ON ei.id = e.id
		WHERE e.entity_name = $1`

	var info MendixIDInfo
	err := conn.DB.QueryRowContext(ctx, query, entityName).Scan(&info.ShortID, &info.ObjectSequence)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("entity %q not found in mendixsystem$entityidentifier (has the app been started at least once?)", entityName)
		}
		return nil, fmt.Errorf("failed to look up entity ID for %q: %w", entityName, err)
	}
	return &info, nil
}

// HasMxObjectVersion checks if the target table has an mxobjectversion column.
func HasMxObjectVersion(ctx context.Context, conn *Connection, tableName string) (bool, error) {
	query := `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = $1 AND column_name = 'mxobjectversion'`

	var count int
	if err := conn.DB.QueryRowContext(ctx, query, tableName).Scan(&count); err != nil {
		return false, fmt.Errorf("failed to check mxobjectversion column: %w", err)
	}
	return count > 0, nil
}

// GenerateMendixID generates a Mendix object ID from a short_id and sequence number.
// Format: (short_id << 48) | (sequence << 7) | random_7bits
// The 7-bit random suffix prevents sequential ID enumeration (IDOR).
func GenerateMendixID(shortID, sequence int64) int64 {
	var b [1]byte
	_, _ = rand.Read(b[:])
	random7 := int64(b[0] & 0x7F) // 0–127
	return (shortID << 48) | (sequence << 7) | random7
}

// importRow holds a row's mapped attribute values and association source values.
type importRow struct {
	attrValues  []any // mapped attribute values
	assocValues []any // association source column values (parallel to cfg.Assocs)
}

// ExecuteImport runs the full import pipeline: read from source, write to target.
func ExecuteImport(ctx context.Context, cfg *ImportConfig, progressFn func(batch, rows int)) (*ImportResult, error) {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	// A LINK stored neither as a column nor in a junction table would be
	// skipped by both write paths in insertBatchWithAssocs: the rows would land
	// unlinked and the import would report success (ako/mxcli#1454). Refuse
	// before the first row is written.
	for _, a := range cfg.Assocs {
		if a.StorageFormat != StorageFormatColumn && a.StorageFormat != StorageFormatTable {
			return nil, fmt.Errorf("LINK %s: unrecognised association storage format %q (expected %q or %q); no rows were imported",
				a.AssociationName, a.StorageFormat, StorageFormatColumn, StorageFormatTable)
		}
	}

	// Count total columns per row for parameter limit check
	// +2 for id and mxobjectversion, plus column-storage assoc FK columns
	colStorageAssocs := 0
	for _, a := range cfg.Assocs {
		if a.StorageFormat == StorageFormatColumn {
			colStorageAssocs++
		}
	}
	colCount := len(cfg.ColumnMap) + 2 + colStorageAssocs
	if batchSize*colCount > MaxPgParams {
		batchSize = MaxPgParams / colCount
	}

	// Look up entity ID info
	idInfo, err := LookupEntityID(ctx, cfg.TargetConn, cfg.EntityName)
	if err != nil {
		return nil, err
	}

	// Check for mxobjectversion column
	hasMxObjVer, err := HasMxObjectVersion(ctx, cfg.TargetConn, cfg.TargetTable)
	if err != nil {
		return nil, err
	}

	// Execute source query
	rows, err := cfg.SourceConn.DB.QueryContext(ctx, cfg.SourceQuery)
	if err != nil {
		return nil, fmt.Errorf("source query failed: %w", err)
	}
	defer rows.Close()

	// Get source columns for mapping validation
	srcCols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get source columns: %w", err)
	}

	// Build source column index for efficient lookup
	srcColIdx := make(map[string]int, len(srcCols))
	for i, c := range srcCols {
		srcColIdx[strings.ToLower(c)] = i
	}

	// Validate all mapped source columns exist
	sourceIndices := make([]int, len(cfg.ColumnMap))
	for i, m := range cfg.ColumnMap {
		idx, ok := srcColIdx[strings.ToLower(m.SourceName)]
		if !ok {
			return nil, fmt.Errorf("source column %q not found in query result (available: %s)",
				m.SourceName, strings.Join(srcCols, ", "))
		}
		sourceIndices[i] = idx
	}

	// Validate LINK source columns exist
	assocSourceIndices := make([]int, len(cfg.Assocs))
	for i, a := range cfg.Assocs {
		idx, ok := srcColIdx[strings.ToLower(a.SourceColumn)]
		if !ok {
			return nil, fmt.Errorf("LINK source column %q not found in query result (available: %s)",
				a.SourceColumn, strings.Join(srcCols, ", "))
		}
		assocSourceIndices[i] = idx
	}

	// Stream rows and batch-insert
	result := &ImportResult{
		LinksCreated: make(map[string]int),
		LinksMissed:  make(map[string]int),
	}
	currentSeq := idInfo.ObjectSequence
	var batch []importRow
	totalRead := 0

	for rows.Next() {
		// Check limit
		if cfg.Limit > 0 && totalRead >= cfg.Limit {
			break
		}

		// Scan source row
		vals := make([]any, len(srcCols))
		ptrs := make([]any, len(srcCols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("failed to scan source row %d: %w", totalRead+1, err)
		}

		// Extract mapped attribute values
		mapped := make([]any, len(cfg.ColumnMap))
		for i, idx := range sourceIndices {
			v := vals[idx]
			if b, ok := v.([]byte); ok {
				mapped[i] = string(b)
			} else {
				mapped[i] = v
			}
		}

		// Extract association source values
		assocVals := make([]any, len(cfg.Assocs))
		for i, idx := range assocSourceIndices {
			v := vals[idx]
			if b, ok := v.([]byte); ok {
				assocVals[i] = string(b)
			} else {
				assocVals[i] = v
			}
		}

		batch = append(batch, importRow{attrValues: mapped, assocValues: assocVals})
		totalRead++

		// Flush batch
		if len(batch) >= batchSize {
			newSeq, err := insertBatchWithAssocs(ctx, cfg, batch, idInfo.ShortID, currentSeq, hasMxObjVer, result)
			if err != nil {
				return nil, fmt.Errorf("batch %d insert failed: %w", result.BatchesWritten+1, err)
			}
			currentSeq = newSeq
			result.BatchesWritten++
			result.TotalRows += len(batch)
			if progressFn != nil {
				progressFn(result.BatchesWritten, result.TotalRows)
			}
			batch = batch[:0]
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("source row iteration error: %w", err)
	}

	// Flush remaining rows
	if len(batch) > 0 {
		newSeq, err := insertBatchWithAssocs(ctx, cfg, batch, idInfo.ShortID, currentSeq, hasMxObjVer, result)
		if err != nil {
			return nil, fmt.Errorf("final batch insert failed: %w", err)
		}
		_ = newSeq
		result.BatchesWritten++
		result.TotalRows += len(batch)
		if progressFn != nil {
			progressFn(result.BatchesWritten, result.TotalRows)
		}
	}

	return result, nil
}

// insertBatchWithAssocs inserts a batch of rows with association support.
// For column-storage associations, FK columns are included in the entity INSERT.
// For table-storage associations, junction table rows are inserted after the entity INSERT.
// Returns the new sequence value after the batch.
func insertBatchWithAssocs(ctx context.Context, cfg *ImportConfig, batch []importRow,
	shortID, startSeq int64, hasMxObjVer bool, result *ImportResult) (int64, error) {

	tx, err := cfg.TargetConn.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Build column list: id, mapped columns, column-storage FK columns, [mxobjectversion]
	var cols []string
	cols = append(cols, "id")
	for _, m := range cfg.ColumnMap {
		cols = append(cols, fmt.Sprintf("%q", m.TargetName))
	}
	// Add column-storage association FK columns
	var colStorageAssocIndices []int // indices into cfg.Assocs for column-storage
	for i, a := range cfg.Assocs {
		if a.StorageFormat == StorageFormatColumn {
			cols = append(cols, fmt.Sprintf("%q", a.FKColumnName))
			colStorageAssocIndices = append(colStorageAssocIndices, i)
		}
	}
	if hasMxObjVer {
		cols = append(cols, "mxobjectversion")
	}

	// Build multi-row VALUES clause
	var valueClauses []string
	var args []any
	paramIdx := 1

	// Track generated IDs for table-storage junction inserts
	generatedRows := make([]junctionRow, len(batch))

	for rowIdx, row := range batch {
		seq := startSeq + int64(rowIdx) + 1
		id := GenerateMendixID(shortID, seq)
		generatedRows[rowIdx] = junctionRow{parentID: id, assocValues: row.assocValues}

		var placeholders []string
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
		args = append(args, id)
		paramIdx++

		// Attribute values
		for _, val := range row.attrValues {
			placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
			args = append(args, val)
			paramIdx++
		}

		// Column-storage FK values
		for _, assocIdx := range colStorageAssocIndices {
			placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
			assocVal := row.assocValues[assocIdx]
			childID, found := cfg.Assocs[assocIdx].Lookup(assocVal)
			if found {
				args = append(args, childID)
				result.LinksCreated[cfg.Assocs[assocIdx].AssociationName]++
			} else {
				args = append(args, nil) // NULL FK
				if assocVal != nil {
					result.LinksMissed[cfg.Assocs[assocIdx].AssociationName]++
				}
			}
			paramIdx++
		}

		if hasMxObjVer {
			placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
			args = append(args, 1)
			paramIdx++
		}

		valueClauses = append(valueClauses, "("+strings.Join(placeholders, ", ")+")")
	}

	// Auto-split if parameter limit exceeded
	if len(args) > MaxPgParams {
		mid := len(batch) / 2
		newSeq, err := insertBatchWithAssocs(ctx, cfg, batch[:mid], shortID, startSeq, hasMxObjVer, result)
		if err != nil {
			return 0, err
		}
		return insertBatchWithAssocs(ctx, cfg, batch[mid:], shortID, newSeq, hasMxObjVer, result)
	}

	query := fmt.Sprintf(`INSERT INTO %q (%s) VALUES %s`,
		cfg.TargetTable,
		strings.Join(cols, ", "),
		strings.Join(valueClauses, ", "),
	)

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return 0, fmt.Errorf("INSERT failed: %w", err)
	}

	// Insert junction table rows for table-storage associations
	for assocIdx, a := range cfg.Assocs {
		if a.StorageFormat != StorageFormatTable {
			continue
		}
		if err := insertJunctionRows(ctx, tx, a, generatedRows, assocIdx, result); err != nil {
			return 0, err
		}
	}

	// Update object sequence
	newSeq := startSeq + int64(len(batch))
	if err := updateObjectSequence(ctx, tx, cfg.EntityName, newSeq); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit batch: %w", err)
	}

	return newSeq, nil
}

// junctionRow tracks a generated parent ID and its association source values.
type junctionRow struct {
	parentID    int64
	assocValues []any // parallel to cfg.Assocs
}

// insertJunctionRows batch-inserts rows into a junction table for a table-storage association.
func insertJunctionRows(ctx context.Context, tx *sql.Tx, assoc *AssocInfo,
	rows []junctionRow, assocIdx int, result *ImportResult) error {

	var valueClauses []string
	var args []any
	paramIdx := 1

	for _, row := range rows {
		srcVal := row.assocValues[assocIdx]
		childID, found := assoc.Lookup(srcVal)
		if !found {
			if srcVal != nil {
				result.LinksMissed[assoc.AssociationName]++
			}
			continue
		}
		result.LinksCreated[assoc.AssociationName]++
		valueClauses = append(valueClauses, fmt.Sprintf("($%d, $%d)", paramIdx, paramIdx+1))
		args = append(args, row.parentID, childID)
		paramIdx += 2
	}

	if len(valueClauses) == 0 {
		return nil
	}

	query := fmt.Sprintf(`INSERT INTO %q (%q, %q) VALUES %s`,
		assoc.JunctionTable,
		assoc.ParentColName,
		assoc.ChildColName,
		strings.Join(valueClauses, ", "),
	)

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("junction table INSERT for %s failed: %w", assoc.AssociationName, err)
	}

	return nil
}

// updateObjectSequence updates the object_sequence counter for the given entity.
func updateObjectSequence(ctx context.Context, tx *sql.Tx, entityName string, newSeq int64) error {
	// The aliases are spelled with AS: PostgreSQL treats it as optional, SQLite
	// requires it, and the import tests run this pipeline against SQLite.
	query := `
		UPDATE mendixsystem$entityidentifier AS ei
		SET object_sequence = $1
		FROM mendixsystem$entity AS e
		WHERE ei.id = e.id AND e.entity_name = $2`

	result, err := tx.ExecContext(ctx, query, newSeq, entityName)
	if err != nil {
		return fmt.Errorf("failed to update object_sequence: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("no rows updated for entity %q sequence", entityName)
	}

	return nil
}
