// SPDX-License-Identifier: Apache-2.0

package visitor

import "testing"

// `filemanager` is a built-in widget keyword (mendixlabs/mxcli#151). Parsed as a
// bare IDENTIFIER it was marked generic, so the validator required it to resolve
// to a pluggable widget definition and refused it as MDL-WIDGET25 "`filemanager`
// is not a widget in this project".
func TestFileManagerIsAnEnumeratedWidgetType(t *testing.T) {
	pg := buildOnePage(t, `create page P.Q (Title: 'x', Layout: A.B) {
  filemanager fileManager1 (Type: Upload, MaxFileSize: 10, AllowedExtensions: '.xlsx', ShowFileInBrowser: true)
}`)
	w := pg.Widgets[0]
	if w.Type != "filemanager" || w.Name != "fileManager1" {
		t.Fatalf("Type, Name = %q, %q", w.Type, w.Name)
	}
	if w.TypeIsGeneric {
		t.Error("TypeIsGeneric = true: `filemanager` must be the FILEMANAGER token, not a generic name " +
			"the validator resolves against pluggable widget definitions")
	}
}

// The keyword must not take the name away: an entity or widget called
// FileManager is an ordinary name elsewhere.
func TestFileManagerStaysUsableAsAName(t *testing.T) {
	pg := buildOnePage(t, `create page P.Q (Title: 'x', Layout: A.B) {
  container FileManager (Class: 'x')
}`)
	if w := pg.Widgets[0]; w.Name != "FileManager" {
		t.Errorf("Name = %q, want FileManager", w.Name)
	}
}
