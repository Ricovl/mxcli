// SPDX-License-Identifier: Apache-2.0

package mprread_test

import (
	"testing"

	genEnum "github.com/mendixlabs/mxcli/modelsdk/gen/enumerations"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genProj "github.com/mendixlabs/mxcli/modelsdk/gen/projects"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// TestGetUnitByName_MatchesTheListedUnit pins the by-name lookup against the
// list-and-scan it replaces: same element, for a document nested in a folder
// (so module resolution has to walk the container chain, not just read the
// unit's own parent).
func TestGetUnitByName_MatchesTheListedUnit(t *testing.T) {
	r := openTestReader(t)

	const want = "Administration.ChangeMyPassword"
	unit, err := mprread.GetUnitByName[*genMf.Microflow](r, want)
	if err != nil {
		t.Fatalf("GetUnitByName[Microflow](%q): %v", want, err)
	}
	if unit == nil {
		t.Fatalf("GetUnitByName[Microflow](%q) = nil, want the microflow", want)
	}
	if unit.Element.Name() != "ChangeMyPassword" {
		t.Errorf("Name = %q, want %q", unit.Element.Name(), "ChangeMyPassword")
	}

	listed, err := mprread.ListUnitsWithContainer[*genMf.Microflow](r)
	if err != nil {
		t.Fatalf("ListUnitsWithContainer: %v", err)
	}
	var found bool
	for _, u := range listed {
		if u.Element.ID() != unit.Element.ID() {
			continue
		}
		found = true
		if u.ContainerID != unit.ContainerID {
			t.Errorf("ContainerID = %q, want %q (the list-and-scan answer)", unit.ContainerID, u.ContainerID)
		}
	}
	if !found {
		t.Errorf("by-name lookup returned ID %q, which the full listing does not contain", unit.Element.ID())
	}
}

// TestGetUnitByName_MissingIsNotAnError distinguishes "no such document" from a
// failed read: callers branch on the nil to report NotFound.
func TestGetUnitByName_MissingIsNotAnError(t *testing.T) {
	r := openTestReader(t)

	unit, err := mprread.GetUnitByName[*genMf.Microflow](r, "Administration.NoSuchFlow")
	if err != nil {
		t.Fatalf("missing lookup returned an error: %v", err)
	}
	if unit != nil {
		t.Fatalf("missing lookup = %#v, want nil", unit)
	}
}

// TestGetUnitByName_ResolvesTypesTheAliasTableOmits is the reason this helper
// exists rather than a per-type method on the Reader. mpr.Reader.GetUnitByName
// resolves its objectType through rawUnitBSONType, a hand-maintained table of a
// dozen aliases; this derives the storage name from the codec registry, so a
// type nobody added to that table still resolves.
//
// Projects$Folder and Enumerations$Enumeration stand in for the two cases:
// Folder has no alias at all, Enumeration has one. Both must work here.
func TestGetUnitByName_ResolvesTypesTheAliasTableOmits(t *testing.T) {
	r := openTestReader(t)

	if _, err := r.GetUnitByName("folder", "Administration.User Management"); err == nil {
		t.Fatal("Reader.GetUnitByName(\"folder\", …) succeeded — the alias table gained an " +
			"entry, so this test no longer demonstrates anything; pick another unaliased type")
	}

	folder, err := mprread.GetUnitByName[*genProj.Folder](r, "Administration.User Management")
	if err != nil {
		t.Fatalf("GetUnitByName[Folder]: %v", err)
	}
	if folder == nil {
		t.Fatal("GetUnitByName[Folder] = nil, want the folder the alias table cannot name")
	}
	if folder.Element.Name() != "User Management" {
		t.Errorf("folder Name = %q, want %q", folder.Element.Name(), "User Management")
	}

	enum, err := mprread.GetUnitByName[*genEnum.Enumeration](r, "FeedbackModule.LogNodes")
	if err != nil {
		t.Fatalf("GetUnitByName[Enumeration]: %v", err)
	}
	if enum == nil {
		t.Fatal("GetUnitByName[Enumeration] = nil")
	}
	if enum.Element.Name() != "LogNodes" {
		t.Errorf("enum Name = %q, want %q", enum.Element.Name(), "LogNodes")
	}
}

// TestListUnitsWithContainerCached_DecodesOncePerInvalidation is the whole point
// of the cached variant: a DESCRIBE sweep calls the same List* once per document
// described, and each uncached call re-decodes the entire type.
//
// Element pointer identity is the observable — a fresh decode allocates new
// elements, so same-pointer means the decode was skipped.
func TestListUnitsWithContainerCached_DecodesOncePerInvalidation(t *testing.T) {
	r := openTestReader(t)

	first, err := mprread.ListUnitsWithContainerCached[*genEnum.Enumeration](r)
	if err != nil {
		t.Fatalf("first cached list: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("fixture has no enumerations; this test cannot detect a re-decode")
	}

	second, err := mprread.ListUnitsWithContainerCached[*genEnum.Enumeration](r)
	if err != nil {
		t.Fatalf("second cached list: %v", err)
	}
	if second[0].Element != first[0].Element {
		t.Fatal("second cached list re-decoded the type; the memo did not hit")
	}

	// A write invalidates the reader, and the memo must not outlive it —
	// otherwise every read after a write is served the pre-write model.
	r.InvalidateCache()
	third, err := mprread.ListUnitsWithContainerCached[*genEnum.Enumeration](r)
	if err != nil {
		t.Fatalf("post-invalidation list: %v", err)
	}
	if len(third) != len(first) {
		t.Fatalf("post-invalidation list has %d enumerations, want %d", len(third), len(first))
	}
	if third[0].Element == first[0].Element {
		t.Fatal("InvalidateCache did not drop the decoded-unit memo — a read after a write " +
			"would be served the stale model")
	}
}

// TestListUnitsWithContainerCached_MatchesUncached guards the memo against
// returning a different answer from the function it stands in for.
func TestListUnitsWithContainerCached_MatchesUncached(t *testing.T) {
	r := openTestReader(t)

	uncached, err := mprread.ListUnitsWithContainer[*genEnum.Enumeration](r)
	if err != nil {
		t.Fatalf("ListUnitsWithContainer: %v", err)
	}
	cached, err := mprread.ListUnitsWithContainerCached[*genEnum.Enumeration](r)
	if err != nil {
		t.Fatalf("ListUnitsWithContainerCached: %v", err)
	}
	if len(cached) != len(uncached) {
		t.Fatalf("cached list has %d entries, uncached has %d", len(cached), len(uncached))
	}
	for i := range uncached {
		if cached[i].Element.ID() != uncached[i].Element.ID() {
			t.Errorf("entry %d: cached ID %q, uncached ID %q", i, cached[i].Element.ID(), uncached[i].Element.ID())
		}
		if cached[i].ContainerID != uncached[i].ContainerID {
			t.Errorf("entry %d: cached ContainerID %q, uncached %q", i, cached[i].ContainerID, uncached[i].ContainerID)
		}
	}
}

// TestListUnitsWithContainerCached_EnvOptOut pins the escape hatch: with the
// memo off the results must still be correct, just re-decoded. Without this the
// opt-out could silently return stale or empty results and nothing would notice
// until someone set the variable in anger.
func TestListUnitsWithContainerCached_EnvOptOut(t *testing.T) {
	r := openTestReader(t)

	warm, err := mprread.ListUnitsWithContainerCached[*genEnum.Enumeration](r)
	if err != nil {
		t.Fatalf("warming the memo: %v", err)
	}
	if len(warm) == 0 {
		t.Fatal("fixture has no enumerations")
	}

	t.Setenv("MXCLI_NO_DECODE_CACHE", "1")
	off, err := mprread.ListUnitsWithContainerCached[*genEnum.Enumeration](r)
	if err != nil {
		t.Fatalf("opted-out list: %v", err)
	}
	if len(off) != len(warm) {
		t.Fatalf("opted-out list has %d entries, want %d", len(off), len(warm))
	}
	if off[0].Element == warm[0].Element {
		t.Fatal("MXCLI_NO_DECODE_CACHE did not bypass the memo — it was served the cached decode")
	}
	for i := range warm {
		if off[i].Element.ID() != warm[i].Element.ID() {
			t.Fatalf("entry %d: opted-out ID %q, cached ID %q", i, off[i].Element.ID(), warm[i].Element.ID())
		}
	}
}
