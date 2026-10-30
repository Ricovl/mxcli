// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// readerListings are the passes allowed to list documents through b.reader,
// which holds every unit, rather than b.lister() or the cached accessors,
// which in an incremental build hold only the units it rebuilds. A listing
// through the reader decodes or reads the whole project on every refresh
// (ako/mxcli#1804), so each one here says why it has to.
var readerListings = map[string]string{
	"builder.go ListModules":                     "the hierarchy: every document's module name",
	"builder.go ListUnits":                       "the hierarchy: every document's container (no contents)",
	"builder.go ListFolders":                     "the hierarchy: every document's folder path",
	"builder_incremental.go ListUnits":           "every unit's basis, to find what changed (no contents)",
	"builder_modules.go ListModules":             "the module rows, rebuilt every time",
	"builder_modules.go ListRawUnitsByType":      "the domain model documentation of every module row, read raw, not decoded",
	"builder_external.go ListRawUnitsByType":     "finds the flows that call an external action by their raw bytes, then decodes only those",
	"builder_source_reuse.go ListUnits":          "every unit's source basis (no contents)",
	"builder_source_reuse.go ListRawUnitsByType": "every entity's source basis, hashed from the raw domain models",
}

var readerListing = regexp.MustCompile(`b\.reader\.(List\w+)\(`)

func TestBuilderListsDocumentsThroughTheScope(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	var stray []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range readerListing.FindAllStringSubmatch(string(src), -1) {
			key := f + " " + m[1]
			found[key] = true
			if _, ok := readerListings[key]; !ok {
				stray = append(stray, key)
			}
		}
	}
	sort.Strings(stray)
	if len(stray) > 0 {
		t.Errorf("passes list documents through b.reader, which decodes the whole project on every incremental refresh:\n  %s\n"+
			"List through b.lister() or a cached accessor, and look other documents up in the catalog tables; "+
			"if the pass really needs every document, add it to readerListings with the reason.",
			strings.Join(stray, "\n  "))
	}
	if len(found) == 0 {
		t.Fatal("control: no listing found at all; the pattern no longer matches the code")
	}
	for key := range readerListings {
		if !found[key] {
			t.Errorf("readerListings names %q, which no longer lists through the reader: remove it", key)
		}
	}
}
