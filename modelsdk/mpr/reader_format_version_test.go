// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// ProjectVersion().FormatVersion is what SHOW VERSION, STATUS and the session
// log's `mpr_format` print. It must be the storage format the reader detected to
// open the project — Version() — not a second answer worked out from _MetaData.
//
// The second answer was wrong for every v1 project: _MetaData._FormatVersion
// exists only in the v2 schema (measured: absent in a 9.24.30 and an 11.13.0 v1
// project, present and 2 in 11.6.6/11.13.0/11.14.0 v2 ones), and the fallback
// for a missing column defaulted to 2, so a v1 project reported `MPR Format: v2`
// (ako/mxcli#1553, upstream mendixlabs/mxcli#1621).
func TestProjectVersionReportsTheDetectedStorageFormat(t *testing.T) {
	// A v2-schema .mpr copied away from its mprcontents/ folder: the directory
	// test says v1 and the Unit.Contents reconciliation corrects it to v2. The
	// format must be taken after that correction, not before it.
	detached := filepath.Join(t.TempDir(), "minimal.mpr")
	data, err := os.ReadFile(v2Fixture)
	if err != nil {
		t.Fatalf("read v2 fixture: %v", err)
	}
	if err := os.WriteFile(detached, data, 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		path string
		want MPRVersion
	}{
		{"v1 project (no _FormatVersion column)", v1Fixture, MPRVersionV1},
		{"v2 project (control: _FormatVersion = 2)", v2Fixture, MPRVersionV2},
		{"v2 schema without its mprcontents folder", detached, MPRVersionV2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Open(tc.path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer r.Close()

			// Precondition: the reader's own detection is right. Without this a
			// wrong fixture would make the assertion below vacuous.
			if r.Version() != tc.want {
				t.Fatalf("Version() = %d, want %d — fixture is not the format this case is about", r.Version(), tc.want)
			}
			if got := r.ProjectVersion().FormatVersion; got != int(tc.want) {
				t.Errorf("ProjectVersion().FormatVersion = %d, want %d (Version() = %d) — "+
					"SHOW VERSION would print `MPR Format: v%d`", got, tc.want, r.Version(), got)
			}
		})
	}
}

// OpenWithDB (the writer's shared-connection path) is told the format by its
// caller rather than probing for it. It must report that, too.
func TestOpenWithDBReportsTheFormatItWasGiven(t *testing.T) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", v1Fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	r, err := OpenWithDB(db, v1Fixture, "")
	if err != nil {
		t.Fatalf("OpenWithDB: %v", err)
	}
	defer r.Close() // does not close db: the caller owns it
	if r.Version() != MPRVersionV1 {
		t.Fatalf("Version() = %d, want 1", r.Version())
	}
	if got := r.ProjectVersion().FormatVersion; got != int(MPRVersionV1) {
		t.Errorf("ProjectVersion().FormatVersion = %d, want 1", got)
	}
}

// DetectFromDB reads _MetaData, which cannot say which storage format the
// project is in — that is decided by what is on disk. It must leave the field
// for the reader to fill in rather than guess, because a guess is what shipped.
func TestDetectFromDBDoesNotGuessTheStorageFormat(t *testing.T) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", v1Fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	pv, err := version.DetectFromDB(db)
	if err != nil {
		t.Fatalf("DetectFromDB: %v", err)
	}
	if pv.ProductVersion == "" {
		t.Fatal("DetectFromDB read no product version from the v1 fixture")
	}
	if pv.FormatVersion != 0 {
		t.Errorf("DetectFromDB set FormatVersion = %d on a database with no _FormatVersion column; "+
			"the storage format is the reader's to report", pv.FormatVersion)
	}
}
