// SPDX-License-Identifier: Apache-2.0

package bsoncompat_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v1Import is the import path of the deprecated v1 BSON driver.
//
// mxcli ran both driver lines side by side until the v1 line stopped getting
// security fixes: CVE-2026-107325 has no v1 release that carries a fix, and
// v1.17.10 is the last v1 release there will be. A customer's release pipeline
// blocks at CVSS >= 7, so a single v1 import anywhere re-adds the module to
// go.mod and blocks their release again — with no upgrade available to clear
// it. Reach for modelsdk/bsoncompat instead when v2 decodes a shape
// differently.
const v1Import = `"go.mongodb.org/mongo-driver/bson`

// TestNoV1DriverImports fails if any Go file imports the v1 BSON driver.
//
// This is a guard, not a style rule: re-adding the import is not a build error
// and not a test failure anywhere else, so nothing but this test would notice
// until the next dependency scan.
func TestNoV1DriverImports(t *testing.T) {
	root := filepath.Join("..", "..")

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "reference", "bin", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue // a comment may legitimately name the old path
			}
			if strings.Contains(trimmed, v1Import+`"`) ||
				strings.Contains(trimmed, v1Import+`/primitive"`) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, rel)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("the deprecated v1 BSON driver is imported again by %d file(s):\n  %s\n\n"+
			"CVE-2026-107325 has no fix on the v1 line and never will. Use "+
			"go.mongodb.org/mongo-driver/v2/bson, and modelsdk/bsoncompat where "+
			"v2 decodes a nested document differently.",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestGoModDoesNotRequireV1Driver fails if go.mod requires the v1 driver,
// which is what a dependency scanner reads.
func TestGoModDoesNotRequireV1Driver(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 2 && f[0] == "go.mongodb.org/mongo-driver" {
			t.Errorf("go.mod requires the v1 driver again: %q", strings.TrimSpace(line))
		}
	}
}
