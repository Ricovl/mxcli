// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

// pageWriterShapes collects the three shapes a describe → exec of a Studio
// Pro-built page used to rewrite (ako/mxcli-ledger FINDINGS #170), in document
// order:
//
//   - the key set of every Forms$StaticOrDynamicString (a static open-link
//     address lost its `AttributeRef: null`);
//   - every Forms$LayoutGridColumn's PreviewWidth (a stored 1 became -1);
//   - the typed-array marker of every `Widgets` and `TabPages` list, tagged with
//     its owner and first child (a column opening with a Forms$TabControl was
//     written with 3 where Studio Pro stores 2).
//
// The pages carry other, unrelated losses (they are on the GetPut allowlist),
// so the assertion is on these shapes rather than on the unit bytes.
func pageWriterShapes(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}
	var out []string
	var walk func(d bson.D)
	walk = func(d bson.D) {
		typ := ""
		for _, e := range d {
			if e.Key == "$Type" {
				typ, _ = e.Value.(string)
			}
		}
		switch typ {
		case "Forms$StaticOrDynamicString":
			var keys []string
			for _, e := range d {
				v := "set"
				if e.Value == nil {
					v = "null"
				}
				keys = append(keys, e.Key+"="+v)
			}
			sort.Strings(keys)
			out = append(out, "address "+strings.Join(keys, ","))
		case "Forms$LayoutGridColumn":
			for _, e := range d {
				if e.Key == "PreviewWidth" {
					out = append(out, fmt.Sprintf("column PreviewWidth=%v", e.Value))
				}
			}
		}
		for _, e := range d {
			switch v := e.Value.(type) {
			case bson.D:
				walk(v)
			case bson.A:
				if (e.Key == "Widgets" || e.Key == "TabPages") && len(v) > 0 {
					first := "<empty>"
					if len(v) > 1 {
						if c, ok := v[1].(bson.D); ok {
							for _, ce := range c {
								if ce.Key == "$Type" {
									first, _ = ce.Value.(string)
								}
							}
						}
					}
					out = append(out, fmt.Sprintf("%s.%s first=%s marker=%v", typ, e.Key, first, v[0]))
				}
				for _, x := range v {
					if c, ok := x.(bson.D); ok {
						walk(c)
					}
				}
			}
		}
	}
	walk(doc)
	return out
}

// unitByLabel finds a unit of the committed fixture by its "$Type Name" label.
func (h *harness) unitByLabel(label string) string {
	h.t.Helper()
	var found string
	for id, l := range h.orig.label {
		if l == label {
			if found != "" {
				h.t.Fatalf("two units are labelled %q", label)
			}
			found = id
		}
	}
	if found == "" {
		h.t.Fatalf("fixture has no unit labelled %q", label)
	}
	return found
}

// shapeDrift executes target's describe output and returns how the three
// shapes of unit id differ from the fixture, or nil when they do not.
func (h *harness) shapeDrift(target, id string) []string {
	h.t.Helper()
	defer h.restore()
	described, err := h.describe(target)
	if err != nil {
		h.t.Fatalf("describe %s: %v", target, err)
	}
	if err := h.exec(described); err != nil {
		h.t.Fatalf("exec describe output of %s: %v", target, err)
	}
	after, ok := h.snapshot().units[id]
	if !ok {
		h.t.Fatalf("%s disappeared", target)
	}
	before := pageWriterShapes(h.t, h.orig.units[id])
	got := pageWriterShapes(h.t, after)
	if reflect.DeepEqual(before, got) {
		return nil
	}
	var drift []string
	n := len(before)
	if len(got) > n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		var b, g string
		if i < len(before) {
			b = before[i]
		}
		if i < len(got) {
			g = got[i]
		}
		if b != g {
			drift = append(drift, fmt.Sprintf("stored %q, written %q", b, g))
		}
	}
	return drift
}

// The pages are PedApp's own: MyFirstModule.Home_Web holds a static open-link
// button and the one column in the fixture whose PreviewWidth is not -1, and
// Administration.Account_Overview a layout grid column whose first widget is a
// tab control. Both are Studio Pro-authored and unmodified.
var pageWriterCarryTargets = []struct{ target, label string }{
	{"page MyFirstModule.Home_Web", "Forms$Page Home_Web"},
	{"page Administration.Account_Overview", "Forms$Page Account_Overview"},
}

func TestPageWriter_DescribeExecKeepsStudioProShapes(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	// Precondition: the fixture holds every shape, or the assertion is empty.
	var all []string
	for _, p := range pageWriterCarryTargets {
		all = append(all, pageWriterShapes(t, h.orig.units[h.unitByLabel(p.label)])...)
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{
		"address $ID=set,$Type=set,AttributeRef=null,IsDynamic=set,Value=set",
		"column PreviewWidth=1",
		"Forms$LayoutGridColumn.Widgets first=Forms$TabControl marker=2",
		"Forms$TabControl.TabPages first=Forms$TabPage marker=3",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the fixture no longer holds %q — the test would assert nothing about it", want)
		}
	}

	for _, p := range pageWriterCarryTargets {
		if drift := h.shapeDrift(p.target, h.unitByLabel(p.label)); drift != nil {
			t.Errorf("%s: describe → exec rewrote Studio Pro's stored shapes:\n  %s", p.target, strings.Join(drift, "\n  "))
		}
	}
}

// CONTROL: the same comparison detects the drift when a fix is stubbed out.
// Without it the test above would pass against a comparison that never saw a
// write. Each stub reinstates the pre-fix encoder behaviour through the codec
// registry and is undone before the next.
func TestPageWriter_DescribeExecShapes_DetectsStubbedFix(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	t.Run("static address without the AttributeRef default", func(t *testing.T) {
		codec.RegisterTypeDefaults("Forms$StaticOrDynamicString", codec.TypeDefaults{})
		defer codec.RegisterTypeDefaults("Forms$StaticOrDynamicString", codec.TypeDefaults{NullFields: []string{"AttributeRef"}})
		drift := h.shapeDrift("page MyFirstModule.Home_Web", h.unitByLabel("Forms$Page Home_Web"))
		if !strings.Contains(strings.Join(drift, "\n"), "AttributeRef=null") {
			t.Errorf("stubbing the AttributeRef default went undetected; drift = %v", drift)
		}
	})

	t.Run("Widgets marker taken from the first child", func(t *testing.T) {
		// 3 is what the child-type fallback gave a Forms$TabControl first child.
		codec.RegisterPropertyListMarker("Forms$LayoutGridColumn", "Widgets", 3)
		defer codec.RegisterPropertyListMarker("Forms$LayoutGridColumn", "Widgets", 2)
		drift := h.shapeDrift("page Administration.Account_Overview", h.unitByLabel("Forms$Page Account_Overview"))
		if !strings.Contains(strings.Join(drift, "\n"), "first=Forms$TabControl marker=3") {
			t.Errorf("stubbing the Widgets property marker went undetected; drift = %v", drift)
		}
	})
}
