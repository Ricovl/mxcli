// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// describeFileManager decodes a stored Forms$FileManager, shaped as the codec
// hands it to describe (int32 for MaxFileSize), and returns the MDL line.
func describeFileManager(t *testing.T, w map[string]any) string {
	t.Helper()
	ctx, buf := newMockCtx(t)
	raws := parseRawWidget(ctx, w)
	if len(raws) != 1 {
		t.Fatalf("parseRawWidget returned %d widgets", len(raws))
	}
	outputWidgetMDLV3(ctx, raws[0], 0)
	return strings.TrimSpace(buf.String())
}

// Reported on mendixlabs/mxcli#151: a page that is described and recreated
// silently drops the file manager. Describe printed it as a comment ("mxcli
// cannot author this widget"), so the replay had nothing to build.
func TestDescribeFileManager_RoundTrips(t *testing.T) {
	got := describeFileManager(t, map[string]any{
		"$Type": "Forms$FileManager", "Name": "fileManager1", "Editable": "Always",
		"Type": "Upload", "MaxFileSize": int32(10), "AllowedExtensions": ".xlsx",
		"ShowFileInBrowser": true,
	})
	if strings.Contains(got, "NOT re-executable") || strings.HasPrefix(got, "--") {
		t.Fatalf("describe still treats the file manager as unauthorable:\n\t%s", got)
	}
	for _, want := range []string{"filemanager fileManager1", "Type: Upload", "MaxFileSize: 10",
		"AllowedExtensions: '.xlsx'", "ShowFileInBrowser: true"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output lacks %q:\n\t%s", want, got)
		}
	}

	// And the line it prints builds the same widget back.
	fm, err := buildFileManagerFromMDL(t, got)
	if err != nil {
		t.Fatalf("describe output does not build: %v\n\t%s", err, got)
	}
	if fm.Type != pages.FileManagerTypeUpload || fm.MaxFileSize != 10 ||
		fm.AllowedExtensions != ".xlsx" || !fm.ShowFileInBrowser {
		t.Errorf("round trip changed the widget: %+v", fm)
	}
}

// Control: Studio Pro's defaults are not printed — emitting them would put
// clauses in the script the author never wrote — and still round-trip, since
// the builder's defaults are the same ones.
func TestDescribeFileManager_OmitsDefaults(t *testing.T) {
	got := describeFileManager(t, map[string]any{
		"$Type": "Forms$FileManager", "Name": "fileManager1", "Editable": "Always",
		"Type": "Both", "MaxFileSize": int32(5), "AllowedExtensions": "", "ShowFileInBrowser": false,
	})
	for _, absent := range []string{"Type:", "MaxFileSize", "AllowedExtensions", "ShowFileInBrowser"} {
		if strings.Contains(got, absent) {
			t.Errorf("describe invented %s for a default file manager:\n\t%s", absent, got)
		}
	}
	fm, err := buildFileManagerFromMDL(t, got)
	if err != nil {
		t.Fatalf("describe output does not build: %v\n\t%s", err, got)
	}
	if fm.Type != pages.FileManagerTypeBoth || fm.MaxFileSize != 5 {
		t.Errorf("defaults did not round-trip: %+v", fm)
	}
}
