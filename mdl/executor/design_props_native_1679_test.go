// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1679: "on `Login_Native` `text6` and `text8`, `'Color': 'Brand
// primary'` is stored before as `Forms$OptionDesignPropertyValue` (Option `Brand
// primary`), and after as `Forms$CustomDesignPropertyValue` (Value `Brand
// primary`). Other properties on the same widgets (`Size`, `Spacing bottom`)
// stay options."
//
// The theme registry read only themesource/*/web/design-properties.json, so a
// native page's widgets were typed against the web definitions. Atlas Core 4.1.3
// declares DynamicText `Color` as a web ColorPicker whose swatch is spelled
// `Brand Primary`, and as a native Dropdown with the option `Brand primary`: off
// the web list, on a ColorPicker, the value was written as a custom colour.
package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// atlasColorTheme is the shape of Atlas Core 4.1.3's two DynamicText `Color`
// definitions (themesource/atlas_core/{web,native}/design-properties.json in a
// blank 11.13.0 app), cut down to the options that matter.
var atlasColorTheme = map[string]string{
	"web": `{"DynamicText": [
		{"name": "Color", "type": "ColorPicker", "options": [{"name": "Brand Primary", "class": "text-primary"}]},
		{"name": "Size", "type": "ToggleButtonGroup", "options": [{"name": "Small", "class": "text-small"}]}]}`,
	"native": `{"DynamicText": [
		{"name": "Color", "type": "Dropdown", "options": [{"name": "Brand primary", "class": "textPrimary"}]},
		{"name": "Size", "type": "Dropdown", "options": [{"name": "Small", "class": "textSmall"}]}]}`,
}

func writeAtlasColorTheme(t *testing.T, projectDir string) {
	t.Helper()
	for platform, body := range atlasColorTheme {
		dir := filepath.Join(projectDir, "themesource", "atlas_core", platform)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "design-properties.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestThemeRegistry_LoadsNativeDefinitions(t *testing.T) {
	dir := t.TempDir()
	writeAtlasColorTheme(t, dir)
	reg, err := loadThemeRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		native bool
		want   string
	}{{false, "ColorPicker"}, {true, "Dropdown"}} {
		props := reg.ForPlatform(tc.native).GetPropertiesForWidget("DynamicText")
		if tp := findThemeProp(props, "Color"); tp == nil || tp.Type != tc.want {
			t.Errorf("native=%v: Color = %+v, want type %s", tc.native, tp, tc.want)
		}
	}
}

// storedDesignPropertyTypes maps each design-property key on the named widget of
// a stored page to the $Type of its value.
func storedDesignPropertyTypes(t *testing.T, exec *Executor, page, widget string) map[string]string {
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
		walkStoredDocs(doc, func(d bson.D) {
			if afGet(d, "Name") != widget {
				return
			}
			app, _ := afGet(d, "Appearance").(bson.D)
			for _, el := range afList(afGet(app, "DesignProperties")) {
				dp := el.(bson.D)
				v, _ := afGet(dp, "Value").(bson.D)
				out[afGet(dp, "Key").(string)] = afGet(v, "$Type").(string)
			}
		})
		return out
	}
	t.Fatalf("page MyFirstModule.%s not found", page)
	return nil
}

func walkStoredDocs(d bson.D, fn func(bson.D)) {
	fn(d)
	for _, e := range d {
		switch v := e.Value.(type) {
		case bson.D:
			walkStoredDocs(v, fn)
		case bson.A:
			for _, x := range v {
				if xd, ok := x.(bson.D); ok {
					walkStoredDocs(xd, fn)
				}
			}
		}
	}
}

func TestNativePageDesignPropertyKeepsOptionKind(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	writeAtlasColorTheme(t, filepath.Dir(exec.newExecContext(context.Background()).MprPath))

	for _, tc := range []struct {
		page, layout string
		wantColor    string
	}{
		// The reported case: a native page types against the native definitions.
		{"Login_Native", "Atlas_Core.NativePhone_Default", "Forms$OptionDesignPropertyValue"},
		// Control: on a web page the same value is off the ColorPicker's list,
		// so it is a custom colour, as before.
		{"Login_Web", "Atlas_Core.Atlas_Default", "Forms$CustomDesignPropertyValue"},
	} {
		t.Run(tc.page, func(t *testing.T) {
			src := "create page MyFirstModule." + tc.page + " (Title: 'Login', Layout: " + tc.layout + ") {\n" +
				"  dynamictext text6 (Content: 'Sign in', DesignProperties: ('Color': 'Brand primary', 'Size': 'Small'))\n};"
			if err := afRun(t, exec, src); err != nil {
				t.Fatalf("exec: %v", err)
			}
			got := storedDesignPropertyTypes(t, exec, tc.page, "text6")
			if got["Color"] != tc.wantColor {
				t.Errorf("Color stored as %q, want %q (all: %v)", got["Color"], tc.wantColor, got)
			}
			if got["Size"] != "Forms$OptionDesignPropertyValue" {
				t.Errorf("Size stored as %q, want an option", got["Size"])
			}
		})
	}
}

// The same typing on `alter page … set`, which resolves the stored page's
// platform from its layout rather than from a statement.
func TestNativePageAlterSetDesignPropertyKeepsOptionKind(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	writeAtlasColorTheme(t, filepath.Dir(exec.newExecContext(context.Background()).MprPath))

	for _, tc := range []struct {
		page, layout string
		wantColor    string
	}{
		{"Alter_Native", "Atlas_Core.NativePhone_Default", "Forms$OptionDesignPropertyValue"},
		{"Alter_Web", "Atlas_Core.Atlas_Default", "Forms$CustomDesignPropertyValue"}, // control
	} {
		t.Run(tc.page, func(t *testing.T) {
			src := "create page MyFirstModule." + tc.page + " (Title: 'Login', Layout: " + tc.layout + ") {\n" +
				"  dynamictext text8 (Content: 'Forgot password?')\n};\n" +
				"alter page MyFirstModule." + tc.page + " {\n  set ('Color': 'Brand primary') on text8\n};"
			if err := afRun(t, exec, src); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if got := storedDesignPropertyTypes(t, exec, tc.page, "text8")["Color"]; got != tc.wantColor {
				t.Errorf("Color stored as %q, want %q", got, tc.wantColor)
			}
		})
	}
}

// check -p typed a native page's design properties against the web definitions
// too, so the values Atlas's native theme declares were reported as not allowed
// (MDL-WIDGET12). A layout the script creates is resolved from the script.
func TestValidateDesignProperties_NativePageUsesNativeDefinitions(t *testing.T) {
	dir := t.TempDir()
	writeAtlasColorTheme(t, dir)
	mpr := filepath.Join(dir, "App.mpr")
	for _, tc := range []struct {
		layoutType string
		wantWarn   bool
	}{{"Default", false}, {"Responsive", true}} { // Responsive is the web control
		t.Run(tc.layoutType, func(t *testing.T) {
			prog, errs := visitor.Build("create layout M.L (LayoutType: " + tc.layoutType + ") {\n  placeholder Main\n};\n" +
				"create page M.P (Title: 'P', Layout: M.L) {\n" +
				"  dynamictext t (Content: 'x', DesignProperties: ('Color': 'Brand primary', 'Size': 'Small'))\n};")
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			var warned []string
			for _, v := range ValidateDesignProperties(prog, mpr, nil) {
				if v.RuleID == "MDL-WIDGET12" {
					warned = append(warned, v.Message)
				}
			}
			if got := len(warned) > 0; got != tc.wantWarn {
				t.Errorf("layout type %s: MDL-WIDGET12 = %v, want warned=%v", tc.layoutType, warned, tc.wantWarn)
			}
		})
	}
}
