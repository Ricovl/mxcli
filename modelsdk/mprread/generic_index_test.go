// SPDX-License-Identifier: Apache-2.0

package mprread_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genPages "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// The lookups below answer from the reader's header index and read only the
// units they return; each used to read every unit of its type to filter. These
// tests pin each one to what the full decode of the type says.

// openReaderWithExcludedMicroflow is openTestReader with one microflow marked
// Excluded on disk, since the fixture has none and the flag would otherwise go
// unchecked.
func openReaderWithExcludedMicroflow(t *testing.T) *mmpr.Reader {
	t.Helper()
	dst := copyMPRTree(t, fixturePath, t.TempDir())
	off, on := []byte("\x08Excluded\x00\x00"), []byte("\x08Excluded\x00\x01")
	patched := false
	err := filepath.WalkDir(filepath.Join(filepath.Dir(dst), "mprcontents"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || patched {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !bytes.Contains(b, []byte("Microflows$Microflow\x00")) || !bytes.Contains(b, off) {
			return nil
		}
		patched = true
		return os.WriteFile(p, bytes.Replace(b, off, on, 1), 0o644)
	})
	if err != nil || !patched {
		t.Fatalf("mark a microflow excluded: patched=%v err=%v", patched, err)
	}
	r, err := mmpr.Open(dst)
	if err != nil {
		t.Fatalf("Open(%s): %v", dst, err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestListUnitHeaders_MatchesFullDecode(t *testing.T) {
	r := openReaderWithExcludedMicroflow(t)

	full, err := mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	if err != nil {
		t.Fatalf("ListUnitsWithContainer: %v", err)
	}
	if len(full) == 0 {
		t.Fatal("fixture has no microflows; the comparison checks nothing")
	}
	want := make(map[model.ID]mprread.UnitHeader, len(full))
	excluded := 0
	for _, u := range full {
		if u.Element.Excluded() {
			excluded++
		}
		id := model.ID(u.Element.ID())
		want[id] = mprread.UnitHeader{
			ID:          id,
			ContainerID: u.ContainerID,
			Name:        u.Element.Name(),
			Excluded:    u.Element.Excluded(),
		}
	}

	if excluded == 0 {
		t.Fatal("no excluded microflow after the patch; the Excluded flag is untested")
	}

	got, err := mprread.ListUnitHeaders[*genMf.Microflow](r)
	if err != nil {
		t.Fatalf("ListUnitHeaders: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("ListUnitHeaders returned %d headers, full decode has %d", len(got), len(want))
	}
	for _, h := range got {
		if w, ok := want[h.ID]; !ok {
			t.Errorf("header %s (%s) is not a microflow of the full decode", h.ID, h.Name)
		} else if h != w {
			t.Errorf("header %s = %+v, full decode says %+v", h.ID, h, w)
		}
	}
}

func TestListUnitsNamed_MatchesFullDecode(t *testing.T) {
	r := openTestReader(t)

	full, err := mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	if err != nil {
		t.Fatalf("ListUnitsWithContainer: %v", err)
	}
	byName := map[string][]string{}
	for _, u := range full {
		byName[u.Element.Name()] = append(byName[u.Element.Name()], string(u.Element.ID()))
	}
	byName["NoSuchMicroflow"] = nil

	for name, wantIDs := range byName {
		got, err := mprread.ListUnitsNamed[*genMf.Microflow](r, name)
		if err != nil {
			t.Fatalf("ListUnitsNamed(%q): %v", name, err)
		}
		var gotIDs []string
		for _, u := range got {
			if u.Element.Name() != name {
				t.Errorf("ListUnitsNamed(%q) returned %q", name, u.Element.Name())
			}
			gotIDs = append(gotIDs, string(u.Element.ID()))
		}
		sort.Strings(gotIDs)
		sort.Strings(wantIDs)
		if len(gotIDs) != len(wantIDs) {
			t.Errorf("ListUnitsNamed(%q) = %v, full decode has %v", name, gotIDs, wantIDs)
			continue
		}
		for i := range gotIDs {
			if gotIDs[i] != wantIDs[i] {
				t.Errorf("ListUnitsNamed(%q) = %v, full decode has %v", name, gotIDs, wantIDs)
				break
			}
		}
	}
}

func TestGetUnitByID_MatchesFullDecodeAndChecksType(t *testing.T) {
	r := openTestReader(t)

	full, err := mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	if err != nil {
		t.Fatalf("ListUnitsWithContainer: %v", err)
	}
	for _, u := range full {
		got, err := mprread.GetUnitByID[*genMf.Microflow](r, model.ID(u.Element.ID()))
		if err != nil {
			t.Fatalf("GetUnitByID(%s): %v", u.Element.ID(), err)
		}
		if got == nil {
			t.Errorf("GetUnitByID(%s) = nil, want microflow %s", u.Element.ID(), u.Element.Name())
			continue
		}
		if got.Element.Name() != u.Element.Name() || got.ContainerID != u.ContainerID {
			t.Errorf("GetUnitByID(%s) = %s in %s, full decode has %s in %s",
				u.Element.ID(), got.Element.Name(), got.ContainerID, u.Element.Name(), u.ContainerID)
		}
	}

	// A unit of another type under that ID is not a microflow.
	pages, err := mprread.ListUnitsWithContainer[*genPages.Page](r)
	if err != nil {
		t.Fatalf("ListUnitsWithContainer[Page]: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("fixture has no pages; the type check is untested")
	}
	if got, err := mprread.GetUnitByID[*genMf.Microflow](r, model.ID(pages[0].Element.ID())); err != nil || got != nil {
		t.Errorf("GetUnitByID[Microflow](a page's ID) = %v, %v; want nil, nil", got, err)
	}
	if got, err := mprread.GetUnitByID[*genMf.Microflow](r, "00000000-0000-0000-0000-000000000000"); err != nil || got != nil {
		t.Errorf("GetUnitByID[Microflow](unknown ID) = %v, %v; want nil, nil", got, err)
	}
}
