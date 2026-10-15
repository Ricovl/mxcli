// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"text/template"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func renderScaffold(t *testing.T, data scaffoldLayoutData) string {
	t.Helper()
	var out strings.Builder
	tmpl := template.Must(template.New("layout").Parse(scaffoldLayoutMDL))
	if err := tmpl.Execute(&out, data); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// The scaffold is MDL rather than Go so a new project contains something the
// user could have written — which is only true if it parses.
func TestScaffoldLayoutMDL_Parses(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module:      "MyFirstModule",
		Layout:      scaffoldLayoutName,
		Profile:     "Responsive",
		RepointFrom: []string{"Atlas_Core.Atlas_TopBar"},
	})
	if _, errs := visitor.Build(script); len(errs) > 0 {
		t.Fatalf("the scaffold script does not parse: %v\n---\n%s", errs[0], script)
	}
}

// With no page to move, the script must be the layout alone — no repoint
// statement, so `mxcli new` does not print "Repointed 0 page(s)" for a source
// that matches nothing.
func TestScaffoldLayoutMDL_OmitsTheRepointWhenThereIsNothingToMove(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "MyFirstModule", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if strings.Contains(script, "alter pages") {
		t.Errorf("emitted a repoint with no source layouts:\n%s", script)
	}
	if _, errs := visitor.Build(script); len(errs) > 0 {
		t.Fatalf("the layout-only script does not parse: %v", errs[0])
	}
}

// The placeholder name is API: every page binds to it as Module.Layout.<Name>,
// and Atlas names it Main. A scaffold that named it anything else would refuse
// every repoint it then attempts.
func TestScaffoldLayoutMDL_DeclaresMain(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if !strings.Contains(script, "placeholder Main") {
		t.Errorf("the scaffold must declare a placeholder named Main:\n%s", script)
	}
}

// The region classes are what Atlas — and every mxcli theme — styles. A
// scaffold that used its own names would render unstyled, which looks like a
// broken app rather than a different layout.
//
// region-sidebar is deliberately absent: Atlas's sidebar is a drawer the
// Forms$SidebarToggleButton opens, and mxcli cannot author that button, so an
// always-open 232px rail would be a difference from the layout being replaced
// rather than a match for it.
func TestScaffoldLayoutMDL_KeepsTheAtlasRegionClasses(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if strings.Contains(script, "region-sidebar") {
		t.Errorf("the scaffold carries a sidebar it has no toggle button for:\n%s", script)
	}
	for _, class := range []string{"region-topbar", "region-content", "topbar-content"} {
		if !strings.Contains(script, "'"+class+"'") {
			t.Errorf("the scaffold does not carry the %q class Atlas styles:\n%s", class, script)
		}
	}
}

// The profile is substituted, not hardcoded: a template shipping a differently
// named web profile would otherwise get a menu that resolves to nothing.
func TestScaffoldLayoutMDL_UsesTheResolvedProfile(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "WebProfile",
	})
	if !strings.Contains(script, "profile: 'WebProfile'") {
		t.Errorf("the menu must draw from the resolved profile:\n%s", script)
	}
	if strings.Contains(script, "profile: 'Responsive'") {
		t.Errorf("a hardcoded profile survived substitution:\n%s", script)
	}
}

// The layout's own CSS class is what makes Atlas's chrome apply: ~24 of Atlas's
// layout rules are scoped to `.layout-atlas` and its variants. A scaffold
// without it builds cleanly and renders with no topbar bar — measured in a
// browser, which is the only place it shows.
func TestScaffoldLayoutMDL_CarriesTheAtlasLayoutClass(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if !strings.Contains(script, "class: 'layout-atlas") {
		t.Errorf("the scaffold must carry the layout-atlas class:\n%s", script)
	}
}

// Trimmed from themesource/atlas_core/web/design-properties.json of a project
// made by `mx create-project` on Mendix 10.24.0 (Atlas Core 3.17): no flex group
// at all. Its .topbar-content rule is already `display: flex; align-items:
// center`, which is why the scaffold needs no design property there.
const atlas3DesignPropsJSON = `{
 "Widget": [{"name": "Spacing", "type": "Spacing"},
  {"name": "Align self", "type": "ToggleButtonGroup", "options": [{"name": "Left", "class": "pull-left"}, {"name": "Right", "class": "pull-right"}]},
  {"name": "Hide on", "type": "ToggleButtonGroup", "options": [{"name": "Phone", "class": "hide-phone"}, {"name": "Tablet", "class": "hide-tablet"}, {"name": "Desktop", "class": "hide-desktop"}]}],
 "DivContainer": [{"name": "Align content", "type": "Dropdown", "options": [{"name": "Left align as a row", "class": "row-left"}, {"name": "Center align as a row", "class": "row-center"}]},
  {"name": "Shade", "type": "ToggleButtonGroup", "options": [{"name": "Light", "class": "background-light"}, {"name": "Dark", "class": "background-dark"}]}]
}`

// The flex group as Mendix 11.6.6's create-project ships it (Atlas Core 4).
const atlas4DesignPropsJSON = `{
 "Widget": [{"name": "Spacing", "type": "Spacing"}],
 "DivContainer": [
  {"name": "Flex container", "type": "ToggleButtonGroup", "options": [{"name": "Horizontal (row)", "class": "flex-row"}, {"name": "Vertical (column)", "class": "flex-column"}]},
  {"name": "Align items Y", "type": "ToggleButtonGroup", "options": [{"name": "Top", "class": "align-y-top"}, {"name": "Center", "class": "align-y-center"}]},
  {"name": "Grow / shrink (self)", "type": "Dropdown", "options": [{"name": "Fill container (only grow)"}, {"name": "Hug content (only shrink)"}]},
  {"name": "Disable row wrap", "type": "Toggle", "class": "flex-nowrap"}]
}`

func themeRegistryFrom(t *testing.T, js string) *executor.ThemeRegistry {
	t.Helper()
	reg := &executor.ThemeRegistry{}
	if err := json.Unmarshal([]byte(js), &reg.WidgetProperties); err != nil {
		t.Fatal(err)
	}
	return reg
}

// scaffoldDesignPropViolations parses the script and asks the validator that
// `mxcli check -p` runs on pages whether the layout's design properties exist
// in reg — an oracle independent of the filter under test.
func scaffoldDesignPropViolations(t *testing.T, script string, reg *executor.ThemeRegistry) []string {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("the scaffold script does not parse: %v\n---\n%s", errs[0], script)
	}
	var out []string
	for _, stmt := range prog.Statements {
		l, ok := stmt.(*ast.CreateLayoutStmt)
		if !ok {
			continue
		}
		// The validator walks pages and snippets; a layout's widget tree has
		// the same shape.
		asSnippet := &ast.CreateSnippetStmtV3{Name: l.Name, Widgets: l.Widgets}
		for _, v := range executor.ValidateDesignPropertiesForStatement(asSnippet, reg) {
			out = append(out, v.Message)
		}
	}
	return out
}

// mendixlabs/mxcli#1433: on Mendix 10.24 the scaffold wrote Atlas 4's flex
// design properties into a project whose Atlas Core 3.17 declares none of them.
// mx check reported CE6083 "Design property Flex container is not supported by
// your theme" four times, so the first build of `mxcli new` failed.
func TestScaffoldScript_OmitsDesignPropertiesTheThemeDoesNotDeclare(t *testing.T) {
	reg := themeRegistryFrom(t, atlas3DesignPropsJSON)
	script, err := renderScaffoldScript(scaffoldTargetInfo{
		Module: "MyFirstModule", Profile: "Responsive",
		RepointFrom: []string{"Atlas_Core.Atlas_TopBar"},
	}, reg)
	if err != nil {
		t.Fatal(err)
	}
	if v := scaffoldDesignPropViolations(t, script, reg); len(v) > 0 {
		t.Errorf("the scaffold sets design properties the project's theme does not declare "+
			"(mxbuild: CE6083):\n  %s\n---\n%s", strings.Join(v, "\n  "), script)
	}
	if strings.Contains(script, "designproperties") {
		t.Errorf("an Atlas 3 theme declares none of the topbar's properties, so none may be written:\n%s", script)
	}
}

// The other direction: on Atlas 4 (Mendix 11) the flex properties are what keep
// the menu and the language selector on one row, so they must survive the filter.
func TestScaffoldScript_KeepsTheFlexPropertiesWhereTheThemeDeclaresThem(t *testing.T) {
	reg := themeRegistryFrom(t, atlas4DesignPropsJSON)
	script, err := renderScaffoldScript(scaffoldTargetInfo{Module: "M", Profile: "Responsive"}, reg)
	if err != nil {
		t.Fatal(err)
	}
	if v := scaffoldDesignPropViolations(t, script, reg); len(v) > 0 {
		t.Errorf("unexpected violations:\n  %s", strings.Join(v, "\n  "))
	}
	for _, want := range []string{
		"'Flex container': 'Horizontal (row)'",
		"'Align items Y': 'Center'",
		"'Disable row wrap': on",
		"'Grow / shrink (self)': 'Fill container (only grow)'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %s:\n%s", want, script)
		}
	}
}

// A project with no readable theme metadata gets no design properties: mxbuild
// checks every one against the theme, so there is nothing safe to write.
func TestScaffoldScript_NoThemeNoDesignProperties(t *testing.T) {
	script, err := renderScaffoldScript(scaffoldTargetInfo{Module: "M", Profile: "Responsive"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "designproperties") {
		t.Errorf("wrote design properties with no theme to check them against:\n%s", script)
	}
	if _, errs := visitor.Build(script); len(errs) > 0 {
		t.Fatalf("does not parse: %v\n%s", errs[0], script)
	}
}
