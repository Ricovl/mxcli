// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1678: "the design properties stored on the page itself (`Forms$Page`
// `Appearance.DesignProperties`)" — `'Spacing top': 'Inner large'` on
// Login_Native, `'Background color': 'Background Primary'` on Home_Native — were
// dropped: "`describe page` prints none of them in the page header, and exec
// writes an empty list."
package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The groups a page's own design properties come from, cut down from Atlas Core
// 4.1.3's native design-properties.json: "Document" holds the reported 'Spacing
// top' and 'Background color', "Page" a toggle only pages take. "Widget" has a
// 'Spacing top' too, with other options — a page does not take it.
const pageDesignPropsTheme = `{
	"Document": [
		{"name": "Spacing top", "type": "Dropdown", "options": [{"name": "Inner large", "class": "spacingInnerTopLarge"}]},
		{"name": "Background color", "type": "Dropdown", "options": [{"name": "Background Primary", "class": "backgroundPrimary"}]}],
	"Page": [{"name": "Hide header bottom border", "type": "Toggle", "class": "pageHeaderBorderNone"}],
	"Widget": [{"name": "Spacing top", "type": "Dropdown", "options": [{"name": "Small", "class": "spacingOuterTopSmall"}]}]}`

func storedPageDesignProperties(t *testing.T, exec *Executor, page string) map[string]string {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListPages()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.Name != page || h.GetModuleName(h.FindModuleID(p.ContainerID)) != "MyFirstModule" {
			continue
		}
		raw, err := ctx.Backend.GetRawUnitBytes(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		app, _ := afGet(doc, "Appearance").(bson.D)
		for _, el := range afList(afGet(app, "DesignProperties")) {
			dp := el.(bson.D)
			v, _ := afGet(dp, "Value").(bson.D)
			out[afGet(dp, "Key").(string)] = afGet(v, "$Type").(string)
		}
		return out
	}
	t.Fatalf("page MyFirstModule.%s not found", page)
	return nil
}

func TestPageDesignProperties_RoundTrip(t *testing.T) {
	exec, out := openPedAppFixture(t)
	dir := filepath.Join(filepath.Dir(exec.newExecContext(context.Background()).MprPath), "themesource", "myfirstmodule", "native")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "design-properties.json"), []byte(pageDesignPropsTheme), 0o644); err != nil {
		t.Fatal(err)
	}

	src := "create page MyFirstModule.Home_Native (Title: 'Home', Layout: Atlas_Core.NativePhone_Default,\n" +
		"  DesignProperties: ('Background color': 'Background Primary', 'Hide header bottom border': on)) {\n" +
		"  dynamictext text4 (Content: 'JTS boat log')\n};"
	if err := afRun(t, exec, src); err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := storedPageDesignProperties(t, exec, "Home_Native")
	if got["Background color"] != "Forms$OptionDesignPropertyValue" || got["Hide header bottom border"] != "Forms$ToggleDesignPropertyValue" {
		t.Fatalf("stored page design properties = %v, want an option and a toggle", got)
	}

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Home_Native;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	header := described[:strings.Index(described, "{")]
	if !strings.Contains(strings.ToLower(header), "designproperties: ('background color': 'background primary', 'hide header bottom border': on)") {
		t.Fatalf("describe page header does not carry the page's design properties:\n%s", described)
	}

	// GetPut: the description, executed unchanged, writes nothing.
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Unchanged page")) {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}
	if got := storedPageDesignProperties(t, exec, "Home_Native"); len(got) != 2 {
		t.Errorf("after the round trip the page has %v", got)
	}
}

// check -p reads the page's own design properties against the theme's "Page"
// group, as it reads a widget's against its type's.
func TestValidateDesignProperties_PageHeader(t *testing.T) {
	dir := t.TempDir()
	themeDir := filepath.Join(dir, "themesource", "atlas_core", "native")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themeDir, "design-properties.json"), []byte(pageDesignPropsTheme), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		props, want string
	}{
		{"'Background color': 'Background Primary'", ""}, // control: declared key and value
		{"'Spacing top': 'Inner large'", ""},             // Document's options, not Widget's
		{"'Spacing top': 'Small'", "MDL-WIDGET12"},       // Widget's option: CE6085 in mxbuild
		{"'Backgroundcolor': 'Background Primary'", "MDL-WIDGET11"},
		{"'Background color': 'Background primary'", "MDL-WIDGET12"},
	} {
		prog, errs := visitor.Build("create layout M.L (LayoutType: Default) {\n  placeholder Main\n};\n" +
			"create page M.P (Title: 'P', Layout: M.L, DesignProperties: (" + tc.props + ")) {\n  dynamictext t (Content: 'x')\n};")
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		var rules []string
		for _, v := range ValidateDesignProperties(prog, filepath.Join(dir, "App.mpr"), nil) {
			rules = append(rules, v.RuleID)
		}
		if got := strings.Join(rules, ","); got != tc.want {
			t.Errorf("%s: rules = %q, want %q", tc.props, got, tc.want)
		}
	}
}

// `alter page … set ('Key': value)` with no widget writes the page's own
// design properties. It was refused as "unsupported page-level property".
func TestPageDesignProperties_AlterPageLevelSet(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	dir := filepath.Join(filepath.Dir(exec.newExecContext(context.Background()).MprPath), "themesource", "myfirstmodule", "native")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "design-properties.json"), []byte(pageDesignPropsTheme), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "create page MyFirstModule.Home_Native (Title: 'Home', Layout: Atlas_Core.NativePhone_Default) {\n" +
		"  dynamictext text4 (Content: 'JTS boat log')\n};\n" +
		"alter page MyFirstModule.Home_Native {\n  set ('Background color': 'Background Primary', 'Hide header bottom border': on)\n};"
	if err := afRun(t, exec, src); err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := storedPageDesignProperties(t, exec, "Home_Native")
	if got["Background color"] != "Forms$OptionDesignPropertyValue" || got["Hide header bottom border"] != "Forms$ToggleDesignPropertyValue" {
		t.Fatalf("after the page-level set: %v", got)
	}
	// OFF removes a toggle, as on a widget.
	if err := afRun(t, exec, "alter page MyFirstModule.Home_Native {\n  set ('Hide header bottom border': off)\n};"); err != nil {
		t.Fatal(err)
	}
	if got := storedPageDesignProperties(t, exec, "Home_Native"); len(got) != 1 {
		t.Errorf("after OFF: %v, want only Background color", got)
	}
}
