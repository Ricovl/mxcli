// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1097: `mxcli fix property-sets` on a real, Studio Pro-authored
// project (testdata/pedapp, 11.13.0).

// dropKeyFromUnit removes key from the top level of the first unit of the given
// $Type and Name, as a unit written by an older mxcli would lack it. Returns the
// file and its new bytes.
func dropKeyFromUnit(t *testing.T, mprPath, typ, name, key string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(mprPath), "mprcontents", "*", "*", "*.mxunit"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		raw := bson.Raw(b)
		if ty, _ := raw.Lookup("$Type").StringValueOK(); ty != typ {
			continue
		}
		if n, _ := raw.Lookup("Name").StringValueOK(); n != name {
			continue
		}
		var d bson.D
		if err := bson.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		out := d[:0]
		found := false
		for _, e := range d {
			if e.Key == key {
				found = true
				continue
			}
			out = append(out, e)
		}
		if !found {
			t.Fatalf("subject: %s %s has no %s", typ, name, key)
		}
		nb, err := bson.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, nb, 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	t.Fatalf("subject: no %s %s in the fixture", typ, name)
	return ""
}

// The control: a project Studio Pro wrote has nothing to repair. A table row
// that added a key Studio Pro does not write would show up here.
func TestFixPropertySets_StudioProProjectIsClean(t *testing.T) {
	mprPath := copyPedApp(t)
	var out bytes.Buffer
	if err := fixPropertySets(mprPath, true, &out); err != nil {
		t.Fatalf("dry run on an untouched Studio Pro project: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Every unit already has") {
		t.Fatalf("control not reported clean:\n%s", out.String())
	}
}

func TestFixPropertySets_CompletesAUnitNothingRewrites(t *testing.T) {
	mprPath := copyPedApp(t)
	f := dropKeyFromUnit(t, mprPath, "ImportMappings$ImportMapping", "IMM_PostResponse", "MessageDefinition2")
	// The stored file now differs from its ContentsHash; rewrite the index so
	// the subject is "a unit an older mxcli wrote", not "a reverted file".
	var sink bytes.Buffer
	if err := fixHashes(mprPath, true, &sink); err != nil {
		t.Fatalf("re-index the subject: %v\n%s", err, sink.String())
	}
	edited, _ := os.ReadFile(f)

	var out bytes.Buffer
	err := fixPropertySets(mprPath, true, &out)
	if !errors.Is(err, errPropertySetsRemain) {
		t.Fatalf("dry run: want errPropertySetsRemain, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "IMM_PostResponse]: +1 key(s), -0 key(s)") || !strings.Contains(out.String(), "Would complete 1 unit(s)") {
		t.Fatalf("dry run did not report the gap:\n%s", out.String())
	}
	if b, _ := os.ReadFile(f); !bytes.Equal(b, edited) {
		t.Fatal("the dry run wrote the unit")
	}

	out.Reset()
	if err := fixPropertySets(mprPath, false, &out); err != nil {
		t.Fatalf("repair: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Completed 1 unit(s): 1 key(s) added, 0 stripped") {
		t.Fatalf("repair not reported:\n%s", out.String())
	}
	b, _ := os.ReadFile(f)
	if v, err := bson.Raw(b).LookupErr("MessageDefinition2"); err != nil || v.StringValue() != "" {
		t.Fatalf("MessageDefinition2 after repair: %v %v", v, err)
	}

	// A second run writes nothing, and the index still describes the files.
	out.Reset()
	if err := fixPropertySets(mprPath, false, &out); err != nil {
		t.Fatalf("second run: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Every unit already has") {
		t.Fatalf("second run was not a no-op:\n%s", out.String())
	}
	if again, _ := os.ReadFile(f); !bytes.Equal(again, b) {
		t.Fatal("the second run rewrote the unit")
	}
	if w := contentsHashDriftWarning(mprPath); w != "" {
		t.Fatalf("the repair left the ContentsHash index stale: %s", w)
	}
}
