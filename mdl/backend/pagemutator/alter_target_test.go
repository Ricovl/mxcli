// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// The page family's half of the generic ALTER (ako/mxcli#712): a page element
// is addressed by name, so the resolver answers from the same finders the
// operations use, and refuses the address forms a page has no use for.

func namedFinder(widgets map[string]bson.D) widgetFinder {
	return func(_ bson.D, name string) *bsonWidgetResult {
		if w, ok := widgets[name]; ok {
			return &bsonWidgetResult{widget: w}
		}
		return nil
	}
}

func resolverMutator() *Mutator {
	grid := buildGridWithColumns([]map[string]string{{"attr": "M.E.Merchant"}, {"caption": "Amount"}})
	scroll := bson.D{
		{Key: "$Type", Value: "Forms$ScrollContainer"},
		{Key: "Name", Value: "layoutContainer"},
		{Key: "Top", Value: bson.D{{Key: "$Type", Value: "Forms$ScrollContainerRegion"}}},
	}
	return &Mutator{rawData: bson.D{}, widgetFinder: namedFinder(map[string]bson.D{
		"btnSave":         {{Key: "$Type", Value: "Forms$ActionButton"}, {Key: "Name", Value: "btnSave"}},
		"dg":              grid,
		"layoutContainer": scroll,
	})}
}

func TestResolveAlterTarget_ByName(t *testing.T) {
	m := resolverMutator()
	cases := []struct {
		path []string
		kind string
	}{
		{[]string{"btnSave"}, "widget"},
		{[]string{"dg", "Merchant"}, "column"},
		{[]string{"layoutContainer", "top"}, "region"},
	}
	for _, c := range cases {
		got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: c.path})
		if err != nil {
			t.Errorf("%v: %v", c.path, err)
			continue
		}
		if got.Kind != c.kind || got.Name != strings.Join(c.path, ".") {
			t.Errorf("%v: got %+v", c.path, got)
		}
	}
}

func TestResolveAlterTarget_Misses(t *testing.T) {
	m := resolverMutator()
	cases := []struct {
		path []string
		want string
	}{
		{[]string{"btnMissing"}, `widget "btnMissing" not found`},
		{[]string{"dg", "Nope"}, "Nope"},
		{[]string{"layoutContainer", "middle"}, `has no region "middle"`},
		{[]string{"layoutContainer", "left"}, "has no left region"},
	}
	for _, c := range cases {
		_, err := m.ResolveAlterTarget(backend.AlterTarget{Path: c.path})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: want error containing %q, got %v", c.path, c.want, err)
		}
	}
}

// A page element has a name; content addressing and @n belong to documents
// whose elements do not. Accepting either would have to mean something, and a
// page has nothing for it to mean — so each is refused, naming the form to use.
func TestResolveAlterTarget_RefusesFormsAPageDoesNotUse(t *testing.T) {
	m := resolverMutator()
	_, err := m.ResolveAlterTarget(backend.AlterTarget{Caption: "Save"})
	if err == nil || !strings.Contains(err.Error(), "by name") {
		t.Errorf("caption: want a by-name refusal, got %v", err)
	}
	_, err = m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"btnSave"}, Ordinal: 1})
	if err == nil || !strings.Contains(err.Error(), "@1") {
		t.Errorf("ordinal: want a refusal naming @1, got %v", err)
	}
	var te *backend.AlterTargetError
	if !errors.As(err, &te) {
		t.Errorf("refusal should be an AlterTargetError, got %T", err)
	}
}
