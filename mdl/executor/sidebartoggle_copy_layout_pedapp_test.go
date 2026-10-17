// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1220 (ako/mxcli#1285), the reported workflow end to end on
// the Studio Pro-authored PedApp: describe Atlas_Core.Atlas_Default, execute it
// under a new name, and the copy carries the sidebar toggle Atlas stores —
// with the same keys, so the copy is what Studio Pro would have written.
func TestCopyAtlasDefaultKeepsTheSidebarToggle(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	out.Reset()
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeLayout, Name: ast.QualifiedName{Module: "Atlas_Core", Name: "Atlas_Default"}}); err != nil {
		t.Fatalf("describe layout: %v", err)
	}
	described := languageHeader.ReplaceAllString(out.String(), "")
	if strings.Contains(described, "NOT re-executable") {
		t.Fatalf("describe still drops a widget:\n%s", described)
	}
	if !strings.Contains(described, "sidebartoggle sidebarToggle3") {
		t.Fatalf("describe does not print the sidebar toggle:\n%s", described)
	}

	copied := strings.Replace(described, "create or modify layout Atlas_Core.Atlas_Default", "create layout MyFirstModule.App_Default", 1)
	if got := agreeCheck(t, exec, dir, "mdl 1;\n"+copied); len(got) != 0 {
		t.Fatalf("check refuses the copy:\n%s", strings.Join(got, "\n"))
	}
	if err := agreeExec(t, exec, "mdl 1;\n"+copied); err != nil {
		t.Fatalf("exec of the copy: %v\n%s", err, copied)
	}

	original := sidebarToggles(t, exec, "Atlas_Core.Atlas_Default")
	copy := sidebarToggles(t, exec, "MyFirstModule.App_Default")
	if len(original) == 0 {
		t.Fatal("Atlas_Default has no Forms$SidebarToggleButton in the fixture; the test proves nothing")
	}
	if len(copy) != len(original) {
		t.Fatalf("the copy has %d sidebar toggles, Atlas_Default %d", len(copy), len(original))
	}
	for i := range original {
		if got, want := rawKeysOf(copy[i]), rawKeysOf(original[i]); got != want {
			t.Errorf("toggle %d keys:\n copy:     %s\n Studio Pro: %s", i, got, want)
		}
		for _, k := range []string{"Name", "ButtonStyle", "RenderType"} {
			if a, b := copy[i].Lookup(k).String(), original[i].Lookup(k).String(); a != b {
				t.Errorf("toggle %d %s = %s, Studio Pro stores %s", i, k, a, b)
			}
		}
		if a, b := copy[i].Lookup("Icon", "Image").String(), original[i].Lookup("Icon", "Image").String(); a != b {
			t.Errorf("toggle %d icon = %s, Studio Pro stores %s", i, a, b)
		}
	}

	// The toggle opens a region whose ToggleMode says how. Without it mxbuild
	// reports CE0628 "For the sidebar toggle button to work, the outermost
	// widget on the layout must be a scroll container with a region that can
	// be toggled" — measured on 11.13.0 against this copy.
	origModes, copyModes := storedRegionToggleModes(t, exec, "Atlas_Core.Atlas_Default"), storedRegionToggleModes(t, exec, "MyFirstModule.App_Default")
	if origModes["Left"] != "ShrinkContentInitiallyClosed" {
		t.Fatalf("fixture's Left region ToggleMode = %q; the test proves nothing", origModes["Left"])
	}
	for slot, want := range origModes {
		if copyModes[slot] != want {
			t.Errorf("region %s ToggleMode = %q in the copy, Studio Pro stores %q", slot, copyModes[slot], want)
		}
	}

	// And the copy describes as the original does, name aside.
	out.Reset()
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeLayout, Name: ast.QualifiedName{Module: "MyFirstModule", Name: "App_Default"}}); err != nil {
		t.Fatalf("describe copy: %v", err)
	}
	again := strings.Replace(languageHeader.ReplaceAllString(out.String(), ""), "create or modify layout MyFirstModule.App_Default", "create layout MyFirstModule.App_Default", 1)
	if again != copied {
		t.Errorf("the copy describes differently:\n--- first\n%s\n--- copy\n%s", copied, again)
	}
}

// sidebarToggles returns the layout's Forms$SidebarToggleButton documents.
func sidebarToggles(t *testing.T, exec *Executor, qn string) []bson.Raw {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	u, err := ctx.Backend.GetRawUnitByName("layout", qn)
	if err != nil || u == nil {
		t.Fatalf("raw layout %s: %v", qn, err)
	}
	var found []bson.Raw
	var walk func(v bson.RawValue)
	walk = func(v bson.RawValue) {
		switch v.Type {
		case bson.TypeEmbeddedDocument:
			doc := v.Document()
			if doc.Lookup("$Type").StringValue() == "Forms$SidebarToggleButton" {
				found = append(found, doc)
			}
			elems, _ := doc.Elements()
			for _, e := range elems {
				walk(e.Value())
			}
		case bson.TypeArray:
			vals, _ := v.Array().Values()
			for _, x := range vals {
				walk(x)
			}
		}
	}
	walk(bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: u.Contents})
	return found
}

func rawKeysOf(d bson.Raw) string {
	elems, _ := d.Elements()
	var ks []string
	for _, e := range elems {
		ks = append(ks, e.Key())
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// storedRegionToggleModes is each scroll container region's ToggleMode, by BSON key.
func storedRegionToggleModes(t *testing.T, exec *Executor, qn string) map[string]string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	u, err := ctx.Backend.GetRawUnitByName("layout", qn)
	if err != nil || u == nil {
		t.Fatalf("raw layout %s: %v", qn, err)
	}
	modes := map[string]string{}
	var walk func(v bson.RawValue)
	walk = func(v bson.RawValue) {
		switch v.Type {
		case bson.TypeEmbeddedDocument:
			doc := v.Document()
			elems, _ := doc.Elements()
			for _, e := range elems {
				if r, ok := e.Value().DocumentOK(); ok && r.Lookup("$Type").StringValue() == "Forms$ScrollContainerRegion" {
					modes[e.Key()] = r.Lookup("ToggleMode").StringValue()
				}
				walk(e.Value())
			}
		case bson.TypeArray:
			vals, _ := v.Array().Values()
			for _, x := range vals {
				walk(x)
			}
		}
	}
	walk(bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: u.Contents})
	return modes
}
