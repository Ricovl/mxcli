// SPDX-License-Identifier: Apache-2.0

// Package mpr - Unit listing infrastructure for Reader.
package mpr

import (
	"fmt"
	"os"
	"path/filepath"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ResolveModuleName walks the container hierarchy upward until it finds a module.
// This is necessary because in MPR v2 projects, documents live inside folders,
// so a document's direct ContainerID is a folder, not the module.
func ResolveModuleName(containerID string, moduleMap map[string]string, containerParent map[string]string) string {
	current := containerID
	for range 20 {
		if name, ok := moduleMap[current]; ok {
			return name
		}
		parent, ok := containerParent[current]
		if !ok || parent == current {
			break
		}
		current = parent
	}
	return ""
}

// BuildContainerParent builds a map of unit ID → parent container ID for hierarchy walking.
func (r *Reader) BuildContainerParent() (map[string]string, error) {
	units, err := r.ListUnits()
	if err != nil {
		return nil, err
	}
	containerParent := make(map[string]string, len(units))
	for _, u := range units {
		containerParent[string(u.ID)] = string(u.ContainerID)
	}
	return containerParent, nil
}

// rawUnit holds raw unit data from the database.
type rawUnit struct {
	ID              string
	ContainerID     string
	ContainmentName string
	Type            string
	Contents        []byte
}

// UnitRef holds unit metadata returned by ListUnitsByType.
type UnitRef struct {
	ID          string
	ContainerID string
	Type        string // BSON $Type (e.g. "Microflows$Microflow")
	Contents    []byte
}

// ListUnitsByType returns all units matching the given BSON $Type prefix.
// This is the exported version for use by TreeWriter and other packages.
func (r *Reader) ListUnitsByType(typePrefix string) ([]UnitRef, error) {
	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// listUnitsByType returns all units of exactly the given storage type. An empty
// typeName returns every unit.
//
// Exact, not prefix: Mendix storage names nest (`Forms$Page` is a prefix of
// `Forms$PageTemplate`), so a prefix match silently folds one document type into
// another. See the note on the same function in sdk/mpr.
func (r *Reader) listUnitsByType(typeName string) ([]rawUnit, error) {
	if r.version == MPRVersionV2 {
		return r.listUnitsByTypeV2(typeName)
	}
	return r.listUnitsByTypeV1(typeName)
}

// listUnitsByTypeV1 handles MPR v1 format (contents in database).
func (r *Reader) listUnitsByTypeV1(typeName string) ([]rawUnit, error) {
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName, Contents
		FROM Unit
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	var units []rawUnit
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName string
		var contents []byte

		if err := rows.Scan(&unitID, &containerID, &containmentName, &contents); err != nil {
			return nil, fmt.Errorf("failed to scan unit row: %w", err)
		}

		if held, ok := r.overlaid(blobToUUID(unitID)); ok {
			contents = held
		}
		unitType := getTypeFromContents(contents)
		if typeName == "" || unitType == typeName {
			units = append(units, rawUnit{
				ID:              blobToUUID(unitID),
				ContainerID:     blobToUUID(containerID),
				ContainmentName: containmentName,
				Type:            unitType,
				Contents:        contents,
			})
		}
	}

	return units, nil
}

// listUnitsByTypeV2 handles MPR v2 format (contents in mprcontents folder).
// Uses caching to avoid reading every file for each query.
func (r *Reader) listUnitsByTypeV2(typeName string) ([]rawUnit, error) {
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}

	// Filter by type using cache, only read contents for matching units.
	var units []rawUnit
	for _, cu := range r.unitCache {
		if typeName == "" || cu.Type == typeName {
			contents, err := r.readMprContents(cu.ID)
			if err != nil {
				continue
			}
			units = append(units, rawUnit{
				ID:              cu.ID,
				ContainerID:     cu.ContainerID,
				ContainmentName: cu.ContainmentName,
				Type:            cu.Type,
				Contents:        contents,
			})
		}
	}
	return units, nil
}

// buildUnitCache reads all unit metadata once and caches it.
func (r *Reader) buildUnitCache() error {
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName
		FROM Unit
	`)
	if err != nil {
		return fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	r.unitCache = nil
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName string

		if err := rows.Scan(&unitID, &containerID, &containmentName); err != nil {
			return fmt.Errorf("failed to scan unit row: %w", err)
		}

		unitUUID := blobToUUID(unitID)
		// Index construction must not populate the optional raw-content cache:
		// only the unit selected after lookup should be retained there.
		contents, err := r.readMprContentsUncached(unitUUID)
		if err != nil {
			continue
		}

		typeName := getTypeFromContents(contents)
		r.unitCache = append(r.unitCache, cachedUnit{
			ID:              blobToUUID(unitID),
			ContainerID:     blobToUUID(containerID),
			ContainmentName: containmentName,
			Type:            typeName,
			Name:            getNameFromContents(contents),
		})
	}

	r.unitCacheValid = true
	return nil
}

// InvalidateCache marks the unit cache as invalid and clears content cache entries.
// Should be called after any write operation.
func (r *Reader) InvalidateCache() {
	r.unitCacheValid = false
	r.nameIndexMu.Lock()
	r.nameIndex = nil
	r.nameIndexBuilt = false
	r.nameIndexMu.Unlock()
	r.decodedMu.Lock()
	r.decoded = nil
	r.decodedMu.Unlock()
	// Clear content cache entries but keep the map non-nil so caching stays active.
	// If contentCache is nil (per-request mode), remain disabled.
	if r.contentCache != nil {
		clear(r.contentCache)
	}
}

// EnableContentCache activates the in-memory content cache for this reader.
// Call once after Connect in persistent daemon mode. The cache survives across
// requests; InvalidateCache empties it (but keeps caching active) on writes.
func (r *Reader) EnableContentCache() {
	if r.contentCache == nil {
		r.contentCache = make(map[string][]byte)
	}
}

// readMprContents reads content from the mprcontents folder for v2 format.
// The path is: mprcontents/XX/YY/UUID.mxunit where XX and YY are first two chars of UUID.
//
// When r.contentCache is non-nil (persistent daemon mode), the result is cached
// in memory so subsequent reads of the same unit skip the file I/O entirely.
// The cache is invalidated by InvalidateCache (called after every write).
func (r *Reader) readMprContents(unitUUID string) ([]byte, error) {
	if len(unitUUID) < 4 {
		return nil, fmt.Errorf("invalid unit UUID: %s", unitUUID)
	}

	// Bytes held in memory (an import buffer, or a deferred run of writes) are
	// what the unit currently is; the listings read through here too, so a
	// held write is seen by every read, not only by GetRawUnitBytes.
	if data, ok := r.overlaid(unitUUID); ok {
		return data, nil
	}

	// Fast path: content cache hit (persistent daemon only).
	if r.contentCache != nil {
		if data, ok := r.contentCache[unitUUID]; ok {
			return data, nil
		}
	}

	data, err := r.readMprContentsUncached(unitUUID)
	if err != nil {
		return nil, err
	}

	// Populate cache (persistent daemon only).
	if r.contentCache != nil {
		r.contentCache[unitUUID] = data
	}
	return data, nil
}

func (r *Reader) readMprContentsUncached(unitUUID string) ([]byte, error) {
	if len(unitUUID) < 4 {
		return nil, fmt.Errorf("invalid unit UUID: %s", unitUUID)
	}

	// Build path: mprcontents/XX/YY/UUID.mxunit
	path := filepath.Join(
		r.contentsDir,
		unitUUID[0:2],
		unitUUID[2:4],
		unitUUID+".mxunit",
	)
	return os.ReadFile(path)
}

// getTypeFromContents extracts the $Type field from BSON contents.
// Uses bson.Raw.LookupErr for O(1) field extraction instead of unmarshalling
// the entire document into map[string]any.
func getTypeFromContents(contents []byte) string {
	if len(contents) == 0 {
		return ""
	}
	val, err := bson.Raw(contents).LookupErr("$Type")
	if err != nil {
		return ""
	}
	s, ok := val.StringValueOK()
	if !ok {
		return ""
	}
	return s
}

func getNameFromContents(contents []byte) string {
	if len(contents) == 0 {
		return ""
	}
	val, err := bson.Raw(contents).LookupErr("Name")
	if err != nil {
		return ""
	}
	name, _ := val.StringValueOK()
	return name
}

// buildUnitNameIndex builds the qualified-name index from top-level BSON
// headers. V2 reuses the metadata pass that listUnitsByType already requires;
// V1 streams rows so full BSON documents are not retained in memory.
func (r *Reader) buildUnitNameIndex() error {
	r.nameIndexMu.Lock()
	defer r.nameIndexMu.Unlock()
	if r.nameIndexBuilt {
		return nil
	}

	headers, err := r.loadUnitHeaders()
	if err != nil {
		return err
	}
	moduleNames := make(map[string]string)
	containerParent := make(map[string]string, len(headers))
	for _, h := range headers {
		containerParent[h.ID] = h.ContainerID
		if h.Type == "Projects$ModuleImpl" || h.Type == "Projects$Module" {
			moduleNames[h.ID] = h.Name
		}
	}

	index := make(map[string]nameIndexEntry, len(headers))
	for _, h := range headers {
		if h.Name == "" {
			continue
		}
		moduleName := ResolveModuleName(h.ContainerID, moduleNames, containerParent)
		qualifiedName := h.Name
		if moduleName != "" {
			qualifiedName = moduleName + "." + h.Name
		}
		index[h.Type+"\x00"+qualifiedName] = nameIndexEntry{
			ID:          h.ID,
			ContainerID: h.ContainerID,
			Type:        h.Type,
		}
	}
	r.nameIndex = index
	r.nameIndexBuilt = true
	return nil
}

func (r *Reader) loadUnitHeaders() ([]cachedUnit, error) {
	if r.version == MPRVersionV2 {
		if !r.unitCacheValid {
			if err := r.buildUnitCache(); err != nil {
				return nil, err
			}
		}
		return r.unitCache, nil
	}

	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName, Contents
		FROM Unit
	`)
	if err != nil {
		return nil, fmt.Errorf("query unit headers: %w", err)
	}
	defer rows.Close()

	var headers []cachedUnit
	for rows.Next() {
		var unitID, containerID, contents []byte
		var containmentName string
		if err := rows.Scan(&unitID, &containerID, &containmentName, &contents); err != nil {
			return nil, fmt.Errorf("scan unit header: %w", err)
		}
		headers = append(headers, cachedUnit{
			ID:              blobToUUID(unitID),
			ContainerID:     blobToUUID(containerID),
			ContainmentName: containmentName,
			Type:            getTypeFromContents(contents),
			Name:            getNameFromContents(contents),
		})
	}
	return headers, rows.Err()
}

// GetUnitByName resolves a top-level document by qualified name and reads only
// that unit's full BSON after the lightweight index has been built.
//
// objectType is a human-friendly alias ("microflow", "page", …) resolved through
// rawUnitBSONType, which covers only the dozen types that alias table lists.
// Callers that already know the concrete gen type should prefer
// mprread.GetUnitByName[T], which derives the storage name from the codec
// registry and therefore works for every registered type.
func (r *Reader) GetUnitByName(objectType, qualifiedName string) (*UnitRef, error) {
	typeName := rawUnitBSONType(objectType)
	if typeName == "" {
		return nil, fmt.Errorf("unsupported object type: %s", objectType)
	}
	return r.GetUnitByTypeName(typeName, qualifiedName)
}

// GetUnitByTypeName resolves a top-level document by BSON $Type and qualified
// name, reading only that unit's full BSON after the lightweight index has been
// built. Returns (nil, nil) when no such document exists.
//
// This is the type-name-keyed core of GetUnitByName: it takes the storage name
// directly, so it needs no entry in the rawUnitBSONType alias table.
func (r *Reader) GetUnitByTypeName(typeName, qualifiedName string) (*UnitRef, error) {
	if typeName == "" {
		return nil, fmt.Errorf("empty BSON type name")
	}
	if err := r.buildUnitNameIndex(); err != nil {
		return nil, err
	}

	r.nameIndexMu.RLock()
	entry, ok := r.nameIndex[typeName+"\x00"+qualifiedName]
	r.nameIndexMu.RUnlock()
	if !ok {
		return nil, nil
	}
	contents, err := r.GetRawUnitBytes(entry.ID)
	if err != nil {
		return nil, err
	}
	return &UnitRef{
		ID:          entry.ID,
		ContainerID: entry.ContainerID,
		Type:        entry.Type,
		Contents:    contents,
	}, nil
}

// RawUnitInfo contains information about a raw unit for BSON debugging.
// Aliased to mdl/types.RawUnitInfo so reader_raw.go methods and modelsdk/codec
// consumers share a single concrete struct.
type RawUnitInfo = types.RawUnitInfo

// DecodedUnits returns the memoized decoded units for a BSON $Type, if any.
// The value is an opaque []mprread.Unit[T]; see
// mprread.ListUnitsWithContainerCached, the only intended caller.
func (r *Reader) DecodedUnits(typeName string) (any, bool) {
	r.decodedMu.RLock()
	defer r.decodedMu.RUnlock()
	v, ok := r.decoded[typeName]
	return v, ok
}

// SetDecodedUnits memoizes decoded units for a BSON $Type until the next
// InvalidateCache. See DecodedUnits.
func (r *Reader) SetDecodedUnits(typeName string, v any) {
	r.decodedMu.Lock()
	defer r.decodedMu.Unlock()
	if r.decoded == nil {
		r.decoded = make(map[string]any)
	}
	r.decoded[typeName] = v
}
