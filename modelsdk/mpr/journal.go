// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Journal makes a run of writes all-or-nothing: what every Writer in the process
// changes on disk while it is open can be put back with Rollback
// (mendixlabs/mxcli#1408).
//
// # Why
//
// `mxcli exec` applies a script one statement at a time, each written as it
// runs. A statement that fails at write time — after the pre-check passed — left
// every earlier statement on disk: a created module, a dropped document that the
// failing create was meant to replace. The model is valid, so nothing downstream
// notices that it is half a script.
//
// # How
//
// It is a before-image journal, not a write buffer. Writes still land as they
// always have — reconciled against what is stored, elided when they change
// nothing, carrying stored identities (ADR-0008) — so every later statement reads
// the project the earlier ones produced, and the identity rules see exactly what
// they see without a journal. What the journal adds is a copy of each file as it
// was before the run first touched it:
//
//   - the SQLite .mpr (with any -wal / -journal beside it), copied at the first
//     write to that project. Every write reaches storage through guardWrite
//     first, so the copy always predates the first change.
//   - each mprcontents/ unit file (MPR v2), copied, or noted as absent, the first
//     time the run writes, replaces or deletes it.
//
// Rollback puts the copies back and removes what the run created, which leaves
// the project byte-identical to where the run began. It must run after every
// connection to the project is closed: the .mpr is replaced underneath SQLite.
//
// Only the model is journaled. Files a statement writes beside it — the
// generated .java / .js source of a code action — are not.
type Journal struct {
	mu      sync.Mutex
	dir     string                // holds the before-images
	dbs     map[string][]dbBackup // keyed by absolute .mpr path
	files   map[string]string     // unit file -> its copy, "" when it did not exist
	dirs    []string              // directories the run created, innermost last
	n       int
	tainted error // a copy that failed; Rollback cannot be trusted past it
}

type dbBackup struct {
	path string // the live file: x.mpr, x.mpr-wal, x.mpr-journal
	copy string // "" when the file did not exist at the snapshot
}

var (
	journalMu     sync.Mutex
	activeJournal *Journal
)

// BeginJournal opens a journal for every write this process makes until Commit
// or Rollback. Only one can be open.
func BeginJournal() (*Journal, error) {
	journalMu.Lock()
	defer journalMu.Unlock()
	if activeJournal != nil {
		return nil, errors.New("a write journal is already open")
	}
	dir, err := os.MkdirTemp("", "mxcli-journal-")
	if err != nil {
		return nil, fmt.Errorf("open write journal: %w", err)
	}
	activeJournal = &Journal{dir: dir, dbs: map[string][]dbBackup{}, files: map[string]string{}}
	return activeJournal, nil
}

func currentJournal() *Journal {
	journalMu.Lock()
	defer journalMu.Unlock()
	return activeJournal
}

func (j *Journal) close() {
	journalMu.Lock()
	if activeJournal == j {
		activeJournal = nil
	}
	journalMu.Unlock()
	_ = os.RemoveAll(j.dir)
}

// Touched reports whether the run wrote anything that Rollback would restore.
func (j *Journal) Touched() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.dbs) > 0 || len(j.files) > 0
}

// Commit keeps what the run wrote and discards the before-images.
func (j *Journal) Commit() { j.close() }

// Rollback restores every journaled file to its state before the run. Every
// connection to the journaled projects must be closed first.
func (j *Journal) Rollback() error {
	defer j.close()
	j.mu.Lock()
	defer j.mu.Unlock()
	var errs []error
	if j.tainted != nil {
		errs = append(errs, fmt.Errorf("write journal incomplete: %w", j.tainted))
	}
	for path, copy := range j.files {
		if err := restoreFile(path, copy); err != nil {
			errs = append(errs, err)
		}
	}
	for i := len(j.dirs) - 1; i >= 0; i-- {
		_ = os.Remove(j.dirs[i]) // only succeeds when empty, as deleteUnit does
	}
	for _, backups := range j.dbs {
		for _, b := range backups {
			if err := restoreFile(b.path, b.copy); err != nil {
				errs = append(errs, err)
			}
		}
		// The shared-memory index describes the WAL just replaced; SQLite
		// rebuilds it on the next open.
		_ = os.Remove(backups[0].path + "-shm")
	}
	return errors.Join(errs...)
}

// journalDB snapshots a project's database before its first write in the run.
func journalDB(mprPath string) error {
	j := currentJournal()
	if j == nil {
		return nil
	}
	abs, err := filepath.Abs(mprPath)
	if err != nil {
		abs = mprPath
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, done := j.dbs[abs]; done {
		return nil
	}
	var backups []dbBackup
	for _, p := range []string{abs, abs + "-wal", abs + "-journal"} {
		c, err := j.backup(p)
		if err != nil {
			j.tainted = err
			return fmt.Errorf("write journal: %w", err)
		}
		backups = append(backups, dbBackup{path: p, copy: c})
	}
	j.dbs[abs] = backups
	return nil
}

// journalFile records a unit file's state before the run first changes it.
func journalFile(path string) error {
	j := currentJournal()
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, done := j.files[path]; done {
		return nil
	}
	// Directories the write is about to create, outermost first.
	var missing []string
	for d := filepath.Dir(path); ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil || d == filepath.Dir(d) {
			break
		}
		missing = append([]string{d}, missing...)
	}
	c, err := j.backup(path)
	if err != nil {
		j.tainted = err
		return fmt.Errorf("write journal: %w", err)
	}
	j.files[path] = c
	j.dirs = append(j.dirs, missing...)
	return nil
}

// backup copies path into the journal and returns the copy, or "" when path
// does not exist.
func (j *Journal) backup(path string) (string, error) {
	src, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer src.Close()
	j.n++
	dst := filepath.Join(j.dir, strconv.Itoa(j.n))
	if err := copyTo(dst, src); err != nil {
		return "", fmt.Errorf("copy %s: %w", path, err)
	}
	return dst, nil
}

// restoreFile puts a before-image back, or removes a file the run created. The
// copy is written beside the target and renamed over it, so a failure part-way
// leaves the file the run wrote rather than a truncated one.
func restoreFile(path, copy string) error {
	if copy == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("roll back %s: %w", path, err)
		}
		return nil
	}
	src, err := os.Open(copy)
	if err != nil {
		return fmt.Errorf("roll back %s: %w", path, err)
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("roll back %s: %w", path, err)
	}
	tmp := path + ".rollback"
	if err := copyTo(tmp, src); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("roll back %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("roll back %s: %w", path, err)
	}
	return nil
}

func copyTo(dst string, src io.Reader) error {
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
