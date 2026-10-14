// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	mdlast "github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1463: a project with the native mobile widget packages
// installed got `image.def.json` and `radiobuttons.def.json` from `widget init`
// (the MDL name was the last segment of the widget id), and those took over the
// built-in web keywords. `check` then validated `image img1 (Image: …)` against
// the NATIVE Image — "has no property Image [MDL-WIDGET01]" — and validated
// `radiobuttons` against the native RadioButtons while the builder, which owns
// that keyword in its switch, wrote a Forms$RadioButtonGroup.
//
// The definitions below are the shape such a generated file has on a user's
// disk: the fix has to hold for files that already exist, not only for the
// names the next `widget init` derives.
const nativeImageDef = `{"widgetId":"com.mendix.widget.native.image.Image","mdlName":"IMAGE","widgetKind":"pluggable","templateFile":"image.json","defaultEditable":"Always",
 "propertyMappings":[{"propertyKey":"defaultImageDynamic","source":"DefaultImage","operation":"image"}]}`

const nativeRadioButtonsDef = `{"widgetId":"com.mendix.widget.native.radiobuttons.RadioButtons","mdlName":"RADIOBUTTONS","widgetKind":"pluggable","templateFile":"radiobuttons.json","defaultEditable":"Always",
 "propertyMappings":[{"propertyKey":"enum","source":"Attribute","operation":"attribute"}]}`

func writeShadowingProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	widgets := filepath.Join(dir, ".mxcli", "widgets")
	if err := os.MkdirAll(widgets, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"image.def.json":        nativeImageDef,
		"radiobuttons.def.json": nativeRadioButtonsDef,
	} {
		if err := os.WriteFile(filepath.Join(widgets, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "App.mpr")
}

func TestUserDefinitionDoesNotShadowBuiltinKeyword(t *testing.T) {
	projectPath := writeShadowingProject(t)
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	if err := reg.LoadUserDefinitions(projectPath); err != nil {
		t.Fatal(err)
	}

	if def, ok := reg.Get("image"); !ok || def.WidgetID != "com.mendix.widget.web.image.Image" {
		got := "<none>"
		if ok {
			got = def.WidgetID
		}
		t.Errorf("`image` must stay the web Image; resolves to %s", got)
	}
	if def, ok := reg.Get("radiobuttons"); ok {
		t.Errorf("`radiobuttons` is the builder's Forms$RadioButtonGroup; the registry resolves it to %s", def.WidgetID)
	}
	// The native widgets stay reachable by id (`pluggablewidget '…'`).
	for _, id := range []string{"com.mendix.widget.native.image.Image", "com.mendix.widget.native.radiobuttons.RadioButtons"} {
		if _, ok := reg.GetByWidgetID(id); !ok {
			t.Errorf("%s is no longer reachable by widget id", id)
		}
	}
	if !strings.Contains(buf.String(), "warning:") {
		t.Errorf("the refused keyword must be reported as a warning, got log: %q", buf.String())
	}
}

// Control: a user definition for the SAME widget id as a built-in is a
// deliberate override and still wins.
func TestUserDefinitionSameWidgetIDStillOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	widgets := filepath.Join(dir, ".mxcli", "widgets")
	if err := os.MkdirAll(widgets, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"widgetId":"com.mendix.widget.web.image.Image","mdlName":"image","templateFile":"image.json","defaultEditable":"Always",
 "propertyMappings":[{"propertyKey":"marker","source":"Attribute","operation":"attribute"}]}`
	if err := os.WriteFile(filepath.Join(widgets, "image.def.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.LoadUserDefinitions(filepath.Join(dir, "App.mpr")); err != nil {
		t.Fatal(err)
	}
	def, ok := reg.Get("image")
	if !ok || len(def.PropertyMappings) != 1 || def.PropertyMappings[0].PropertyKey != "marker" {
		t.Errorf("a same-id user override of `image` must still replace the built-in, got %+v", def)
	}
}

// The reported symptom, at the layer `check` runs it.
func TestCheckImageKeywordNotValidatedAgainstNativeImage(t *testing.T) {
	reg := LoadWidgetRegistry(writeShadowingProject(t))
	if reg == nil {
		t.Fatal("registry not available")
	}
	img := &mdlast.WidgetV3{Name: "img1", Type: "image", Properties: map[string]any{"Image": "Atlas_Core.Content.mendix"}}
	for _, v := range validatePluggableWidgetProperties(img, reg, "page P") {
		if v.RuleID == "MDL-WIDGET01" {
			t.Errorf("web `image` validated against the native Image: %s", v.Message)
		}
	}
	// `radiobuttons` is built by the builder's own switch; check must not
	// resolve it to any pluggable definition either.
	rb := &mdlast.WidgetV3{Name: "rb", Type: "radiobuttons", Properties: map[string]any{"Attribute": "Status"}}
	if def := lookupWidgetDef(rb, reg); def != nil {
		t.Errorf("check resolves `radiobuttons` to %s; the builder writes Forms$RadioButtonGroup", def.WidgetID)
	}
}

// DESCRIBE must print a header that reads back to the same widget: a native
// widget whose stored definition still claims a built-in keyword falls back to
// the explicit id form.
func TestDescribeNativeWidgetHeaderReadsBack(t *testing.T) {
	reg := LoadWidgetRegistry(writeShadowingProject(t))
	if reg == nil {
		t.Fatal("registry not available")
	}
	for id, want := range map[string]string{
		"com.mendix.widget.native.image.Image":               "pluggablewidget 'com.mendix.widget.native.image.Image' w",
		"com.mendix.widget.native.radiobuttons.RadioButtons": "pluggablewidget 'com.mendix.widget.native.radiobuttons.RadioButtons' w",
		"com.mendix.widget.web.image.Image":                  "image w",
	} {
		if got := pluggableWidgetHeader(reg, id, "w"); got != want {
			t.Errorf("header for %s = %q, want %q", id, got, want)
		}
	}
}

// builderOwnedWidgetKeywords must list exactly the keywords buildWidgetV3's
// switch handles itself (bar `image`, which it hands to the registry). If the
// two drift, check and exec resolve a keyword differently again.
func TestBuilderOwnedWidgetKeywordsMatchSwitch(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "cmd_pages_builder_v3.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fromSwitch []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "buildWidgetV3" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			call, ok := sw.Tag.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "ToLower" {
				return true
			}
			for _, stmt := range sw.Body.List {
				for _, e := range stmt.(*ast.CaseClause).List {
					if lit, ok := e.(*ast.BasicLit); ok {
						s, _ := strconv.Unquote(lit.Value)
						if s != "image" {
							fromSwitch = append(fromSwitch, s)
						}
					}
				}
			}
			return false
		})
	}
	if len(fromSwitch) == 0 {
		t.Fatal("found no keyword switch in buildWidgetV3")
	}
	var fromSet []string
	for k := range builderOwnedWidgetKeywords {
		fromSet = append(fromSet, k)
	}
	sort.Strings(fromSwitch)
	sort.Strings(fromSet)
	if strings.Join(fromSwitch, ",") != strings.Join(fromSet, ",") {
		t.Errorf("builderOwnedWidgetKeywords drifted from buildWidgetV3's switch:\n switch: %v\n set:    %v", fromSwitch, fromSet)
	}
}
