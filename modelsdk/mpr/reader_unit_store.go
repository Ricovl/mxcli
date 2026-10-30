// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// The unit store keeps a copy of every MPR v2 unit's bytes in one file,
// <project>/.mxcli/cache/unit-contents.db, so a process that reads the whole
// project opens that file instead of every .mxunit (ako/mxcli#1804).
//
// Opening a file is what Windows Defender scans, and a v2 project is one file
// per document: a catalog refresh of a 4,753-unit app opened 4,753 of them,
// after every save, and the same users who reported hours of waiting measured
// a 77% speed-up from excluding the project from scanning. Reading a unit's
// size and modification time does not open it, so the store is checked with
// a stat and a unit is read from its file only when it changed.
//
// A stored copy is served only while all three of these still hold:
//
//   - the file's size and modification time are what they were when it was
//     read. A write by Studio Pro, by mxcli, or by a git checkout changes
//     them; the stat is taken before the read, so a file caught mid-write is
//     stored under the old stat and never served for the new one;
//   - the Unit table's ContentsHash for it is the one it had then, which
//     catches a project copied over another with its timestamps preserved;
//   - the store was written by this format (unitStoreVersion).
//
// Anything else reads the file, as before. The store is a pure cache:
// absent, unreadable, locked by another process or unwritable all mean
// "read the file", never a wrong answer. It holds the bytes zstd-compressed
// (145 MB of units take about 60), and it lives in a directory of its own
// whose .gitignore ignores it, because .mxcli/ itself is not ignored in every
// project and a copy of the whole model must never be committed.

const (
	unitStoreDir     = "cache"
	unitStoreName    = "unit-contents.db"
	unitStoreVersion = "1"
)

// unitStoreMeta is what a stored copy was read under.
type unitStoreMeta struct {
	size  int64
	mtime int64 // UnixNano
	hash  string
}

type unitStorePending struct {
	meta unitStoreMeta
	data []byte
}

// unitStore is a reader's view of the persisted unit copies. All methods are
// safe for concurrent use: describes read units from several goroutines.
type unitStore struct {
	path string

	mu      sync.Mutex
	loaded  bool
	db      *sql.DB
	stored  map[string]unitStoreMeta    // unit ID -> what its stored copy was read under
	rowHash map[string]string           // unit ID -> the Unit table's ContentsHash now
	pending map[string]unitStorePending // read from a file since the last flush
	removed bool                        // rowHash shows units the store still holds
	foreign bool                        // the file is a store of another format
	hits    int64
	dec     *zstd.Decoder
	enc     *zstd.Encoder
}

func newUnitStore(projectPath string) *unitStore {
	if projectPath == "" {
		return nil
	}
	return &unitStore{
		path:    filepath.Join(filepath.Dir(projectPath), ".mxcli", unitStoreDir, unitStoreName),
		rowHash: map[string]string{},
		pending: map[string]unitStorePending{},
	}
}

// open loads the stored copies' metadata on first use. Caller holds mu.
func (s *unitStore) open() {
	if s.loaded {
		return
	}
	s.loaded = true
	s.stored = map[string]unitStoreMeta{}
	if _, err := os.Stat(s.path); err != nil {
		return
	}
	db, err := sql.Open("sqlite", "file:"+s.path+"?_pragma=busy_timeout(3000)")
	if err != nil {
		return
	}
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key = 'version'`).Scan(&version); err != nil || version != unitStoreVersion {
		// Only a store this build can tell is another format is replaced; one
		// it could not read (locked by another process) is left alone.
		s.foreign = err == nil
		db.Close()
		return
	}
	rows, err := db.Query(`SELECT id, size, mtime, hash FROM units`)
	if err != nil {
		db.Close()
		return
	}
	for rows.Next() {
		var id string
		var m unitStoreMeta
		if rows.Scan(&id, &m.size, &m.mtime, &m.hash) != nil {
			rows.Close()
			db.Close()
			s.stored = map[string]unitStoreMeta{}
			return
		}
		s.stored[id] = m
	}
	rows.Close()
	s.db = db
}

// noteRowHash records the Unit table's ContentsHash of a unit, which a stored
// copy must match. buildUnitCache calls it for every row before it reads any.
func (s *unitStore) noteRowHash(id, hash string) {
	s.mu.Lock()
	s.rowHash[id] = hash
	s.mu.Unlock()
}

// setRowHashes replaces the row hashes with the Unit table's as a whole, so a
// unit that has gone is neither served nor kept.
func (s *unitStore) setRowHashes(hashes map[string]string) {
	s.mu.Lock()
	s.rowHash = hashes
	s.removed = true
	s.mu.Unlock()
}

// lookup returns the stored copy of a unit if its file is unchanged, and the
// file's stat for recording a read when it is not (nil when the file cannot
// be stat'ed, which the read then reports).
func (s *unitStore) lookup(id, path string) ([]byte, os.FileInfo, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil, false
	}
	s.mu.Lock()
	s.open()
	m, stored := s.stored[id]
	hash, known := s.rowHash[id]
	db := s.db
	s.mu.Unlock()
	if !stored || !known || db == nil ||
		m.size != fi.Size() || m.mtime != fi.ModTime().UnixNano() || m.hash != hash {
		return nil, fi, false
	}
	var blob []byte
	if db.QueryRow(`SELECT data FROM units WHERE id = ?`, id).Scan(&blob) != nil {
		return nil, fi, false
	}
	data, err := s.decoder().DecodeAll(blob, nil)
	if err != nil || int64(len(data)) != fi.Size() {
		return nil, fi, false
	}
	s.mu.Lock()
	s.hits++
	s.mu.Unlock()
	return data, fi, true
}

// record keeps the bytes of a unit read from its file under the stat taken
// before the read, for the next flush.
func (s *unitStore) record(id string, fi os.FileInfo, data []byte) {
	if fi == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hash, known := s.rowHash[id]
	if !known {
		return
	}
	s.pending[id] = unitStorePending{
		meta: unitStoreMeta{size: fi.Size(), mtime: fi.ModTime().UnixNano(), hash: hash},
		data: data,
	}
}

func (s *unitStore) decoder() *zstd.Decoder {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dec == nil {
		s.dec, _ = zstd.NewReader(nil)
	}
	return s.dec
}

// flush writes what was read from files since the last flush, and drops the
// copies of units the project no longer has. Best effort: a failure leaves
// the store as it was, and the next process reads those files again.
func (s *unitStore) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open()
	var gone []string
	if s.removed {
		for id := range s.stored {
			if _, ok := s.rowHash[id]; !ok {
				gone = append(gone, id)
			}
		}
	}
	if len(s.pending) == 0 && len(gone) == 0 {
		return
	}
	if s.db == nil && !s.create() {
		return
	}
	if s.enc == nil {
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
		if err != nil {
			return
		}
		s.enc = enc
	}
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	put, err := tx.Prepare(`INSERT OR REPLACE INTO units (id, size, mtime, hash, data) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return
	}
	for id, p := range s.pending {
		if _, err := put.Exec(id, p.meta.size, p.meta.mtime, p.meta.hash, s.enc.EncodeAll(p.data, nil)); err != nil {
			put.Close()
			tx.Rollback()
			return
		}
	}
	put.Close()
	for _, id := range gone {
		if _, err := tx.Exec(`DELETE FROM units WHERE id = ?`, id); err != nil {
			tx.Rollback()
			return
		}
	}
	if tx.Commit() != nil {
		return
	}
	for id, p := range s.pending {
		s.stored[id] = p.meta
	}
	for _, id := range gone {
		delete(s.stored, id)
	}
	s.pending = map[string]unitStorePending{}
	s.removed = false
}

// create makes an empty store. Caller holds mu.
func (s *unitStore) create() bool {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	ignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(ignore); err != nil {
		_ = os.WriteFile(ignore, []byte("# mxcli caches; never commit\n*\n"), 0o644)
	}
	if _, err := os.Stat(s.path); err == nil {
		if !s.foreign {
			return false
		}
		_ = os.Remove(s.path)
	}
	db, err := sql.Open("sqlite", "file:"+s.path+"?_pragma=busy_timeout(3000)")
	if err != nil {
		return false
	}
	for _, stmt := range []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE units (id TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, hash TEXT, data BLOB)`,
		`INSERT INTO meta (key, value) VALUES ('version', '` + unitStoreVersion + `')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return false
		}
	}
	s.db = db
	return true
}

// close flushes and releases the store's file.
func (s *unitStore) close() {
	s.flush()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		s.db.Close()
		s.db = nil
	}
	s.loaded = false
	if s.dec != nil {
		s.dec.Close()
		s.dec = nil
	}
}
