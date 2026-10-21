// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1511. A snippet call renders no element of its own, so the
// Class / Style / DynamicClasses / design properties it carries never reach the
// DOM — measured on Mendix 11.15.0 with run --local + playwright check, against
// a container on the same page carrying the same settings as the control.

// snippetCallCatalog is a real catalog holding the given widgets_data rows
// (Id, Name, WidgetType, ContainerId, ContainerQualifiedName, ContainerType,
// ModuleName, Class, Style, DynamicClasses) in module M.
func snippetCallCatalog(t *testing.T, rows ...string) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	stmts := []string{
		`INSERT INTO modules_data (Id, Name, QualifiedName, ModuleName, Source) VALUES ('m1', 'M', 'M', 'M', '')`,
	}
	for _, r := range rows {
		stmts = append(stmts, `INSERT INTO widgets_data (Id, Name, WidgetType, ContainerId, ContainerQualifiedName,
			ContainerType, ModuleName, Class, Style, DynamicClasses) VALUES `+r)
	}
	for _, st := range stmts {
		if _, err := cat.CatalogDB().Exec(st); err != nil {
			t.Fatalf("%s: %v", st, err)
		}
	}
	return cat
}

func TestSnippetCallAppearance_ReportsClassStyleDynamicClasses(t *testing.T) {
	cat := snippetCallCatalog(t,
		`('w1', 'scStyled', 'Forms$SnippetCallWidget', 'p1', 'M.P', 'PAGE', 'M', 'probe-x', 'border: 3px solid red', '''d''')`,
		// controls: a bare snippet call, and a styled container
		`('w2', 'scBare', 'Forms$SnippetCallWidget', 'p1', 'M.P', 'PAGE', 'M', '', '', '')`,
		`('w3', 'c1', 'Forms$DivContainer', 'p1', 'M.P', 'PAGE', 'M', 'probe-x', 'border: 1px', '')`,
	)
	got := NewSnippetCallAppearanceRule().Check(linter.NewLintContext(cat, nil))
	if len(got) != 1 {
		t.Fatalf("want exactly the styled snippet call reported, got %d: %+v", len(got), got)
	}
	v := got[0]
	for _, want := range []string{"scStyled", "Class", "Style", "DynamicClasses", "renders no element"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q lacks %q", v.Message, want)
		}
	}
	if v.Location.DocumentName != "P" || v.Location.DocumentType != "page" {
		t.Errorf("location = %+v, want page P", v.Location)
	}
	if !strings.Contains(v.Suggestion, "container") {
		t.Errorf("suggestion %q does not point at a container", v.Suggestion)
	}
}

// Design properties are not in the catalog; they are read from the stored
// document when a reader is available.
func TestFindSnippetCallDesignProperties(t *testing.T) {
	dp := func(key string) map[string]any {
		return map[string]any{"$Type": "Forms$DesignPropertyValue", "Key": key}
	}
	call := func(name string, props ...any) map[string]any {
		return map[string]any{
			"$Type": "Forms$SnippetCallWidget",
			"Name":  name,
			"Appearance": map[string]any{
				"DesignProperties": append([]any{int32(3)}, props...),
			},
		}
	}
	raw := map[string]any{
		"FormCall": map[string]any{"Arguments": []any{int32(2), map[string]any{
			"Widgets": []any{int32(2), map[string]any{
				"$Type": "Forms$DivContainer",
				"Name":  "c1",
				// a styled container is the control
				"Appearance": map[string]any{"DesignProperties": []any{int32(3), dp("Align self")}},
				"Widgets": []any{int32(2),
					call("scStyled", dp("Align self"), dp("Hide on")),
					call("scBare"),
				},
			}},
		}}},
	}
	got := findSnippetCallDesignProperties(raw)
	if len(got) != 1 || strings.Join(got["scStyled"], ",") != "Align self,Hide on" {
		t.Fatalf("got %v, want only scStyled with Align self, Hide on", got)
	}
}

// rawUnitReader is dataview_layout_grid_test.go's.

func TestSnippetCallAppearance_ReportsDesignPropertiesFromTheDocument(t *testing.T) {
	cat := snippetCallCatalog(t,
		`('w1', 'scDP', 'Forms$SnippetCallWidget', 'p1', 'M.P', 'PAGE', 'M', '', '', '')`,
	)
	reader := rawUnitReader{units: map[model.ID]map[string]any{
		"p1": {"FormCall": map[string]any{"Arguments": []any{int32(2), map[string]any{
			"Widgets": []any{int32(2), map[string]any{
				"$Type": "Forms$SnippetCallWidget",
				"Name":  "scDP",
				"Appearance": map[string]any{"DesignProperties": []any{int32(3),
					map[string]any{"$Type": "Forms$DesignPropertyValue", "Key": "Align self"}}},
			}},
		}}}},
	}}
	got := NewSnippetCallAppearanceRule().Check(linter.NewLintContext(cat, reader))
	if len(got) != 1 || !strings.Contains(got[0].Message, "Align self") {
		t.Fatalf("want one violation naming the design property, got %+v", got)
	}
}

func TestSnippetCallAppearanceRule_Identity(t *testing.T) {
	r := NewSnippetCallAppearanceRule()
	if r.ID() != "MPR013" {
		t.Errorf("ID = %q, want MPR013", r.ID())
	}
	if r.RequiredCatalogMode() != linter.CatalogFull {
		t.Error("ctx.Widgets() needs the full catalog; under the fast build the rule finds nothing")
	}
	if r.Name() == "" || r.Description() == "" {
		t.Error("a rule with no name or description cannot be configured or explained")
	}
}
