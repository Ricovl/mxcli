// SPDX-License-Identifier: Apache-2.0

// The classic file manager (mendixlabs/mxcli#151).
//
// Reported: "MDL can't create a `Forms$FileManager` widget, which is needed for
// file upload in pages" — `pluggablewidget 'Forms$FileManager'` and
// `customwidget` both failed with "no definition for widget Forms$FileManager",
// and later `filemanager fileManager1 (…)` was refused as MDL-WIDGET25 "is not a
// widget in this project". The type was in the metamodel and in sdk/pages, but
// there was no keyword, builder or writer for it.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// buildFileManagerFromMDL runs the real parser and page builder over one widget.
func buildFileManagerFromMDL(t *testing.T, widget string) (*pages.FileManager, error) {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n  " + widget + "\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", widget, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	built, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		return nil, err
	}
	fm, ok := built.(*pages.FileManager)
	if !ok {
		t.Fatalf("built %T, want *pages.FileManager", built)
	}
	return fm, nil
}

func TestFileManager_BuildsWithProperties(t *testing.T) {
	fm, err := buildFileManagerFromMDL(t, "filemanager fileManager1 (Label: 'Excel file', "+
		"Type: Upload, MaxFileSize: 10, AllowedExtensions: '.xlsx;.xls', ShowFileInBrowser: true)")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if fm.TypeName != "Forms$FileManager" {
		t.Errorf("TypeName = %q, want Forms$FileManager", fm.TypeName)
	}
	if fm.Name != "fileManager1" {
		t.Errorf("Name = %q", fm.Name)
	}
	if fm.Label != "Excel file" {
		t.Errorf("Label = %q, want %q", fm.Label, "Excel file")
	}
	if fm.Type != pages.FileManagerTypeUpload {
		t.Errorf("Type = %q, want Upload", fm.Type)
	}
	if fm.MaxFileSize != 10 {
		t.Errorf("MaxFileSize = %d, want 10", fm.MaxFileSize)
	}
	if fm.AllowedExtensions != ".xlsx;.xls" {
		t.Errorf("AllowedExtensions = %q", fm.AllowedExtensions)
	}
	if !fm.ShowFileInBrowser {
		t.Error("ShowFileInBrowser = false, want true")
	}
}

// Control: a bare file manager gets the Model SDK's defaults (mendixmodelsdk
// 4.116.0, pages.FileManager: type Both, maxFileSize 5, allowedExtensions "",
// showFileInBrowser false) — not Go's zero values, which would write a 0 MB
// limit and an empty Type enum.
func TestFileManager_DefaultsMatchModelSDK(t *testing.T) {
	fm, err := buildFileManagerFromMDL(t, "filemanager fileManager1")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if fm.Type != pages.FileManagerTypeBoth {
		t.Errorf("Type = %q, want Both", fm.Type)
	}
	if fm.MaxFileSize != 5 {
		t.Errorf("MaxFileSize = %d, want 5", fm.MaxFileSize)
	}
	if fm.AllowedExtensions != "" || fm.ShowFileInBrowser {
		t.Errorf("unexpected non-defaults: %+v", fm)
	}
}

// MDL values are matched case-insensitively; the stored enum must be Mendix's
// own casing, since an unknown member is a document Studio Pro cannot load.
func TestFileManager_TypeIsCanonicalised(t *testing.T) {
	fm, err := buildFileManagerFromMDL(t, "filemanager f (Type: download)")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if fm.Type != pages.FileManagerTypeDownload {
		t.Errorf("Type = %q, want Download", fm.Type)
	}
}

func TestFileManager_RefusesBadValues(t *testing.T) {
	for _, tc := range []struct{ widget, want string }{
		{"filemanager f (Type: Uploads)", "Upload, Download, Both"},
		{"filemanager f (MaxFileSize: -1)", "MaxFileSize"},
	} {
		if _, err := buildFileManagerFromMDL(t, tc.widget); err == nil {
			t.Errorf("%s: accepted, want a refusal", tc.widget)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %q should mention %q", tc.widget, err, tc.want)
		}
	}
}

// `check` must not call a file manager's own properties "silently dropped on
// write" (MDL-WIDGET07) — and must still say so for a misspelt one.
func TestFileManager_PropsAreKnownToCheck(t *testing.T) {
	fm := &ast.WidgetV3{Type: "filemanager", Name: "f", Properties: map[string]any{
		"Type": "Upload", "MaxFileSize": 10, "AllowedExtensions": ".xlsx", "ShowFileInBrowser": true,
	}}
	if v := validateStaticWidgetUnknownProps(fm, "page P"); len(v) != 0 {
		t.Errorf("MDL-WIDGET07 on a file manager's own properties: %v", v)
	}
	typo := &ast.WidgetV3{Type: "filemanager", Name: "f", Properties: map[string]any{"MaxFileSise": 10}}
	if v := validateStaticWidgetUnknownProps(typo, "page P"); len(v) != 1 {
		t.Errorf("a misspelt property should still warn, got %v", v)
	}
}
