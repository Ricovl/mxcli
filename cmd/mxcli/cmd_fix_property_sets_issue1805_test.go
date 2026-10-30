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

// ako/mxcli#1805: on a Mendix 11.14.0 project mxcli wrote, `fix property-sets`
// reported
//
//	MIXED  DomainModels$NoGeneralization.HasOwnerAttr: on 81 of 84 element(s); without it: bf8b2a01-… [DomainModels$DomainModel]
//
// (with HasChangedByAttr, HasChangedDateAttr, HasCreatedDateAttr and
// DateTimeAttributeType.LocalizeDate) and wrote nothing: the elements were
// stored by an older mxcli, and the table had no measured value for the keys.
// The subject is the same gap in the Studio Pro-authored fixture (11.13.0):
// one entity's NoGeneralization and the DateTime attribute type of the same
// domain model as the older mxcli left them.

// stripNestedKeys removes keys from the first element of typ in the unit file
// f and returns how many it removed.
func stripNestedKeys(t *testing.T, f, typ string, keys ...string) int {
	t.Helper()
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	drop := map[string]bool{}
	for _, k := range keys {
		drop[k] = true
	}
	removed, done := 0, false
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case bson.D:
			isSubject := false
			for _, e := range x {
				if e.Key == "$Type" && e.Value == typ && !done {
					isSubject = true
				}
			}
			out := make(bson.D, 0, len(x))
			for _, e := range x {
				if isSubject && drop[e.Key] {
					removed++
					continue
				}
				out = append(out, bson.E{Key: e.Key, Value: walk(e.Value)})
			}
			if isSubject {
				done = true
			}
			return out
		case bson.A:
			out := make(bson.A, len(x))
			for i, e := range x {
				out[i] = walk(e)
			}
			return out
		}
		return v
	}
	nb, err := bson.Marshal(walk(doc))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, nb, 0o644); err != nil {
		t.Fatal(err)
	}
	return removed
}

func TestFixPropertySets_Issue1805FillsAnOlderEntitysFlags(t *testing.T) {
	mprPath := copyPedApp(t)
	// The pedapp domain model holding both a NoGeneralization and the
	// project's DateTime attribute.
	f := filepath.Join(filepath.Dir(mprPath), "mprcontents", "35", "2a", "352a0695-fe3e-4b42-8e50-529188236707.mxunit")
	flags := []string{"HasChangedByAttr", "HasChangedDateAttr", "HasCreatedDateAttr", "HasOwnerAttr"}
	if n := stripNestedKeys(t, f, "DomainModels$NoGeneralization", flags...); n != len(flags) {
		t.Fatalf("subject: removed %d system-attribute flag(s), want %d — the fixture changed", n, len(flags))
	}
	if n := stripNestedKeys(t, f, "DomainModels$DateTimeAttributeType", "LocalizeDate"); n != 1 {
		t.Fatalf("subject: removed %d LocalizeDate key(s), want 1 — the fixture changed", n)
	}
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
	if got := out.String(); strings.Contains(got, "MIXED") || !strings.Contains(got, "[DomainModels$DomainModel]: +5 key(s), -0 key(s)") {
		t.Fatalf("dry run: want the domain model offered for the 5 missing keys and no MIXED name:\n%s", got)
	}
	if b, _ := os.ReadFile(f); !bytes.Equal(b, edited) {
		t.Fatal("the dry run wrote the unit")
	}

	out.Reset()
	if err := fixPropertySets(mprPath, false, &out); err != nil {
		t.Fatalf("repair: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Completed 1 unit(s): 5 key(s) added, 0 stripped") {
		t.Fatalf("repair not reported:\n%s", out.String())
	}
	out.Reset()
	if err := fixPropertySets(mprPath, true, &out); err != nil {
		t.Fatalf("after the repair: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Every unit already has") {
		t.Fatalf("after the repair the project is not complete:\n%s", out.String())
	}
}
