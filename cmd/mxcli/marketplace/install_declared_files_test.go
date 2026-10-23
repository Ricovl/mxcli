// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A module package's package.xml lists, under <files>, every project file the
// package installs. Everything else in the archive is the package's own
// metadata. Measured with `mx module-import` on Mendix 11.12.1, importing
// ExternalDatabaseConnector 6.3.0 and BusinessEvents 3.12.1 into a blank app:
// the files mx wrote were exactly the declared ones — not License.txt,
// manifest.json, the READMEOSS report or "Mendix EULA.docx", all of which sit at
// the root of those packages — and the one declared entry missing from the
// archive (the snowflake driver stripped from mx-modules/, see its README) was
// passed over without complaint.
const edcPackageXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns="http://www.mendix.com/package/1.0/">
  <modelerProject xmlns="http://www.mendix.com/modelerProject/1.0/">
    <module name="ExternalDatabaseConnector" />
    <projectFile path="project.mpr" />
    <files>
      <file path="vendorlib/gson-2.13.1.jar" />
      <file path="vendorlib/snowflake-jdbc-4.0.2.jar" />
      <file path="javasource/externaldatabaseconnector/pojo/QueryDetails.java" />
      <file path="themesource/externaldatabaseconnector/settings.json" />
    </files>
  </modelerProject>
</package>`

// TestInstallPackageFiles_LeavesPackageMetadataOutOfTheProject is
// mendixlabs/mxcli#1487 (ako/mxcli#1344), reported against an update of
// Nanoflow Commons 6.2.1 → 7.4.0:
//
//	the package's own metadata files end up in the **app root**:
//	- `manifest.json`
//	- `License.txt`
//	- `SiemensMendixNanoflowCommons__7.4.0__READMEOSS_<timestamp>.html`
//
// InstallPackageFiles copied every entry but project.mpr and package.xml, so
// whatever else a publisher put at the package root landed beside the .mpr.
func TestInstallPackageFiles_LeavesPackageMetadataOutOfTheProject(t *testing.T) {
	mpk := buildMPK(t, map[string]string{
		"package.xml": edcPackageXML,
		"project.mpr": "model",
		// Package metadata, at the root of every real package measured.
		"License.txt":   "license",
		"manifest.json": `{"name":"ExternalDatabaseConnector"}`,
		"SiemensMendixExternalDatabaseConnector__6.3.0__READMEOSS_2026-06-17__04-28-14.html": "<html/>",
		"Mendix EULA.docx": "eula",
		// Declared payload. The snowflake jar is declared and absent.
		"vendorlib/gson-2.13.1.jar":                                      "jar",
		"javasource/externaldatabaseconnector/pojo/QueryDetails.java":    "class QueryDetails {}",
		"themesource/externaldatabaseconnector/settings.json":            "{}",
		"themesource/externaldatabaseconnector/undeclared-but-shipped.x": "not in package.xml",
	})
	proj := t.TempDir()

	written, skipped, err := InstallPackageFiles(mpk, proj)
	if err != nil {
		t.Fatalf("InstallPackageFiles: %v", err)
	}

	for _, name := range []string{"License.txt", "manifest.json",
		"SiemensMendixExternalDatabaseConnector__6.3.0__READMEOSS_2026-06-17__04-28-14.html",
		"Mendix EULA.docx"} {
		if _, err := os.Stat(filepath.Join(proj, name)); err == nil {
			t.Errorf("%s was written into the app root; it is package metadata, not a project file", name)
		}
	}
	if _, err := os.Stat(filepath.Join(proj, "themesource", "externaldatabaseconnector", "undeclared-but-shipped.x")); err == nil {
		t.Errorf("an entry package.xml does not list was installed")
	}

	// Control: the declared payload must still arrive, or the assertions above
	// pass because nothing was installed at all.
	want := []string{
		filepath.Join("javasource", "externaldatabaseconnector", "pojo", "QueryDetails.java"),
		filepath.Join("themesource", "externaldatabaseconnector", "settings.json"),
		filepath.Join("vendorlib", "gson-2.13.1.jar"),
	}
	if strings.Join(written, "|") != strings.Join(want, "|") {
		t.Errorf("written = %v\nwant      %v", written, want)
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(proj, rel)); err != nil {
			t.Errorf("declared file %s not installed: %v", rel, err)
		}
	}

	// What was left out is reported, not dropped silently.
	var left []string
	for _, s := range skipped {
		left = append(left, s.Path)
	}
	sort.Strings(left)
	wantLeft := []string{"License.txt", "Mendix EULA.docx",
		"SiemensMendixExternalDatabaseConnector__6.3.0__READMEOSS_2026-06-17__04-28-14.html",
		"manifest.json", "themesource/externaldatabaseconnector/undeclared-but-shipped.x"}
	if strings.Join(left, "|") != strings.Join(wantLeft, "|") {
		t.Errorf("skipped = %v\nwant      %v", left, wantLeft)
	}
}

// TestInstallPackageFiles_RealPackageWritesOnlyItsDeclaredFiles runs the same
// question against the published ExternalDatabaseConnector 6.3.0 package in
// mx-modules/, so the parsing is held to a real package.xml (two default
// namespaces, a declared entry the archive lacks) rather than to the fixture
// above. The expectation is what `mx module-import` itself wrote.
func TestInstallPackageFiles_RealPackageWritesOnlyItsDeclaredFiles(t *testing.T) {
	mpk := filepath.Join("..", "..", "..", "mx-modules", "ExternalDatabaseConnector-v6.3.0.mpk")
	if _, err := os.Stat(mpk); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	proj := t.TempDir()
	written, _, err := InstallPackageFiles(mpk, proj)
	if err != nil {
		t.Fatalf("InstallPackageFiles: %v", err)
	}

	root, err := os.ReadDir(proj)
	if err != nil {
		t.Fatal(err)
	}
	var rootFiles, rootDirs []string
	for _, e := range root {
		if e.IsDir() {
			rootDirs = append(rootDirs, e.Name())
		} else {
			rootFiles = append(rootFiles, e.Name())
		}
	}
	if len(rootFiles) != 0 {
		t.Errorf("files written into the app root: %v", rootFiles)
	}
	sort.Strings(rootDirs)
	if got := strings.Join(rootDirs, ","); got != "javasource,themesource,vendorlib" {
		t.Errorf("top-level directories = %s, want javasource,themesource,vendorlib", got)
	}
	// 176 entries declared, one of them (the snowflake driver) stripped from
	// the fixture: mx module-import wrote 175.
	if len(written) != 175 {
		t.Errorf("wrote %d files, want the 175 declared files the archive holds", len(written))
	}
}

// An empty <files/> is a declaration that the package installs no project
// files; no list at all is not, and falls back to leaving out only the root.
func TestInstallPackageFiles_EmptyListIsNotNoList(t *testing.T) {
	entries := map[string]string{
		"License.txt":                     "license",
		"themesource/probe/web/main.scss": "// probe",
	}
	withEmpty := map[string]string{"package.xml": `<package><modelerProject><module name="P"/><files/></modelerProject></package>`}
	for k, v := range entries {
		withEmpty[k] = v
	}

	written, _, err := InstallPackageFiles(buildMPK(t, withEmpty), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 0 {
		t.Errorf("an empty <files/> installed %v", written)
	}

	written, _, err = InstallPackageFiles(buildMPK(t, entries), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(written, "|") != filepath.Join("themesource", "probe", "web", "main.scss") {
		t.Errorf("with no package.xml, written = %v; want the folder entry and not the root one", written)
	}
}
