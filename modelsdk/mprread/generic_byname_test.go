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
