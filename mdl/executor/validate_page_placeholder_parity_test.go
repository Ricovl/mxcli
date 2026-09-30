// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlmodelsdk "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#841. A page's widgets live in two AST fields: `Widgets` is the bare
// body, and content written inside `placeholder X { … }` is held apart in
// `Placeholders` (#532). The builder writes both, but most of the check-time
// validators were wired to `Widgets` alone — so on a page whose content sits in
// a placeholder block, which is every admin page of CapTrackV6 and the shape
// mxcli's own skills write, those validators saw nothing:
//
//	placeholder Main {
//	  container c1 (dynamicclasses: 'if $currentObject/F then ''on'' else ''''') { }
//	  listview lv (datasource: database M.Thing) {
//	    textbox t (label: 'N', attribute: Name, editable: Always)
//	  }
//	}
//
// checked clean and exec wrote DynamicClasses as the literal text and the list
// view Editable false, where the same widgets in the bare body are refused
// (MDL-WIDGET33) and warned about (MDL-WIDGET31). #1149 had already moved the
// three --references validators onto allPageWidgets; this is the rest.
//
// The table runs every case three ways — bare body, `placeholder Main { … }`,
// and a named placeholder — through ValidateProgram, the one definition of what
// `check` (and `exec`) checks, and requires the diagnostics to be identical. The
// bare-body run must report the case's rule: that is the control which makes
// "identical" mean "reported in all three" rather than "silent in all three".

type placeholderParityCase struct {
	name string
	// params is the page's `params:` clause, if the widgets need one.
	params string
	// body is the widget content placed in each position.
	body string
	// after is appended after the page (a statement the body refers forward to).
	after string
	// want is the rule the bare body must report; "" means it must be clean.
	want string
}

var placeholderParityCases = []placeholderParityCase{
	{
		name: "MDL-WIDGET33 legacy quoted DynamicClasses (reported)",
		body: `container c1 (dynamicclasses: 'if $currentObject/Featured then ''on'' else ''''') { }`,
		want: "MDL-WIDGET33",
	},
	{
		name: "MDL-WIDGET31 list view holding an editable text box (reported)",
		body: `listview lv (datasource: database M.Thing) {
      textbox t (label: 'N', attribute: Name, editable: Always)
    }`,
		want: "MDL-WIDGET31",
	},
	{
		name: "MDL-WIDGET32 expression property written as a list",
		body: `container c1 (dynamicclasses: ['a', 'b']) { }`,
		want: "MDL-WIDGET32",
	},
	{
		name:   "MPR010 form data view outside a layout grid",
		params: `params: ( $Thing: M.Thing ),`,
		body: `dataview dvThing (datasource: $Thing) {
      textbox tb (label: 'Name', attribute: Name)
    }`,
		want: "MPR010",
	},
	{
		name: "MDL-BUTTON01 control-bar button passing $currentObject",
		body: `datagrid dgThings (datasource: database from M.Thing) {
      controlbar cb1 {
        actionbutton btnEdit (caption: 'Edit', action: show page M.Detail (Thing: $currentObject))
      }
      column (attribute: Name) { }
    }`,
		want: "MDL-BUTTON01",
	},
	{
		name: "MDL-PAGE01 reference to a page created further down",
		body: `container ctn {
      linkbutton btnNew (caption: 'New', action: show page M.Later)
    }`,
		after: `create page M.Later (title: 'L', layout: Atlas_Core.Atlas_Default) {
  container ctnL { dynamictext txtL (content: 'x') }
};`,
		want: "MDL-PAGE01",
	},
	{
		name: "control: the unquoted expression and an editable list view are clean",
		body: `container c1 (dynamicclasses: if $currentObject/Featured then 'on' else '') { }
    listview lv (datasource: database M.Thing, editable: true) {
      textbox t (label: 'N', attribute: Name, editable: Always)
    }`,
	},
}

// placeholderParityPage renders a case's body in one of the three positions.
func placeholderParityPage(c placeholderParityCase, position string) string {
	var content string
	switch position {
	case "bare":
		// One blank line where the placeholder's opening line goes, so every
		// widget sits on the same source line in all three positions and a
		// line-numbered diagnostic compares equal.
		content = "\n    " + c.body
	default:
		content = "  placeholder " + position + " {\n    " + c.body + "\n  }"
	}
	src := "create page M.P (" + c.params + " title: 'P', layout: Atlas_Core.Atlas_Default) {\n" + content + "\n};\n"
	if c.after != "" {
		src += c.after + "\n"
	}
	return src
}

// placeholderParityDiagnostics is everything ValidateProgram reports for src,
// as sorted strings so that two runs compare by value.
func placeholderParityDiagnostics(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("source does not parse: %v\n%s", errs, src)
	}
	var out []string
	for _, v := range ValidateProgram(prog, "") {
		out = append(out, diagnosticKey(v))
	}
	sort.Strings(out)
	return out
}

func diagnosticKey(v linter.Violation) string {
	return fmt.Sprintf("%s|%v|%s|%s", v.RuleID, v.Severity, v.Message, v.Suggestion)
}

func TestPlaceholderContent_SameDiagnosticsAsPageBody(t *testing.T) {
	for _, c := range placeholderParityCases {
		t.Run(c.name, func(t *testing.T) {
			bare := placeholderParityDiagnostics(t, placeholderParityPage(c, "bare"))

			// Control: the bare body reports the case's rule (or is clean).
			reported := false
			for _, d := range bare {
				if c.want != "" && strings.HasPrefix(d, c.want+"|") {
					reported = true
				}
			}
			if c.want != "" && !reported {
				t.Fatalf("control failed: the bare body did not report %s; got %v", c.want, bare)
			}
			if c.want == "" && len(bare) != 0 {
				t.Fatalf("control failed: the clean case reported %v", bare)
			}

			for _, position := range []string{"Main", "Sidebar"} {
				got := placeholderParityDiagnostics(t, placeholderParityPage(c, position))
				if strings.Join(got, "\n") != strings.Join(bare, "\n") {
					t.Errorf("inside `placeholder %s { … }` the diagnostics differ from the page body:\n  body:        %v\n  placeholder: %v",
						position, bare, got)
				}
			}
		})
	}
}

// The builder half. Placeholder content and the bare body must be written the
// same: the reported symptom was the stored DynamicClasses, so this asserts the
// built widgets — including the expression spelling that reaches storage — and
// not just the diagnostics.
func TestPlaceholderContent_SameBuiltWidgetsAsPageBody(t *testing.T) {
	// stored is a fragment the serialized widget must hold — the control that
	// the comparison is over the written document and not an empty one.
	bodies := []struct{ name, body, stored string }{
		{"container with a DynamicClasses expression", `container c1 (class: 'card', style: 'padding: 4px', dynamicclasses: if $currentObject/Featured then 'on' else '') {
      dynamictext d1 (content: 'hello')
    }`, `"DynamicClasses":"if $currentObject/Featured then 'on' else ''"`},
		{"container with a DynamicClasses string", `container c1 (dynamicclasses: 'is-featured') { }`, `"DynamicClasses":"'is-featured'"`},
		{"layout grid", `layoutgrid lg { row { column (desktopwidth: 6) { dynamictext d2 (content: 'x') } } }`, `"Forms$LayoutGridRow"`},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			c := placeholderParityCase{body: tc.body}
			bare := buildPlaceholderParityPage(t, placeholderParityPage(c, "bare"), "Main")
			if !strings.Contains(bare, tc.stored) {
				t.Fatalf("control failed: the bare body's widgets do not hold %s:\n%s", tc.stored, bare)
			}
			for _, position := range []string{"Main", "Sidebar"} {
				got := buildPlaceholderParityPage(t, placeholderParityPage(c, position), position)
				if got != bare {
					t.Errorf("inside `placeholder %s { … }` the built widgets differ from the page body:\n  body:        %s\n  placeholder: %s", position, bare, got)
				}
			}
		})
	}
}

// Generated IDs are binary; every one is blanked so two builds compare equal.
var placeholderParityIDRe = regexp.MustCompile(`"base64":"[^"]*"`)

// buildPlaceholderParityPage builds the page and returns the widgets bound to
// the named placeholder, serialized to BSON as the writer stores them, with
// generated IDs blanked.
func buildPlaceholderParityPage(t *testing.T, src, placeholder string) string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("source does not parse: %v", errs)
	}
	s := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := newPopupPageBuilder()
	pb.layoutsCache = []*pages.Layout{{
		BaseElement: model.BaseElement{ID: "layout1"},
		ContainerID: "atlas",
		Name:        "Atlas_Default",
	}}
	pb.execCache = &executorCache{hierarchy: &ContainerHierarchy{
		moduleIDs:   map[model.ID]bool{"atlas": true},
		moduleNames: map[model.ID]string{"atlas": "Atlas_Core"},
	}}
	pb.argCtx = atDocumentRoot()
	page, err := pb.buildPageV3(s)
	if err != nil {
		t.Fatalf("buildPageV3: %v", err)
	}
	if page.LayoutCall == nil {
		t.Fatal("the test layout did not resolve")
	}
	for _, arg := range page.LayoutCall.Arguments {
		if string(arg.ParameterID) != "Atlas_Core.Atlas_Default."+placeholder || len(arg.Widgets) == 0 {
			continue
		}
		var out []string
		for _, w := range arg.Widgets {
			raw, err := bson.MarshalExtJSON((&mdlmodelsdk.Backend{}).SerializeWidgetToOpaque(w), false, false)
			if err != nil {
				t.Fatalf("serialize: %v", err)
			}
			out = append(out, placeholderParityIDRe.ReplaceAllString(string(raw), `"base64":""`))
		}
		return strings.Join(out, "\n")
	}
	return ""
}

// The validators ValidateProgram only runs with a project — design properties
// against the theme, and the --references forward-page check — go through the
// same documentWidgets roots. Each is run here with the dependency supplied
// directly, three positions, bare body as the control.
func TestPlaceholderContent_ProjectValidatorsSeeIt(t *testing.T) {
	positions := []string{"bare", "Main", "Sidebar"}

	t.Run("MDL-WIDGET11 design property unknown to the theme", func(t *testing.T) {
		c := placeholderParityCase{body: `container c1 (designproperties: ['Nonexistent': 'x']) { }`}
		var runs [][]string
		for _, position := range positions {
			prog, errs := visitor.Build(placeholderParityPage(c, position))
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			var got []string
			for _, v := range ValidateDesignPropertiesForStatement(prog.Statements[0], testThemeRegistry()) {
				got = append(got, diagnosticKey(v))
			}
			runs = append(runs, got)
		}
		if len(runs[0]) == 0 {
			t.Fatal("control failed: the bare body reported no design-property violation")
		}
		for i := 1; i < len(runs); i++ {
			if strings.Join(runs[i], "\n") != strings.Join(runs[0], "\n") {
				t.Errorf("placeholder %s: got %v, body reported %v", positions[i], runs[i], runs[0])
			}
		}
	})

	t.Run("forward page reference under --references", func(t *testing.T) {
		c := placeholderParityCase{
			body: `container ctn { linkbutton btnNew (caption: 'New', action: show page M.Later) }`,
			after: `create page M.Later (title: 'L', layout: Atlas_Core.Atlas_Default) {
  container ctnL { dynamictext txtL (content: 'x') }
};`,
		}
		var runs []string
		for _, position := range positions {
			prog, errs := visitor.Build(placeholderParityPage(c, position))
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			ctx, _ := newMockCtx(t)
			var got []string
			for _, err := range validateForwardPageRefs(ctx, prog) {
				got = append(got, err.Error())
			}
			runs = append(runs, strings.Join(got, "\n"))
		}
		if !strings.Contains(runs[0], "M.Later") {
			t.Fatalf("control failed: the bare body reported no forward reference; got %q", runs[0])
		}
		for i := 1; i < len(runs); i++ {
			if runs[i] != runs[0] {
				t.Errorf("placeholder %s: got %q, body reported %q", positions[i], runs[i], runs[0])
			}
		}
	})
}

// MDL-OFFLINE01 is the one page validator that needs a project with an offline
// navigation profile before it says anything, so ValidateProgram without a
// project never reaches it and the table above cannot cover it. Seeded here the
// same way TestOfflineProfilesIn_FindsASeededOfflineProfile does; the bare body
// reporting the rule is the control.
func TestPlaceholderContent_OfflinePathsSeeIt(t *testing.T) {
	p := projectFixture(t)
	execAgainst(t, p, `create or replace navigation "PhoneOffline"
  home page "MyFirstModule"."Home_Web";`)

	c := placeholderParityCase{body: `dataview dv (datasource: $Thing) {
      textbox tb (label: 'Far', attribute: A_B/B_C/Name)
    }`, params: `params: ( $Thing: M.Thing ),`}
	var runs []string
	for _, position := range []string{"bare", "Main", "Sidebar"} {
		prog, errs := visitor.Build(placeholderParityPage(c, position))
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		var got []string
		for _, v := range ValidateOfflineAttributePaths(prog, p) {
			got = append(got, diagnosticKey(v))
		}
		runs = append(runs, strings.Join(got, "\n"))
	}
	if !strings.Contains(runs[0], "MDL-OFFLINE01") {
		t.Fatalf("control failed: the bare body reported no MDL-OFFLINE01; got %q", runs[0])
	}
	for i, position := range []string{"Main", "Sidebar"} {
		if runs[i+1] != runs[0] {
			t.Errorf("placeholder %s: got %q, body reported %q", position, runs[i+1], runs[0])
		}
	}
}
