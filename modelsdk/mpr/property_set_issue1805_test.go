// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1805: on a Mendix 11.14.0 project mxcli wrote, `fix property-sets`
// reported
//
//	MIXED  DomainModels$DateTimeAttributeType.LocalizeDate: on 15 of 16 element(s); without it: bf8b2a01-… [DomainModels$DomainModel]
//	MIXED  DomainModels$NoGeneralization.HasChangedByAttr:   on 81 of 84 element(s); without it: bf8b2a01-… [DomainModels$DomainModel]
//	(and HasChangedDateAttr, HasCreatedDateAttr, HasOwnerAttr)
//
// No writer of today leaves those keys out — create, create or modify, alter,
// rename, move and the association writes all emit them (measured: a script
// through every one gives no such MIXED line on 10.24.24, 11.13.0 or 11.15.0).
// The elements without them were written by an older mxcli, which wrote only
// the system-attribute flags that were true and no LocalizeDate, and they
// survive every later write of their domain model: a write rebuilds the entity
// it names and passes its siblings through as their stored bytes. The table had
// no row for the keys, because no measurement of today's writers saw one
// missing, so neither a rewrite nor the repair filled them.

const (
	issue1805DMID    = "18051805-0000-4000-8000-000000000001"
	issue1805NewID   = "18051805-0000-4000-8000-000000000002"
	issue1805OlderID = "18051805-0000-4000-8000-000000000003"
)

// issue1805DomainModel is a domain model holding an entity as today's writer
// writes it and one as an older mxcli wrote it: a NoGeneralization with only
// Persistable, and a DateTime attribute type without LocalizeDate.
func issue1805DomainModel(t *testing.T, newDoc string) []byte {
	t.Helper()
	bin := func(id string) bson.Binary { return bson.Binary{Subtype: 0, Data: uuidToBlob(id)} }
	sub := func(id string, n byte) bson.Binary {
		b := uuidToBlob(id)
		b[14] = n
		return bson.Binary{Subtype: 0, Data: b}
	}
	entity := func(id, name, doc string, gen bson.D, dt bson.D) bson.D {
		return bson.D{
			{Key: "$ID", Value: bin(id)},
			{Key: "$Type", Value: "DomainModels$EntityImpl"},
			{Key: "Documentation", Value: doc},
			{Key: "Image", Value: ""},
			{Key: "ImageData", Value: bson.Binary{Subtype: 0, Data: []byte{}}},
			{Key: "MaybeGeneralization", Value: gen},
			{Key: "Name", Value: name},
			{Key: "Source", Value: nil},
			{Key: "Attributes", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: sub(id, 0xa0)},
				{Key: "$Type", Value: "DomainModels$Attribute"},
				{Key: "Name", Value: "At"},
				{Key: "NewType", Value: dt},
			}}},
		}
	}
	current := entity(issue1805NewID, "Current", newDoc,
		bson.D{
			{Key: "$ID", Value: sub(issue1805NewID, 0xb0)},
			{Key: "$Type", Value: "DomainModels$NoGeneralization"},
			{Key: "HasChangedByAttr", Value: false},
			{Key: "HasChangedDateAttr", Value: false},
			{Key: "HasCreatedDateAttr", Value: false},
			{Key: "HasOwnerAttr", Value: false},
			{Key: "Persistable", Value: true},
		},
		bson.D{
			{Key: "$ID", Value: sub(issue1805NewID, 0xc0)},
			{Key: "$Type", Value: "DomainModels$DateTimeAttributeType"},
			{Key: "LocalizeDate", Value: true},
		})
	older := entity(issue1805OlderID, "Older", "",
		bson.D{
			{Key: "$ID", Value: sub(issue1805OlderID, 0xb0)},
			{Key: "$Type", Value: "DomainModels$NoGeneralization"},
			{Key: "Persistable", Value: true},
		},
		bson.D{
			{Key: "$ID", Value: sub(issue1805OlderID, 0xc0)},
			{Key: "$Type", Value: "DomainModels$DateTimeAttributeType"},
		})
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bin(issue1805DMID)},
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "Entities", Value: bson.A{int32(3), current, older}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var issue1805Keys = map[string][]string{
	"DomainModels$NoGeneralization":      {"HasChangedByAttr", "HasChangedDateAttr", "HasCreatedDateAttr", "HasOwnerAttr"},
	"DomainModels$DateTimeAttributeType": {"LocalizeDate"},
}

// olderEntityKeys returns, per $Type, the keys the Older entity's
// generalization and attribute type carry in raw.
func olderEntityKeys(t *testing.T, raw []byte) map[string]map[string]any {
	t.Helper()
	ents, err := bson.Raw(raw).Lookup("Entities").Array().Values()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents[1:] {
		d := e.Document()
		if d.Lookup("Name").StringValue() != "Older" {
			continue
		}
		out := map[string]map[string]any{}
		collect := func(sub bson.Raw) {
			var m bson.D
			if err := bson.Unmarshal(sub, &m); err != nil {
				t.Fatal(err)
			}
			typ := ""
			vals := map[string]any{}
			for _, x := range m {
				if x.Key == "$Type" {
					typ, _ = x.Value.(string)
				}
				vals[x.Key] = x.Value
			}
			out[typ] = vals
		}
		collect(d.Lookup("MaybeGeneralization").Document())
		attrs, _ := d.Lookup("Attributes").Array().Values()
		collect(attrs[1].Document().Lookup("NewType").Document())
		return out
	}
	t.Fatal("no Older entity")
	return nil
}

func TestIssue1805_CensusFindsNoMixedNameOnAnOlderEntity(t *testing.T) {
	stored := issue1805DomainModel(t, "")
	w, _ := newV2WriterForCommitTest(t, issue1805DMID, stored)
	w.reader.projectVersion = pv(11, 14, 0)

	rep, err := w.reader.StoredPropertySetGaps(issue1805DMID)
	if err != nil {
		t.Fatal(err)
	}
	c := NewPropertySetCensus()
	c.Count(rep)
	for _, m := range c.Mixed() {
		t.Errorf("MIXED  %s.%s: on %d of %d element(s)", m.Type, m.Key, m.With, m.Elements)
	}
	if rep.Added != 5 {
		t.Errorf("the repair would add %d key(s), want the 5 the older entity lacks", rep.Added)
	}
}

// Any write of the domain model — here one that changes only the CURRENT
// entity, as `alter entity Current set documentation` does, with the older one
// passed through as its stored bytes — completes the older entity on the way.
func TestIssue1805_AnyRewriteOfTheDomainModelCompletesAnOlderEntity(t *testing.T) {
	w, path := newV2WriterForCommitTest(t, issue1805DMID, issue1805DomainModel(t, ""))
	w.reader.projectVersion = pv(11, 14, 0)

	if err := w.UpdateRawUnit(issue1805DMID, issue1805DomainModel(t, "changed")); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := olderEntityKeys(t, after)
	for typ, keys := range issue1805Keys {
		for _, k := range keys {
			if _, ok := got[typ][k]; !ok {
				t.Errorf("after a rewrite of its domain model the older entity's %s still has no %s", typ, k)
			}
		}
	}
	// The values are Mendix's for the absent key, so the model says what it
	// said before: no system member appears, the date stays localized.
	for _, k := range issue1805Keys["DomainModels$NoGeneralization"] {
		if v, ok := got["DomainModels$NoGeneralization"][k]; ok && v != false {
			t.Errorf("%s = %#v, want false", k, v)
		}
	}
	if v, ok := got["DomainModels$DateTimeAttributeType"]["LocalizeDate"]; ok && v != true {
		t.Errorf("LocalizeDate = %#v, want true", v)
	}
}
