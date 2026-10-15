// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// LayoutNone opts out of the scaffolded layout: `mxcli new --layout none`.
const LayoutNone = "none"

// scaffoldLayoutName is the layout `mxcli new` creates in the project's own
// module. Named after Atlas's own default so the intent is obvious, but living
// in a module the project owns — which is the whole point.
const scaffoldLayoutName = "App_Default"

// scaffoldLayoutMDL is the layout a new project gets, and it is deliberately
// *not* a copy of Atlas's.
//
// Copying was the plan, and measurement killed it: every Atlas layout a real app
// uses carries widgets MDL cannot spell. Atlas_TopBar — the one the blank
// template's own page uses — contains a Forms$MenuBar, a Forms$SidebarToggleButton
// and a pluggable image, so describe → exec produces a topbar with no navigation
// and no logo. A scaffold that shipped that would be worse than no scaffold.
//
// What this reproduces instead is the *result*: the same three region classes
// Atlas styles (region-topbar / region-sidebar / region-content), the same
// topbar-content wrapper, navigation in the topbar and in the sidebar, and a
// placeholder named Main so every page binds to it unchanged. The sidebar toggle
// and the stock logo are the two things it does not carry.
//
// The topbar container's design properties are load-bearing, not decoration:
// `topbar-content` styles a flex row, and Atlas sets the flex direction as a
// design property rather than in the class. Without them the menu bar and the
// language selector stack on top of each other in the corner — measured in a
// browser, which is the only place that shows up. mx check is silent about it.
// They are Atlas 4's, though, and Atlas is versioned with the project: Mendix
// 10.24's template ships Atlas Core 3.17, which declares none of them, and
// mxbuild refuses each one with CE6083 (mendixlabs/mxcli#1433). So only the
// ones the project's theme declares are written — on Atlas 3 that is none, and
// none are needed, because its `.topbar-content` rule is already
// `display: flex; align-items: center`.
//
// It is MDL rather than Go so the project contains something the user could have
// written — `describe layout` gives this back, and `create or replace layout`
// re-runs it.
const scaffoldLayoutMDL = `create or replace layout {{.Module}}.{{.Layout}} (
  layouttype: 'Responsive',
  class: 'layout-atlas layout-atlas-responsive-topbar'
) {
  scrollcontainer layoutContainer {
    region top (class: 'region-topbar') {
      container topbarContent (
        class: 'topbar-content'{{if .DesignProperties}},
        designproperties: ({{range $i, $p := .DesignProperties}}{{if $i}},{{end}}
          '{{$p.Key}}': {{if $p.Toggle}}on{{else}}'{{$p.Value}}'{{end}}{{end}}
        ){{end}}
      ) {
        menubar mainMenu (profile: '{{.Profile}}')
        snippetcall languageSelector (snippet: Atlas_Core.LanguageSelectorWidget)
      }
    }
    region center (class: 'region-content') {
      placeholder Main
    }
  }
};
{{range .RepointFrom}}
alter pages in {{$.Module}} set layout = {{$.Module}}.{{$.Layout}}
  where layout = {{.}};
{{end}}`

type scaffoldLayoutData struct {
	Module           string
	Layout           string
	Profile          string
	RepointFrom      []string
	DesignProperties []scaffoldDesignProp
}

// scaffoldDesignProp is one design property on the topbar container. Toggle
// properties are written as a bare `on`, the rest as a quoted option name.
type scaffoldDesignProp struct {
	Key    string
	Value  string
	Toggle bool
}

// topbarDesignProps is what the topbar container asks for on Atlas 4. Each is
// written only where the project's theme declares it (scaffoldDesignProps).
var topbarDesignProps = []scaffoldDesignProp{
	{Key: "Flex container", Value: "Horizontal (row)"},
	{Key: "Align items Y", Value: "Center"},
	{Key: "Disable row wrap", Value: "on", Toggle: true},
	{Key: "Grow / shrink (self)", Value: "Fill container (only grow)"},
}

// scaffoldDesignProps keeps the topbar properties reg declares for a container.
// A nil registry (no readable themesource) keeps none: mxbuild checks every
// design property against the theme, so there is nothing safe to write.
func scaffoldDesignProps(reg *executor.ThemeRegistry) []scaffoldDesignProp {
	var out []scaffoldDesignProp
	for _, p := range topbarDesignProps {
		if executor.ThemeDeclaresDesignProperty(reg, "DivContainer", p.Key, p.Value) {
			out = append(out, p)
		}
	}
	return out
}

// scaffoldProjectLayout creates the project-owned layout and moves the project's
// own pages onto it.
//
// Returns the module it wrote into, or an error. Every failure is the caller's
// to report as a warning: a project without the scaffold is a perfectly good
// project, just one that starts where finding #136 describes — with a layout it
// cannot touch, in a module it must not edit.
func scaffoldProjectLayout(projectPath string, out io.Writer) (string, error) {
	target, err := scaffoldTarget(projectPath)
	if err != nil {
		return "", err
	}

	script, err := renderScaffoldScript(target, executor.LoadThemeRegistryForProject(projectPath))
	if err != nil {
		return "", err
	}
	if err := runMDL(projectPath, script, out); err != nil {
		return "", err
	}
	return target.Module, nil
}

// renderScaffoldScript writes the scaffold MDL for target, with only the design
// properties the project's theme (reg) declares.
func renderScaffoldScript(target scaffoldTargetInfo, reg *executor.ThemeRegistry) (string, error) {
	var script strings.Builder
	tmpl := template.Must(template.New("layout").Parse(scaffoldLayoutMDL))
	if err := tmpl.Execute(&script, scaffoldLayoutData{
		Module:           target.Module,
		Layout:           scaffoldLayoutName,
		Profile:          target.Profile,
		RepointFrom:      target.RepointFrom,
		DesignProperties: scaffoldDesignProps(reg),
	}); err != nil {
		return "", err
	}
	return script.String(), nil
}

// scaffoldTargetInfo is what the project has to be read for before the script
// can be written: which module owns it, which navigation profile the menus draw
// from, and which layouts its pages are actually on.
type scaffoldTargetInfo struct {
	Module      string
	Profile     string
	RepointFrom []string
}

// scaffoldTarget reads what the scaffold needs from the project.
//
// The module is the one the project owns: not from the Marketplace, not System.
// Resolved rather than hardcoded to "MyFirstModule" — that is what Mendix's
// template happens to be called today, and a name that changed would turn this
// into a step that silently did nothing.
func scaffoldTarget(projectPath string) (scaffoldTargetInfo, error) {
	var info scaffoldTargetInfo
	b := newBackendFactory()()
	if err := b.Connect(projectPath); err != nil {
		return info, err
	}
	defer func() { _ = b.Disconnect() }()

	modules, err := b.ListModules()
	if err != nil {
		return info, err
	}
	var owned []*model.Module
	for _, m := range modules {
		if m.FromAppStore || strings.TrimSpace(m.AppStoreGuid) != "" || m.Name == "System" {
			continue
		}
		owned = append(owned, m)
	}
	switch len(owned) {
	case 0:
		return info, errors.New("the project has no module of its own to put a layout in")
	case 1:
		info.Module = owned[0].Name
	default:
		// More than one is not a blank project, and guessing which one is "the
		// app" would put the layout somewhere arbitrary.
		names := make([]string, len(owned))
		for i, m := range owned {
			names[i] = m.Name
		}
		return info, fmt.Errorf("the project has more than one module of its own (%s); "+
			"create the layout yourself with `mxcli syntax layout`", strings.Join(names, ", "))
	}

	// The menus are drawn from a navigation profile by name. Reading it rather
	// than assuming "Responsive" means a template that ships a differently named
	// web profile still produces menus that resolve.
	info.Profile = "Responsive"
	if nav, nerr := b.GetNavigation(); nerr == nil && nav != nil {
		for _, p := range nav.Profiles {
			// The web profile, not a native one: a menu bar in a web layout
			// drawn from the native profile resolves to nothing.
			if !p.IsNative {
				info.Profile = p.Name
				break
			}
		}
	}

	info.RepointFrom, err = scaffoldRepointSources(b, owned[0].ID)
	if err != nil {
		return info, err
	}
	return info, nil
}

// scaffoldRepointSources lists the layouts the module's own pages currently sit
// on, excluding popups.
//
// Derived rather than hardcoded for two reasons. A hardcoded list emits a
// statement per name whether or not anything matches, so the output carries
// "Repointed 0 page(s)" lines that mean nothing. And it would name Atlas
// layouts the template might stop using, which reads as a step that ran and did
// nothing.
//
// **Popups are excluded on purpose.** A popup page sits on ModalPopup/Popup and
// must stay there: rendering a popup inside the app frame — topbar, sidebar and
// all — is a visible regression, and repointing every page in the module would
// do exactly that.
func scaffoldRepointSources(b backend.FullBackend, moduleID model.ID) ([]string, error) {
	// A page's ContainerID is its folder when it sits in one, so "is this page
	// in the module" is a walk up the container tree, not an equality check.
	h, err := executor.NewContainerHierarchyFromBackend(b)
	if err != nil {
		return nil, err
	}
	layouts, err := b.ListLayouts()
	if err != nil {
		return nil, err
	}
	isPopup := make(map[string]bool, len(layouts))
	for _, l := range layouts {
		switch l.LayoutType {
		case pages.LayoutTypeModalPopup, pages.LayoutTypePopup:
			isPopup[l.Name] = true
		}
	}

	allPages, err := b.ListPages()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range allPages {
		if h.FindModuleID(p.ContainerID) != moduleID {
			continue
		}
		name, lerr := b.PageLayoutName(p.ID)
		if lerr != nil || name == "" || seen[name] {
			continue
		}
		// The name is qualified; the popup check is on the layout's own name.
		bare := name
		if i := strings.LastIndex(name, "."); i >= 0 {
			bare = name[i+1:]
		}
		if isPopup[bare] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// runMDL executes a script against a project, returning the error instead of
// exiting — unlike executeMDL, which is for top-level subcommands. A step of
// `mxcli new` has to be able to fail without taking the project with it.
func runMDL(projectPath, script string, out io.Writer) error {
	exec, logger := newLoggedExecutorTo("subcommand", out)
	defer logger.Close()
	defer exec.Close()

	full := fmt.Sprintf("CONNECT LOCAL '%s'; %s", visitor.QuoteString(projectPath), script)
	prog, errs := visitor.Build(full)
	if len(errs) > 0 {
		return fmt.Errorf("scaffold script did not parse: %v", errs[0])
	}
	if err := exec.ExecuteProgram(prog); err != nil && !errors.Is(err, executor.ErrExit) {
		return err
	}
	return nil
}
