// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1460: "Role-based widget visibility
// (ConditionalVisibilitySettings.ModuleRoles) is not printed by DESCRIBE", so a
// `create or replace` from describe output silently dropped it. The stored
// shape is TestApp's WorkflowCommons.Snip_WorkflowActivityRecord_Header: an
// expression AND a role list on one settings element.
func roleVisibleStored() map[string]any {
	return map[string]any{
		"$Type": "Forms$DivContainer",
		"Name":  "c1",
		"ConditionalVisibilitySettings": map[string]any{
			"$Type":       "Forms$ConditionalVisibilitySettings",
			"Attribute":   "",
			"Conditions":  []any{int32(2)},
			"Expression":  "$currentObject/Name != empty",
			"ModuleRoles": []any{int32(1), "WorkflowCommons.Administrator", "WorkflowCommons.User"},
		},
		"Widgets": []any{int32(2)},
	}
}

func TestDescribe_VisibleRoles(t *testing.T) {
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	raw := parseRawWidget(ctx, roleVisibleStored())
	outputWidgetMDLV3(ctx, raw[0], 1)
	got := buf.String()
	if !strings.Contains(got, "Visible: roles (WorkflowCommons.Administrator, WorkflowCommons.User)") {
		t.Errorf("describe output lacks the role visibility:\n%s", got)
	}
	if !strings.Contains(got, "Visible: $currentObject/Name != empty") {
		t.Errorf("describe output lost the expression beside the roles:\n%s", got)
	}

	// The output parses back to both settings.
	prog, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + got + "}\n")
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, got)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	if roles, _ := w.Properties["VisibleRoles"].([]string); !reflect.DeepEqual(roles,
		[]string{"WorkflowCommons.Administrator", "WorkflowCommons.User"}) {
		t.Errorf("VisibleRoles = %#v", w.Properties["VisibleRoles"])
	}
	if w.GetStringProp("VisibleIf") != "$currentObject/Name != empty" {
		t.Errorf("VisibleIf = %q", w.GetStringProp("VisibleIf"))
	}
}

// A navigation-list item carries its own ConditionalVisibilitySettings — 13 of
// the 97 role-locked widgets in the report were items of one navigation list.
// The item path read none of it, so describe → exec made every item visible.
func TestNavigationListItem_VisibleRolesRoundTrip(t *testing.T) {
	stored := map[string]any{
		"$Type": "Forms$NavigationList",
		"Name":  "nl",
		"Items": []any{int32(2), map[string]any{
			"$Type":  "Forms$NavigationListItem",
			"Action": map[string]any{"$Type": "Forms$NoAction"},
			"ConditionalVisibilitySettings": map[string]any{
				"$Type":       "Forms$ConditionalVisibilitySettings",
				"Expression":  "",
				"ModuleRoles": []any{int32(1), "M.Admin"},
			},
			"Widgets": []any{int32(2)},
		}},
	}
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	outputWidgetMDLV3(ctx, parseRawWidget(ctx, stored)[0], 1)
	got := buf.String()
	if !strings.Contains(got, "item (Visible: roles (M.Admin))") {
		t.Fatalf("describe output lacks the item's role visibility:\n%s", got)
	}
	prog, errs := visitor.Build("create snippet M.S {\n" + got + "}\n")
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, got)
	}
	nl := prog.Statements[0].(*ast.CreateSnippetStmtV3).Widgets[0]
	item, err := visibleWhenPB("").buildNavigationListItemV3(nl.Children[0])
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if item.ConditionalVisibility == nil || !reflect.DeepEqual(item.ConditionalVisibility.ModuleRoles, []string{"M.Admin"}) {
		t.Errorf("item ConditionalVisibility = %+v, want ModuleRoles [M.Admin]", item.ConditionalVisibility)
	}
}

// The builder carries the roles into the settings element, alone or beside an
// expression or attribute condition — Studio Pro stores all three on one node.
func TestVisibleRoles_Builds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		props map[string]any
		check func(*pages.ConditionalVisibilitySettings) string
	}{
		{"roles only", map[string]any{"VisibleRoles": []string{"M.Admin"}},
			func(c *pages.ConditionalVisibilitySettings) string {
				if c.Expression != "" || c.Attribute != "" {
					return "roles alone must store no condition"
				}
				return ""
			}},
		{"with expression", map[string]any{"VisibleRoles": []string{"M.Admin"}, "VisibleIf": "false"},
			func(c *pages.ConditionalVisibilitySettings) string {
				if c.Expression != "false" {
					return "expression lost: " + c.Expression
				}
				return ""
			}},
		{"with attribute", map[string]any{"VisibleRoles": []string{"M.Admin"},
			"VisibleWhen": &ast.VisibleWhenV3{Attribute: "IsLocal", Values: []string{"true"}}},
			func(c *pages.ConditionalVisibilitySettings) string {
				if c.Attribute != "M.Job.IsLocal" {
					return "attribute lost: " + c.Attribute
				}
				return ""
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &ast.WidgetV3{Type: "container", Name: "c1", Properties: tc.props}
			built, err := visibleWhenPB("M.Job").buildWidgetV3(w)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			cvs := built.(*pages.Container).ConditionalVisibility
			if cvs == nil {
				t.Fatal("no ConditionalVisibilitySettings — the role restriction was dropped")
			}
			if !reflect.DeepEqual(cvs.ModuleRoles, []string{"M.Admin"}) {
				t.Errorf("ModuleRoles = %v, want [M.Admin]", cvs.ModuleRoles)
			}
			if msg := tc.check(cvs); msg != "" {
				t.Error(msg)
			}
		})
	}
}
