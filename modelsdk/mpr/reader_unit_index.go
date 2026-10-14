// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// MPR v2 unit index: each unit's $Type, Name and Excluded, stamped with its
// file's size and mtime, persisted in .mxcli/unit-index. A unit whose file
// still matches its stamp is not read again.
// See docs/11-proposals/PROPOSAL_incremental_project_index.md.

// racyMargin: an mtime this recent is not trusted, since the file can change
// again without a visible size or mtime change (git's "racily clean").
const racyMargin = 2 * time.Second

const unitIndexFormat = 1

type unitIndexEntry struct {
	Type     string
	Name     string
	Excluded bool
	Size     int64
	MtimeNs  int64
	Racy     bool
}

type persistedUnitIndex struct {
	Format int
	Units  map[string]unitIndexEntry
}

func (r *Reader) unitFilePath(unitUUID string) string {
	return filepath.Join(r.contentsDir, unitUUID[0:2], unitUUID[2:4], unitUUID+".mxunit")
}

// unitIndexPath persists only into an existing .mxcli directory, so reading
// a project never creates one.
func (r *Reader) unitIndexPath() (string, bool) {
	if v := strings.TrimSpace(os.Getenv("MXCLI_NO_PERSISTED_INDEX")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return "", false
	}
	dir := filepath.Join(filepath.Dir(r.path), ".mxcli")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", false
	}
	return filepath.Join(dir, "unit-index"), true
}

// loadUnitIndex loads the persisted index once; anything unreadable counts
// as empty.
func (r *Reader) loadUnitIndex() map[string]unitIndexEntry {
	if r.unitIndexLoaded {
		return r.unitIndex
	}
	r.unitIndexLoaded = true
	path, ok := r.unitIndexPath()
	if !ok {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var p persistedUnitIndex
	if gob.NewDecoder(f).Decode(&p) != nil || p.Format != unitIndexFormat {
		return nil
	}
	r.unitIndex = p.Units
	return r.unitIndex
}

// saveUnitIndex writes via temp file and rename, so readers never see a torn
// file. Failure only costs the next process a full read.
func (r *Reader) saveUnitIndex() {
	path, ok := r.unitIndexPath()
	if !ok {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "unit-index-*")
	if err != nil {
		return
	}
	err = gob.NewEncoder(tmp).Encode(persistedUnitIndex{Format: unitIndexFormat, Units: r.unitIndex})
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// UnitStateFingerprint hashes every unit's ID, container and file size/mtime,
// fresh from disk (stat only). racy lists the units whose mtime is too recent
// to trust; UnitContentDigest covers those. supported is false on MPR v1, where
// the .mpr mtime covers every change.
func (r *Reader) UnitStateFingerprint() (fp string, racy []string, supported bool, err error) {
	if r.version != MPRVersionV2 {
		return "", nil, false, nil
	}
	rows, err := r.db.Query(`SELECT UnitID, ContainerID, ContainmentName FROM Unit`)
	if err != nil {
		return "", nil, true, fmt.Errorf("query units: %w", err)
	}
	defer rows.Close()
	racyAfter := time.Now().Add(-racyMargin).UnixNano()
	var lines []string
	for rows.Next() {
		var unitID, containerID []byte
		var containment string
		if err := rows.Scan(&unitID, &containerID, &containment); err != nil {
			return "", nil, true, fmt.Errorf("scan unit row: %w", err)
		}
		id := blobToUUID(unitID)
		var size, mtime int64 = -1, -1
		if fi, err := os.Stat(r.unitFilePath(id)); err == nil {
			size, mtime = fi.Size(), fi.ModTime().UnixNano()
			if mtime > racyAfter {
				racy = append(racy, id)
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s %s %d %d", id, blobToUUID(containerID), containment, size, mtime))
	}
	if err := rows.Err(); err != nil {
		return "", nil, true, err
	}
	sort.Strings(lines)
	sort.Strings(racy)
	return hashLines(lines), racy, true, nil
}

// UnitContentDigest hashes the contents of the given units' files.
func (r *Reader) UnitContentDigest(ids []string) string {
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		b, _ := r.readMprContentsUncached(id)
		sum := sha256.Sum256(b)
		lines = append(lines, id+" "+hex.EncodeToString(sum[:]))
	}
	return hashLines(lines)
}

func hashLines(lines []string) string {
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// UnitStamps re-stats every unit (dropping the reader's caches, which a
// changed file makes stale anyway) and returns each one's stamp. nil on MPR v1.
func (r *Reader) UnitStamps() ([]types.UnitStamp, error) {
	if r.version != MPRVersionV2 {
		return nil, nil
	}
	r.InvalidateCache()
	if err := r.buildUnitCache(); err != nil {
		return nil, err
	}
	out := make([]types.UnitStamp, 0, len(r.unitCache))
	for _, cu := range r.unitCache {
		e := r.unitIndex[cu.ID]
		mtime := e.MtimeNs
		if e.Racy {
			mtime = -1
		}
		out = append(out, types.UnitStamp{ID: cu.ID, Type: cu.Type, ContainerID: cu.ContainerID, Size: e.Size, MtimeNs: mtime})
	}
	return out, nil
}

// InvalidateUnit is InvalidateCache for a write to one unit: on MPR v2 it
// re-reads that unit's row and file into the unit cache instead of dropping
// it, so the next lookup does not re-stat every unit. Derived caches that a
// changed unit can affect are dropped.
func (r *Reader) InvalidateUnit(unitID string) {
	blob := uuidToBlob(unitID)
	if r.version != MPRVersionV2 || !r.unitCacheValid || blob == nil {
		r.InvalidateCache()
		return
	}
	id := blobToUUID(blob)
	var containerID []byte
	var containment string
	err := r.db.QueryRow(`SELECT ContainerID, ContainmentName FROM Unit WHERE UnitID = ?`, blob).Scan(&containerID, &containment)

	idx := -1
	for i := range r.unitCache {
		if r.unitCache[i].ID == id {
			idx = i
			break
		}
	}
	oldType := ""
	if idx >= 0 {
		oldType = r.unitCache[idx].Type
	}

	var cu cachedUnit
	present := err == nil
	if present {
		fi, serr := os.Stat(r.unitFilePath(id))
		contents, rerr := r.readMprContentsUncached(id)
		if serr != nil || rerr != nil {
			r.InvalidateCache()
			return
		}
		r.headerReads.Add(1)
		e := unitIndexEntry{
			Type:     getTypeFromContents(contents),
			Name:     getNameFromContents(contents),
			Excluded: isExcludedUnit(contents),
			Size:     fi.Size(),
			MtimeNs:  fi.ModTime().UnixNano(),
			Racy:     fi.ModTime().After(time.Now().Add(-racyMargin)),
		}
		if r.unitIndex == nil {
			r.unitIndex = map[string]unitIndexEntry{}
		}
		r.unitIndex[id] = e
		cu = cachedUnit{ID: id, ContainerID: blobToUUID(containerID), ContainmentName: containment, Type: e.Type, Name: e.Name, Excluded: e.Excluded}
		if held, ok := r.overlaid(id); ok {
			cu.Type, cu.Name, cu.Excluded = getTypeFromContents(held), getNameFromContents(held), isExcludedUnit(held)
		}
	} else {
		delete(r.unitIndex, id)
	}
	switch {
	case present && idx >= 0:
		r.unitCache[idx] = cu
	case present:
		r.unitCache = append(r.unitCache, cu)
	case idx >= 0:
		r.unitCache = append(r.unitCache[:idx], r.unitCache[idx+1:]...)
	}

	r.nameIndexMu.Lock()
	r.nameIndex, r.nameIndexBuilt = nil, false
	r.moduleNameByID, r.containerParent, r.folderIDs = nil, nil, nil
	r.headersByType, r.headerByID = nil, nil
	r.nameIndexMu.Unlock()
	r.decodedMu.Lock()
	delete(r.decoded, oldType)
	delete(r.decoded, cu.Type)
	r.decodedMu.Unlock()
	r.contentMu.Lock()
	delete(r.contentCache, id)
	r.contentMu.Unlock()
}

// UnitHeaderReads counts unit files read to build the header index (tests).
func (r *Reader) UnitHeaderReads() int64 { return r.headerReads.Load() }
