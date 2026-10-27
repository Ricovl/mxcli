// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1602 (ako/mxcli#1512), second half: "Other unit types without
// a case (constants, java actions, etc.) have the same silent diff. At minimum,
// fall back to a field-level difference summary (path, type, size, no values)
// instead of '(no changes)'."
package executor

import (
	"strings"
	"testing"
)

const fld1602Unit = "5a5a5a5a-1111-2222-3333-444444444444"

func TestDiffLocal_UnrenderedTypeListsChangedFields(t *testing.T) {
	unitPath := "mprcontents/5a/5a/" + fld1602Unit + ".mxunit"
	constant := func(def string) map[string]any {
		return map[string]any{
			"$Type":        "Constants$Constant",
			"Name":         "ApiKey",
			"DefaultValue": def,
			"Type":         map[string]any{"$Type": "DataTypes$StringType"},
		}
	}
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": constant("x"),
		"HEAD":   constant("secret123"),
	})

	ctx, out := set1602Context()
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{Format: DiffFormatStructural}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "(no changes)") {
		t.Errorf("a changed constant reported (no changes) (#1602):\n%s", got)
	}
	if !strings.Contains(got, "DefaultValue") {
		t.Errorf("structural diff does not name the changed field DefaultValue:\n%s", got)
	}

	ctx, out = set1602Context()
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got = out.String()
	if !strings.Contains(got, "DefaultValue (string, 1 → 9 chars)") {
		t.Errorf("unified diff shows no field-level hunk for the changed constant (#1602):\n%s", got)
	}
	if strings.Contains(got, "secret123") {
		t.Errorf("the field summary printed a value:\n%s", got)
	}
}

// Security$ModuleSecurity contains "Module", so it went to the module renderer
// and both sides rendered as `create module ;` — a header that names the wrong
// thing on top of saying nothing.
func TestDiffLocal_ModuleSecurityIsNotRenderedAsAModule(t *testing.T) {
	unitPath := "mprcontents/5a/5a/" + fld1602Unit + ".mxunit"
	sec := func(roles ...any) map[string]any {
		return map[string]any{
			"$Type":       "Security$ModuleSecurity",
			"ModuleRoles": append([]any{int32(3)}, roles...),
		}
	}
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": sec(map[string]any{"$Type": "Security$ModuleRole", "Name": "User"}),
		"HEAD": sec(map[string]any{"$Type": "Security$ModuleRole", "Name": "User"},
			map[string]any{"$Type": "Security$ModuleRole", "Name": "Admin"}),
	})

	ctx, out := set1602Context()
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "create module") {
		t.Errorf("module security rendered as a module:\n%s", got)
	}
	if !strings.Contains(got, `ModuleRoles["Admin"] (added)`) {
		t.Errorf("the added module role is not listed (#1602):\n%s", got)
	}
}
