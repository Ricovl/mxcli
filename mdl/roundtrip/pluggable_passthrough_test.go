// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#721 L4: a pluggable widget the statement did not change keeps the
// Type and Object Studio Pro stored, instead of being rebuilt from the widget's
// template. Both fixtures' widgets are Studio Pro-authored, so the stored
// property order, translations and values are not the template's.
const activeSessions = "page Administration.ActiveSessions"

// Executing the description of a page whose pluggable widgets are unchanged
// writes nothing: not the DataGrid2 of PedApp's ActiveSessions, and not the
// Image of TestApp's Feedback pop-up, whose stored Type the template replaced,
// and not the Timeline of a WorkflowCommons snippet.
func TestPluggableWidgetUnchangedIsNotRewritten(t *testing.T) {
	cases := []struct {
		fx     fixture
		target string
		header string
	}{
		{pedApp, activeSessions, ""},
		{pedApp, activeSessions, "mdl 1;"},
		{testApp, "page FeedbackModule.PopupFailure_Logo", ""},
		// Its timeline's stored lists carry the older typed-array marker 2; a
		// carry that re-set an equal flag inside the kept Object dirtied it, and
		// the re-encode wrote marker 3.
		{testApp, "snippet WorkflowCommons.Snip_DashboardContext_Timeline", ""},
	}
	for _, c := range cases {
		t.Run(c.fx.name+" "+c.target+" "+c.header, func(t *testing.T) {
			h := newFixtureHarness(t, c.fx)
			defer h.close()
			script := h.mustDescribe(t, c.target)
			if c.header != "" {
				script = c.header + "\n" + h.describeUnder(c.header, c.target)
				h.out.Reset()
			}
			if err := h.exec(script); err != nil {
				t.Fatalf("exec the description: %v\n%s", err, script)
			}
			if changed := h.orig.diff(h.snapshot()); len(changed) > 0 {
				t.Errorf("the description of an unchanged page wrote:\n  %s", strings.Join(changed, "\n  "))
			}
		})
	}
}

// An edit elsewhere on the page rewrites the page, and the data grid it did
// not touch keeps its stored Type and Object byte for byte. The control edits
// the grid itself, which must rebuild it — else the byte comparison could not
// fail.
func TestPluggableWidgetKeptWhenASiblingChanges(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	first := h.mustDescribe(t, activeSessions)
	storedType, storedObj := pluggableParts(t, h.pageUnit(t, "ActiveSessions"), "dataGrid21")

	edit := func(from, to string) string {
		if !strings.Contains(first, from) {
			t.Fatalf("the description has no %q:\n%s", from, first)
		}
		return strings.Replace(first, from, to, 1)
	}

	t.Run("sibling edited", func(t *testing.T) {
		h.restore()
		if err := h.exec(edit("Content: 'Active Sessions'", "Content: 'Live Sessions'")); err != nil {
			t.Fatal(err)
		}
		unit := h.pageUnit(t, "ActiveSessions")
		if !bytes.Contains(unit, []byte("Live Sessions")) {
			t.Fatalf("the edited caption was not written")
		}
		typ, obj := pluggableParts(t, unit, "dataGrid21")
		if !bytes.Equal(typ, storedType) || !bytes.Equal(obj, storedObj) {
			t.Errorf("the untouched data grid was rebuilt:\n  %s", strings.Join(bsonDiff(storedObj, obj), "\n  "))
		}
	})

	t.Run("control: the grid edited", func(t *testing.T) {
		h.restore()
		if err := h.exec(edit("Caption: 'User name'", "Caption: 'User'")); err != nil {
			t.Fatal(err)
		}
		_, obj := pluggableParts(t, h.pageUnit(t, "ActiveSessions"), "dataGrid21")
		if bytes.Equal(obj, storedObj) {
			t.Fatalf("an edited grid kept its stored Object — the comparison above cannot fail")
		}
		if got := h.mustDescribe(t, activeSessions); !strings.Contains(got, "Caption: 'User'") {
			t.Errorf("the edited caption was not written:\n%s", got)
		}
	})
}

// pageUnit returns the raw bytes of the page named name.
func (h *harness) pageUnit(t *testing.T, name string) []byte {
	t.Helper()
	for _, b := range h.snapshot().units {
		if typ, n := typeAndName(b); n == name && typ == "Forms$Page" {
			return b
		}
	}
	t.Fatalf("page %s not found", name)
	return nil
}

// pluggableParts returns the encoded Type and Object of the pluggable widget
// named name in a unit.
func pluggableParts(t *testing.T, unit []byte, name string) (typ, obj []byte) {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(unit, &doc); err != nil {
		t.Fatal(err)
	}
	var found bson.D
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			isCW, n := false, ""
			for _, e := range x {
				switch e.Key {
				case "$Type":
					isCW = e.Value == "CustomWidgets$CustomWidget"
				case "Name":
					n, _ = e.Value.(string)
				}
			}
			if isCW && n == name {
				found = x
				return
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	if found == nil {
		t.Fatalf("no pluggable widget %s", name)
	}
	enc := func(key string) []byte {
		for _, e := range found {
			if e.Key == key {
				b, err := bson.Marshal(e.Value)
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
		}
		t.Fatalf("widget %s has no %s", name, key)
		return nil
	}
	return enc("Type"), enc("Object")
}
