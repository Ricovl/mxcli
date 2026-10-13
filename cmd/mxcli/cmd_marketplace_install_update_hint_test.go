// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

// An install of a module that is already present still refuses to update it,
// but names the command that does (mendixlabs/mxcli#1442) rather than handing
// the user off to Studio Pro, which a headless environment does not have.
func TestReportModuleAlreadyInstalled_PointsToMarketplaceUpdate(t *testing.T) {
	var out bytes.Buffer
	reportModuleAlreadyInstalled(&out, "Administration", "4.3.2", "version 4.5.0", 23513, "4.5.0", "X/X.mpr")
	got := out.String()
	for _, want := range []string{
		`Module "Administration" is already installed (version 4.3.2).`,
		"not updated by install",
		"mxcli marketplace diff 23513 -p X/X.mpr --to 4.5.0",
		"mxcli marketplace update 23513 -p X/X.mpr --to 4.5.0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Update via Studio Pro") {
		t.Errorf("still hands off to Studio Pro:\n%s", got)
	}

	// --file has no content id or marketplace version: the command is named
	// with placeholders instead of invented values.
	out.Reset()
	reportModuleAlreadyInstalled(&out, "CompanyTheme", "", "from CompanyTheme.mpk", 0, "", "app.mpr")
	got = out.String()
	if !strings.Contains(got, "mxcli marketplace update <content-id> -p app.mpr --to <version>") {
		t.Errorf("--file install: no update command named:\n%s", got)
	}
}
