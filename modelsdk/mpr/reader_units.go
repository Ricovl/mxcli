// SPDX-License-Identifier: Apache-2.0

// Package mpr - Unit listing infrastructure for Reader.
package mpr

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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

// UnitHeaderRef is a unit's identity and its top-level Name and Excluded.
type UnitHeaderRef struct {
	ID          string
	ContainerID string
	Name        string
	Excluded    bool
}

// ListUnitHeaders lists the units of exactly typeName with their Name and
// Excluded. On MPR v2 these come from the unit index, so a unit whose bytes the
// index has seen is not opened: naming every folder or microflow used to open
// every one of their files on each call (ako/mxcli#1791). MPR v1 reads the
// blobs, which live in the one database file.
func (r *Reader) ListUnitHeaders(typeName string) ([]UnitHeaderRef, error) {
	if r.version != MPRVersionV2 {
		units, err := r.listUnitsByType(typeName)
		if err != nil {
			return nil, err
		}
		out := make([]UnitHeaderRef, 0, len(units))
		for _, u := range units {
			e := unitIndexEntryOf(u.Contents)
			out = append(out, UnitHeaderRef{ID: u.ID, ContainerID: u.ContainerID, Name: e.Name, Excluded: e.Excluded})
		}
		return out, nil
	}
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}
	var out []UnitHeaderRef
	for _, cu := range r.unitCache {
		if cu.Type != typeName {
			continue
		}
		name, excluded := cu.Name, cu.Excluded
		if held, ok := r.overlaid(cu.ID); ok {
			// Held after the cache was built: the held bytes are the unit now.
			e := unitIndexEntryOf(held)
			if e.Type != typeName {
				continue
			}
			name, excluded = e.Name, e.Excluded
		}
		out = append(out, UnitHeaderRef{ID: cu.ID, ContainerID: cu.ContainerID, Name: name, Excluded: excluded})
	}
	return out, nil
}

// UnitByID returns the unit whose ID is id, reading that unit's contents only.
// ok is false when the project has no such unit. A unit's ID is its
// document's $ID, so a lookup of one document by ID needs no listing: the
// catalog's source build described each microflow through a listing of them
// all and opened millions of files on a large project (ako/mxcli#1789).
func (r *Reader) UnitByID(id string) (unit UnitRef, ok bool, err error) {
	if r.version != MPRVersionV2 {
		return r.unitByIDV1(id)
	}
	h, found, err := r.UnitHeaderByID(id)
	if err != nil || !found {
		return UnitRef{}, false, err
	}
	contents, err := r.readMprContents(h.ID)
	if err != nil {
		return UnitRef{}, false, err
	}
	return UnitRef{ID: h.ID, ContainerID: h.ContainerID, Type: h.Type, Contents: contents}, true, nil
}

// unitByIDV1 is UnitByID on MPR v1, where the contents are a column.
func (r *Reader) unitByIDV1(id string) (UnitRef, bool, error) {
	blob := uuidToBlob(id)
	if blob == nil {
		return UnitRef{}, false, nil
	}
	var containerID, contents []byte
	err := r.db.QueryRow("SELECT ContainerID, Contents FROM Unit WHERE UnitID = ?", blob).Scan(&containerID, &contents)
	if errors.Is(err, sql.ErrNoRows) {
		return UnitRef{}, false, nil
	}
	if err != nil {
		return UnitRef{}, false, err
	}
	if held, ok := r.overlaid(id); ok {
		contents = held
	}
	return UnitRef{ID: id, ContainerID: blobToUUID(containerID), Type: getTypeFromContents(contents), Contents: contents}, true, nil
}

// ListUnitsByTypeInContainer returns the units of exactly typeName whose
// ContainerID is containerID, reading the contents of those units only: a
// lookup of one module's domain model must not read every module's
// (mendixlabs/mxcli#1616).
func (r *Reader) ListUnitsByTypeInContainer(typeName, containerID string) ([]UnitRef, error) {
	var units []rawUnit
	var err error
	if r.version == MPRVersionV2 {
		units, err = r.listUnitsByTypeV2In(typeName, containerID)
	} else {
		units, err = r.listUnitsByTypeV1In(typeName, containerID)
	}
	if err != nil {
		return nil, err
	}
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// ListUnitsByTypeWhere is ListUnitsByType for the units whose ID keep
// accepts. On v2 the others' contents are never read: a catalog build that
// rebuilds a few units reads only those (ako/mxcli#1804).
func (r *Reader) ListUnitsByTypeWhere(typeName string, keep func(id string) bool) ([]UnitRef, error) {
	units, err := r.listUnitsByTypeWhere(typeName, keep)
	if err != nil {
		return nil, err
	}
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// listUnitsByTypeWhere is listUnitsByType for the units keep accepts (nil
// keeps all), filtered before any v2 contents are read.
func (r *Reader) listUnitsByTypeWhere(typeName string, keep func(id string) bool) ([]rawUnit, error) {
	if keep == nil {
		return r.listUnitsByType(typeName)
	}
	if r.version == MPRVersionV2 {
		return r.listUnitsByTypeV2Where(typeName, "", keep)
	}
	units, err := r.listUnitsByTypeV1(typeName)
	if err != nil {
		return nil, err
	}
	kept := units[:0]
	for _, u := range units {
		if keep(u.ID) {
			kept = append(kept, u)
		}
	}
	return kept, nil
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

// listUnitTypes returns every unit's metadata and $Type without reading the
// contents of any unit whose type is already indexed (Contents is nil): on v2
// the index is keyed by ContentsHash and persisted (reader_units_v2.go), on v1
// by blob length (reader_units_v1.go). For callers that need the project's
// shape, not its documents.
func (r *Reader) listUnitTypes() ([]rawUnit, error) {
	if r.version != MPRVersionV2 {
		return r.listUnitTypesV1()
	}
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}
	units := make([]rawUnit, 0, len(r.unitCache))
	for _, cu := range r.unitCache {
		units = append(units, rawUnit{ID: cu.ID, ContainerID: cu.ContainerID, ContainmentName: cu.ContainmentName, Type: cu.Type})
	}
	return units, nil
}

// listUnitsByTypeV2 handles MPR v2 format (contents in mprcontents folder).
// Uses caching to avoid reading every file for each query.
func (r *Reader) listUnitsByTypeV2(typeName string) ([]rawUnit, error) {
	return r.listUnitsByTypeV2In(typeName, "")
}

// listUnitsByTypeV2In is listUnitsByTypeV2 restricted to one container ("" for
// any), filtered before any contents are read.
func (r *Reader) listUnitsByTypeV2In(typeName, containerID string) ([]rawUnit, error) {
	return r.listUnitsByTypeV2Where(typeName, containerID, nil)
}

// listUnitsByTypeV2Where is listUnitsByTypeV2In for the units keep accepts
// (nil keeps all).
func (r *Reader) listUnitsByTypeV2Where(typeName, containerID string, keep func(id string) bool) ([]rawUnit, error) {
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}

	// Filter by type using cache, only read contents for matching units.
	var units []rawUnit
	for _, cu := range r.unitCache {
		if containerID != "" && cu.ContainerID != containerID {
			continue
		}
		if keep != nil && !keep(cu.ID) {
			continue
		}
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

// buildUnitCache reads all unit metadata once and caches it. A unit's $Type
// comes from the type index when its ContentsHash is known there, so only new
// and changed units are opened (reader_units_v2.go, mendixlabs/mxcli#1603).
func (r *Reader) buildUnitCache() error {
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName, COALESCE(ContentsHash, '')
		FROM Unit
	`)
	if err != nil {
		return fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	r.v2TypesMu.Lock()
	defer r.v2TypesMu.Unlock()
	r.contentMu.Lock()
	scoped := r.cacheScopes > 0
	r.contentMu.Unlock()

	r.unitCache = nil
	hashes := make(map[string]struct{})
	store := r.unitStore()
	rowHashes := make(map[string]string)
	opened := 0
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName, hash string

		if err := rows.Scan(&unitID, &containerID, &containmentName, &hash); err != nil {
			return fmt.Errorf("failed to scan unit row: %w", err)
		}

		unitUUID := blobToUUID(unitID)
		rowHashes[unitUUID] = hash
		if store != nil {
			store.noteRowHash(unitUUID, hash)
		}
		var entry unitIndexEntry
		if held, ok := r.overlaid(unitUUID); ok {
			// Bytes held in memory are what the unit is now; its stored hash
			// describes the file it will replace.
			entry = unitIndexEntryOf(held)
		} else if known, ok := r.knownUnit(hash); ok {
			entry = known
		} else {
			// The few units a warm index misses are read again soon, so they are
			// cached. A cold build caches them all only inside a CacheUnitReads
			// scope, which ends; the daemon's lasting cache stops early instead.
			contents, err := r.readUnit(unitUUID, scoped || opened < unitTypeIndexPersistAfter)
			if err != nil {
				continue
			}
			opened++
			entry = unitIndexEntryOf(contents)
			r.learnUnit(contentsHash(contents), entry)
		}
		if hash != "" {
			hashes[hash] = struct{}{}
		}
		r.unitCache = append(r.unitCache, cachedUnit{
			ID:              unitUUID,
			ContainerID:     blobToUUID(containerID),
			ContainmentName: containmentName,
			Type:            entry.Type,
			Name:            entry.Name,
			Excluded:        entry.Excluded,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to scan units: %w", err)
	}

	if store != nil {
		store.setRowHashes(rowHashes)
	}
	first := r.v2Hashes == nil
	r.v2Hashes = hashes
	if first || opened >= unitTypeIndexPersistAfter {
		r.persistUnitTypeIndex()
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
	r.moduleNameByID = nil
	r.containerParent = nil
	r.folderIDs = nil
	r.headersByType = nil
	r.headerByID = nil
	r.nameIndexMu.Unlock()
	r.decodedMu.Lock()
	r.decoded = nil
	r.decodedMu.Unlock()
	// Clear content cache entries but keep the map non-nil so caching stays active.
	// If contentCache is nil (per-request mode), remain disabled.
	r.contentMu.Lock()
	if r.contentCache != nil {
		clear(r.contentCache)
	}
	r.contentMu.Unlock()
}

// EnableContentCache activates the in-memory content cache for this reader.
// Call once after Connect in persistent daemon mode. The cache survives across
// requests; InvalidateCache empties it (but keeps caching active) on writes.
func (r *Reader) EnableContentCache() {
	r.contentMu.Lock()
	defer r.contentMu.Unlock()
	if r.contentCache == nil {
		r.contentCache = make(map[string][]byte)
	}
	r.cacheScopes = -1
}

// CacheUnitReads holds every unit read from mprcontents/ in memory until
// release is called, so a pass that looks documents up again and again reads
// each file once (mendixlabs/mxcli#1272: check -p re-read every microflow of
// the project several times per `create or modify` it predicted). Scopes
// nest; the cache is dropped when the outermost one is released, so it never
// outlives the pass — a long session must not keep serving bytes Studio Pro
// has since rewritten. A write through this reader empties it as before.
//
// MPR v1 reads its contents from SQLite, which caches its own pages; there
// the scope changes nothing.
func (r *Reader) CacheUnitReads() (release func()) {
	r.contentMu.Lock()
	defer r.contentMu.Unlock()
	if r.cacheScopes < 0 {
		return func() {} // cached for the reader's lifetime
	}
	if r.contentCache == nil {
		r.contentCache = make(map[string][]byte)
	}
	r.cacheScopes++
	var once sync.Once
	return func() {
		once.Do(func() {
			r.contentMu.Lock()
			last := false
			if r.cacheScopes > 0 {
				if r.cacheScopes--; r.cacheScopes == 0 {
					r.contentCache = nil
					last = true
				}
			}
			r.contentMu.Unlock()
			// A pass over the project has just read what it needed: keep it
			// for the next process (reader_unit_store.go).
			if store := r.unitStore(); last && store != nil {
				store.flush()
			}
		})
	}
}

// UnitFileReads is how many unit files this reader has read from
// mprcontents/. For tests.
func (r *Reader) UnitFileReads() int64 { return r.fileReads.Load() }

// UnitStoreHits is how many unit reads this reader served from the persisted
// store instead of a file. For tests.
func (r *Reader) UnitStoreHits() int64 {
	if store := r.unitStore(); store != nil {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.hits
	}
	return 0
}

// unitStore is the reader's persisted unit store, or nil when there is none:
// MPR v1 holds its units in the .mpr, and a reader without a project path
// has nowhere to keep one.
func (r *Reader) unitStore() *unitStore {
	r.storeOnce.Do(func() {
		if r.version == MPRVersionV2 && r.contentsDir != "" {
			r.store = newUnitStore(r.path)
		}
	})
	return r.store
}

// readMprContents reads content from the mprcontents folder for v2 format.
// The path is: mprcontents/XX/YY/UUID.mxunit where XX and YY are first two chars of UUID.
//
// While the content cache is on (EnableContentCache, CacheUnitReads) a unit is
// read from disk once and served from memory after that, as a copy: every
// caller gets bytes of its own, as os.ReadFile gave it, so one that edits them
// cannot change what the next caller reads.
func (r *Reader) readMprContents(unitUUID string) ([]byte, error) {
	return r.readUnit(unitUUID, true)
}

// readUnit is readMprContents; with fill false it does not add the unit to
// the content cache, so building the index does not hold every unit it reads.
func (r *Reader) readUnit(unitUUID string, fill bool) ([]byte, error) {
	if len(unitUUID) < 4 {
		return nil, fmt.Errorf("invalid unit UUID: %s", unitUUID)
	}

	// Bytes held in memory (an import buffer, or a deferred run of writes) are
	// what the unit currently is; the listings read through here too, so a
	// held write is seen by every read, not only by GetRawUnitBytes.
	if data, ok := r.overlaid(unitUUID); ok {
		return data, nil
	}

	r.contentMu.Lock()
	cached, ok := r.contentCache[unitUUID]
	caching := r.contentCache != nil && fill
	r.contentMu.Unlock()
	if ok {
		return bytes.Clone(cached), nil
	}

	// Build path: mprcontents/XX/YY/UUID.mxunit
	path := filepath.Join(
		r.contentsDir,
		unitUUID[0:2],
		unitUUID[2:4],
		unitUUID+".mxunit",
	)
	// An unchanged unit is read from the persisted store rather than its
	// own file, which on Windows is what a virus scanner inspects (#1804).
	store := r.unitStore()
	var fi os.FileInfo
	if store != nil {
		stored, info, ok := store.lookup(unitUUID, path)
		if ok {
			if caching {
				r.contentMu.Lock()
				if r.contentCache != nil {
					r.contentCache[unitUUID] = bytes.Clone(stored)
				}
				r.contentMu.Unlock()
			}
			return stored, nil
		}
		fi = info
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r.fileReads.Add(1)

	// One copy serves both the in-memory cache and the store: neither hands
	// out the bytes it holds, only copies of them.
	held := bytes.Clone(data)
	if caching {
		r.contentMu.Lock()
		if r.contentCache != nil {
			r.contentCache[unitUUID] = held
		}
		r.contentMu.Unlock()
	}
	if store != nil {
		store.record(unitUUID, fi, held)
	}
	return data, nil
}

// contentsHash is a unit's ContentsHash as Studio Pro and the writer compute
// it: base64(SHA-256(contents)).
func contentsHash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return base64.StdEncoding.EncodeToString(sum[:])
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

// RawUnitInfo contains information about a raw unit for BSON debugging.
// Aliased to mdl/types.RawUnitInfo so reader_raw.go methods and modelsdk/codec
// consumers share a single concrete struct.
type RawUnitInfo = types.RawUnitInfo

// cachedUnitsOfType returns the MPR v2 unit cache's entries of one type, which
// carry the name without reading any file; ok is false on MPR v1.
func (r *Reader) cachedUnitsOfType(typeName string) (_ []cachedUnit, ok bool, err error) {
	if r.version != MPRVersionV2 {
		return nil, false, nil
	}
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, true, err
		}
	}
	var out []cachedUnit
	for _, cu := range r.unitCache {
		if cu.Type == typeName {
			out = append(out, cu)
		}
	}
	return out, true, nil
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
	var folderIDs []string
	for _, h := range headers {
		containerParent[h.ID] = h.ContainerID
		switch h.Type {
		case "Projects$ModuleImpl", "Projects$Module":
			moduleNames[h.ID] = h.Name
		case "Projects$Folder":
			folderIDs = append(folderIDs, h.ID)
		}
	}

	headersByType := make(map[string][]UnitHeaderInfo)
	headerByID := make(map[string]UnitHeaderInfo, len(headers))
	for _, h := range headers {
		info := UnitHeaderInfo{ID: h.ID, ContainerID: h.ContainerID, Type: h.Type, Name: h.Name, Excluded: h.Excluded}
		headersByType[h.Type] = append(headersByType[h.Type], info)
		headerByID[h.ID] = info
	}

	index := make(map[string][]nameIndexEntry, len(headers))
	for _, h := range headers {
		if h.Name == "" {
			continue
		}
		moduleName := ResolveModuleName(h.ContainerID, moduleNames, containerParent)
		qualifiedName := h.Name
		if moduleName != "" {
			qualifiedName = moduleName + "." + h.Name
		}
		key := h.Type + "\x00" + qualifiedName
		index[key] = append(index[key], nameIndexEntry{
			ID:          h.ID,
			ContainerID: h.ContainerID,
			Type:        h.Type,
		})
	}
	r.nameIndex = index
	r.moduleNameByID = moduleNames
	r.containerParent = containerParent
	r.folderIDs = folderIDs
	r.headersByType = headersByType
	r.headerByID = headerByID
	r.nameIndexBuilt = true
	return nil
}

// ModuleNameOf returns the name of the module enclosing unitID — through any
// depth of folders — or "" when it is in none (the project root, a project-level
// document). It answers from the header index, so it reads no unit contents.
func (r *Reader) ModuleNameOf(unitID string) (string, error) {
	if err := r.buildUnitNameIndex(); err != nil {
		return "", err
	}
	r.nameIndexMu.RLock()
	defer r.nameIndexMu.RUnlock()
	parent, ok := r.containerParent[unitID]
	if !ok || parent == unitID {
		return "", nil
	}
	return ResolveModuleName(parent, r.moduleNameByID, r.containerParent), nil
}

// ContainersInModule returns moduleID together with every folder nested under
// it, at any depth — the containers a document of that module can sit in. The
// map is the caller's to keep or modify.
func (r *Reader) ContainersInModule(moduleID string) (map[string]bool, error) {
	if err := r.buildUnitNameIndex(); err != nil {
		return nil, err
	}
	r.nameIndexMu.RLock()
	defer r.nameIndexMu.RUnlock()
	set := map[string]bool{moduleID: true}
	for changed := true; changed; {
		changed = false
		for _, id := range r.folderIDs {
			if !set[id] && set[r.containerParent[id]] {
				set[id] = true
				changed = true
			}
		}
	}
	return set, nil
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
			Excluded:        isExcludedUnit(contents),
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
// A name is not a unique key: a module may hold an excluded twin of a document
// (#914). The live one is returned when there is one, otherwise the first match,
// so a name whose every match is excluded still resolves. Twins are rare, so
// telling them apart reads only the few units that share the name.
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
	entries := r.nameIndex[typeName+"\x00"+qualifiedName]
	r.nameIndexMu.RUnlock()

	var first *UnitRef
	for _, entry := range entries {
		contents, err := r.GetRawUnitBytes(entry.ID)
		if err != nil {
			return nil, err
		}
		unit := &UnitRef{
			ID:          entry.ID,
			ContainerID: entry.ContainerID,
			Type:        entry.Type,
			Contents:    contents,
		}
		if len(entries) == 1 || !isExcludedUnit(contents) {
			return unit, nil
		}
		if first == nil {
			first = unit
		}
	}
	return first, nil
}

// UnitHeadersOfType returns the header of every unit whose $Type is typeName,
// from the index: no unit is read. The slice is shared; callers must not
// modify it.
func (r *Reader) UnitHeadersOfType(typeName string) ([]UnitHeaderInfo, error) {
	if err := r.buildUnitNameIndex(); err != nil {
		return nil, err
	}
	r.nameIndexMu.RLock()
	defer r.nameIndexMu.RUnlock()
	return r.headersByType[typeName], nil
}

// UnitHeaderByID returns the header of the unit with that ID, from the index;
// ok is false when the project has no such unit.
func (r *Reader) UnitHeaderByID(unitID string) (UnitHeaderInfo, bool, error) {
	if err := r.buildUnitNameIndex(); err != nil {
		return UnitHeaderInfo{}, false, err
	}
	r.nameIndexMu.RLock()
	defer r.nameIndexMu.RUnlock()
	h, ok := r.headerByID[unitID]
	return h, ok, nil
}

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

// isExcludedUnit reports a unit's top-level Excluded flag without decoding the
// rest of the document.
func isExcludedUnit(contents []byte) bool {
	val, err := bson.Raw(contents).LookupErr("Excluded")
	if err != nil {
		return false
	}
	excluded, _ := val.BooleanOK()
	return excluded
}

// UnitHeaderInfo is a unit's header as the index holds it: no contents.
type UnitHeaderInfo struct {
	ID          string
	ContainerID string
	Type        string
	Name        string
	Excluded    bool
}
