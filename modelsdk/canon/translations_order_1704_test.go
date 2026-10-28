// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1704: describe → exec rewrote JTSBootLogboek.Home_Native although
// nothing was lost — "the order … of some English/Dutch texts changed". The
// rebuild writes the one language MDL carries and the carry appended the stored
// others in sorted order, so a text stored `[nl_NL, en_US]` came back `[en_US,
// nl_NL]`. The canonical form compares lists in order, so the write went ahead.
//
// Studio Pro's order is not sorted: across PedApp and TestApp, 129 texts are
// stored nl_NL first, and orders such as en_US,fr_FR,nl_NL,de_DE occur.

// languageOrder returns the LanguageCode order of the first Texts$Text found.
func languageOrder(t *testing.T, raw []byte) string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var out []string
	var walk func(v any) bool
	walk = func(v any) bool {
		switch n := v.(type) {
		case bson.D:
			if docLookup(n, "$Type") == "Texts$Text" {
				items, _ := docLookup(n, "Items").(bson.A)
				for _, it := range items {
					if d, ok := it.(bson.D); ok {
						l, _ := docLookup(d, "LanguageCode").(string)
						out = append(out, l)
					}
				}
				return true
			}
			for _, e := range n {
				if walk(e.Value) {
					return true
				}
			}
		case bson.A:
			for _, e := range n {
				if walk(e) {
					return true
				}
			}
		}
		return false
	}
	walk(doc)
	return strings.Join(out, ",")
}

func TestCarryTranslations_KeepsTheStoredLanguageOrder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored []bson.D
	}{
		// The reported shape: the rebuilt language is not the stored first.
		{"Dutch first", []bson.D{tr(11, "nl_NL", "Mijn account"), tr(12, "en_US", "My Account")}},
		// The carried languages are not in sorted order (measured in TestApp).
		{"unsorted", []bson.D{
			tr(11, "en_US", "My Account"), tr(12, "fr_FR", "Mon compte"),
			tr(13, "nl_NL", "Mijn account"), tr(14, "de_DE", "Mein Konto"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := textDoc(t, 10, "MyAccount", tc.stored...)
			rebuilt := textDoc(t, 20, "MyAccount", tr(21, "en_US", "My Account"))

			if got, want := languageOrder(t, CarryTranslations(rebuilt, stored)), languageOrder(t, stored); got != want {
				t.Errorf("carried order %s, want the stored %s", got, want)
			}
			if _, unchanged := Reconcile(rebuilt, stored); !unchanged {
				t.Errorf("a rebuild that only dropped languages was written: the carry reordered them")
			}
		})
	}
}

// The same when the text is paired by its source string: a text in a list under
// an unnamed element, in a rebuild whose texts sit at other paths.
func TestCarryTranslations_KeepsTheStoredLanguageOrderBySource(t *testing.T) {
	caption := func(id byte, items ...bson.D) bson.D {
		arr := bson.A{int32(3)}
		for _, it := range items {
			arr = append(arr, it)
		}
		return bson.D{
			{Key: "$Type", Value: "Forms$Unnamed"},
			{Key: "$ID", Value: bin(id)},
			{Key: "Caption", Value: bson.D{
				{Key: "$Type", Value: "Texts$Text"},
				{Key: "$ID", Value: bin(id + 1)},
				{Key: "Items", Value: arr},
			}},
		}
	}
	page := func(widgets ...bson.D) []byte {
		arr := bson.A{int32(2)}
		for _, w := range widgets {
			arr = append(arr, w)
		}
		return marshal(t, bson.D{
			{Key: "$Type", Value: "Forms$Form"},
			{Key: "$ID", Value: bin(1)},
			{Key: "Name", Value: "Home"},
			{Key: "Widgets", Value: arr},
		})
	}
	stored := page(caption(10, tr(12, "nl_NL", "Welkom"), tr(13, "en_US", "Welcome")))
	rebuilt := page(
		caption(20, tr(22, "en_US", "Welcome")),
		caption(30, tr(32, "en_US", "Added")),
	)
	if got := languageOrder(t, CarryTranslations(rebuilt, stored)); got != "nl_NL,en_US" {
		t.Errorf("carried order %s, want the stored nl_NL,en_US", got)
	}
}
