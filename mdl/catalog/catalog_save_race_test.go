// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestSaveToFile_ConcurrentWritersAndReaders is the guard for ako/mxcli#951
// item 2. Eight parallel `mxcli lint` runs on a fresh project copy all built the
// catalog and saved it to the same .mxcli/catalog.db. The save removed the file
// and wrote into the path in place: VACUUM INTO refused a file another process
// had just created, the manual-copy fallback then hit "table catalog_meta already
// exists" or "database is locked", and a reader could open a half-written file.
//
// The save must be atomic: every writer succeeds, and a reader sees either the
// old cache or a complete new one — never a partial file.
func TestSaveToFile_ConcurrentWritersAndReaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.db")

	newCat := func(t *testing.T, mode string) *Catalog {
		t.Helper()
		cat, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := cat.SetCacheInfo("/p/app.mpr", time.Unix(1700000000, 0), "11.8.0", mode, time.Second); err != nil {
			t.Fatalf("SetCacheInfo: %v", err)
		}
		return cat
	}

	// An existing cache is the common case (the project changed, so every
	// process rebuilds and overwrites it).
	seed := newCat(t, "fast")
	if err := seed.SaveToFile(path); err != nil {
		t.Fatalf("seed SaveToFile: %v", err)
	}
	seed.Close()

	const writers = 8
	cats := make([]*Catalog, writers)
	for i := range cats {
		cats[i] = newCat(t, "full")
	}
	defer func() {
		for _, c := range cats {
			c.Close()
		}
	}()

	stop := make(chan struct{})
	var readerErrs []error
	var mu sync.Mutex
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				c, err := NewFromFile(path)
				if err != nil {
					mu.Lock()
					readerErrs = append(readerErrs, err)
					mu.Unlock()
					continue
				}
				info, err := c.GetCacheInfo()
				c.Close()
				if err == nil && info.BuildMode != "fast" && info.BuildMode != "full" {
					err = fmt.Errorf("reader saw build mode %q (partial cache)", info.BuildMode)
				}
				if err != nil {
					mu.Lock()
					readerErrs = append(readerErrs, err)
					mu.Unlock()
				}
			}
		}()
	}

	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(c *Catalog) {
			defer wg.Done()
			<-start
			errs <- c.SaveToFile(path)
		}(cats[i])
	}
	close(start)
	wg.Wait()
	close(stop)
	readers.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent SaveToFile: %v", err)
		}
	}
	for _, err := range readerErrs {
		t.Errorf("concurrent reader: %v", err)
	}

	final, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("final cache does not open: %v", err)
	}
	defer final.Close()
	info, err := final.GetCacheInfo()
	if err != nil {
		t.Fatalf("final cache info: %v", err)
	}
	if info.BuildMode != "full" {
		t.Errorf("final cache build mode = %q, want full", info.BuildMode)
	}

	// No temporary files may be left behind next to the cache.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if n := e.Name(); n != "catalog.db" && n != "catalog.db-journal" && n != "catalog.db-wal" && n != "catalog.db-shm" {
			t.Errorf("leftover file next to the cache: %s", n)
		}
	}
}
