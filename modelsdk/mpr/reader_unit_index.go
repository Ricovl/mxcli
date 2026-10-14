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

// UnitHeaderReads counts unit files read to build the header index (tests).
func (r *Reader) UnitHeaderReads() int64 { return r.headerReads.Load() }
