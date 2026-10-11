// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// treeNodeTypesInSource reads every node type project_tree.go can emit from its
// source: each `Type: "x"` literal, plus `externalentity`, which is assigned
// through a variable. Reading the source rather than a hand-kept list is the
// point — a list is what drifted (the VS Code tree offered javascriptaction and
// pagetemplate clicks that no describe could answer).
func treeNodeTypesInSource(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("project_tree.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{"externalentity": true}
	for _, m := range regexp.MustCompile(`Type:\s*"([a-z]+)"`).FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestProjectTree_EveryNodeTypeIsDescribableOrDeclaredWithoutSource is the
// drift guard between `project-tree` and `describe`: a node type the tree emits
// is either one `describe` answers (so a client may offer its MDL) or on the
// explicit no-source list. A new type in the tree fails here until someone
// decides which.
func TestProjectTree_EveryNodeTypeIsDescribableOrDeclaredWithoutSource(t *testing.T) {
	for _, typ := range treeNodeTypesInSource(t) {
		_, noSource := treeTypesWithoutSource[typ]
		describable := treeNodeDescribable(typ)
		switch {
		case describable && noSource:
			t.Errorf("%q is describable but also listed in treeTypesWithoutSource; remove it from the list", typ)
		case !describable && !noSource:
			t.Errorf("project-tree emits %q, which describe cannot answer and which is not in treeTypesWithoutSource; "+
				"add a describe keyword or list it with the reason", typ)
		}
	}
	for typ := range treeTypesWithoutSource {
		if !slices.Contains(treeNodeTypesInSource(t), typ) {
			t.Errorf("treeTypesWithoutSource lists %q, which project-tree never emits", typ)
		}
	}
}

// TestProjectTree_DescribableTypesParse pins that each type the tree marks
// describable produces an MDL statement that parses — a keyword mapped to a
// statement the grammar rejects would mark a node the client then fails on.
func TestProjectTree_DescribableTypesParse(t *testing.T) {
	for _, typ := range treeNodeTypesInSource(t) {
		if !treeNodeDescribable(typ) {
			continue
		}
		stmt, _ := describeMDLCommand(strings.ToUpper(typ), "MyModule.Thing")
		if _, errs := visitor.Build(stmt); len(errs) > 0 {
			t.Errorf("%q maps to %q, which does not parse: %v", typ, stmt, errs)
		}
	}
}

func TestProjectTree_MarkDescribable(t *testing.T) {
	tree := []*TreeNode{{
		Label: "Mod", Type: "module", QualifiedName: "Mod",
		Children: []*TreeNode{
			{Label: "Folder", Type: "folder"},
			{Label: "ACT_X", Type: "microflow", QualifiedName: "Mod.ACT_X"},
			{Label: "JS_X", Type: "javascriptaction", QualifiedName: "Mod.JS_X"},
			{Label: "Tmpl", Type: "pagetemplate", QualifiedName: "Mod.Tmpl"},
		},
	}, {Label: "Navigation", Type: "navigation", QualifiedName: "Navigation"}}
	markDescribable(tree)

	want := map[string]bool{"Mod": false, "Folder": false, "ACT_X": true, "JS_X": true, "Tmpl": false, "Navigation": true}
	var walk func([]*TreeNode)
	walk = func(nodes []*TreeNode) {
		for _, n := range nodes {
			if n.Describable != want[n.Label] {
				t.Errorf("%s (%s): describable = %v, want %v", n.Label, n.Type, n.Describable, want[n.Label])
			}
			walk(n.Children)
		}
	}
	walk(tree)
}

// TestDescribeMDLCommand_NavigationRoot pins that the tree's Navigation node
// (qualified name "Navigation", not a profile) describes every profile rather
// than looking for a profile called "Navigation".
func TestDescribeMDLCommand_NavigationRoot(t *testing.T) {
	if got, _ := describeMDLCommand("NAVIGATION", "Navigation"); got != "DESCRIBE NAVIGATION" {
		t.Errorf("navigation root = %q, want DESCRIBE NAVIGATION", got)
	}
	if got, _ := describeMDLCommand("NAVIGATION", "Responsive"); got != "DESCRIBE NAVIGATION Responsive" {
		t.Errorf("navigation profile = %q, want DESCRIBE NAVIGATION Responsive", got)
	}
}
