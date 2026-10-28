// SPDX-License-Identifier: Apache-2.0

// Package version provides Mendix project version detection and handling.
package version

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ProjectVersion is an alias for types.ProjectVersion, the canonical
// declaration. All its methods — IsAtLeast, IsAtLeastFull, String, IsMPRv2 —
// are defined there.
//
// It is an ALIAS and not a struct of its own on purpose. This package used to
// declare a duplicate whose fields matched types.ProjectVersion exactly, which
// made the two unrelated Go types that print under the same name: a value could
// not cross the mdl/ ↔ modelsdk/ boundary without a conversion, and a mismatch
// reported itself as a tautology rather than as a type error anyone could read.
// The sdk/mpr copy deleted in the legacy-engine retirement aliased the canonical
// type; this one did not, and that asymmetry is what CLAUDE.md's shared-types
// rule exists to prevent.
type ProjectVersion = types.ProjectVersion

// DefaultVersion returns the default version (11.6.0) used when detection fails.
//
// FormatVersion is left zero, as DetectFromDB leaves it: see there.
func DefaultVersion() *ProjectVersion {
	return &ProjectVersion{
		ProductVersion: "11.6.0",
		BuildVersion:   "11.6.0",
		MajorVersion:   11,
		MinorVersion:   6,
		PatchVersion:   0,
	}
}

// DetectFromDB reads version information from the MPR database.
//
// It does NOT set FormatVersion. Whether a project is MPR v1 or v2 is a fact
// about what is on disk — an mprcontents/ folder beside the .mpr, and a Unit
// table with or without a Contents column — and the reader decides it from
// exactly that before it can read a single unit (mpr.OpenWithOptions). The
// reader stamps its answer onto the ProjectVersion it returns, so there is one
// answer to the question, not two.
//
// _MetaData cannot stand in for it. Its _FormatVersion column exists only in
// the v2 schema — measured absent in a 9.24.30 and an 11.13.0 v1 project,
// present and 2 in 11.6.6, 11.13.0 and 11.14.0 v2 ones — and this function used
// to read a missing column as "a newer schema, so 2". Every v1 project therefore
// reported `MPR Format: v2` (ako/mxcli#1553, upstream mendixlabs/mxcli#1621).
// The three columns read here are present in both schemas.
func DetectFromDB(db *sql.DB) (*ProjectVersion, error) {
	var productVersion, buildVersion, schemaHash string

	row := db.QueryRow("SELECT _ProductVersion, _BuildVersion, _SchemaHash FROM _MetaData LIMIT 1")
	if err := row.Scan(&productVersion, &buildVersion, &schemaHash); err != nil {
		if err == sql.ErrNoRows {
			// Return default if no metadata found
			return DefaultVersion(), nil
		}
		return nil, fmt.Errorf("failed to read version metadata: %w", err)
	}

	pv := &ProjectVersion{
		ProductVersion: productVersion,
		BuildVersion:   buildVersion,
		SchemaHash:     schemaHash,
	}

	// Parse version components
	pv.MajorVersion, pv.MinorVersion, pv.PatchVersion = parseVersion(productVersion)

	return pv, nil
}

// parseVersion extracts major, minor, patch from a version string like "10.18.0"
func parseVersion(version string) (major, minor, patch int) {
	parts := strings.Split(version, ".")
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return
}

// Removed with the alias: IsSupported, SupportsFeature, Feature and its
// constants, MinVersion, featureVersions and SupportedVersionRange.
//
// They could not survive as methods on an aliased type, and nothing called
// them — measured, zero references outside their own declarations, in this
// package or any other. They were also a second, hand-maintained copy of the
// feature registry: the live one is sdk/versions/mendix-{9,10,11}.yaml, read
// through checkFeature in mdl/executor/cmd_features.go, and featureVersions
// described itself as "the fallback when the YAML registry is unavailable" —
// a fallback with no caller is a list that can only drift.
