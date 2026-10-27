// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"reflect"
	"strings"
	"testing"
)

// ako/mxcli#1702 (mendixlabs/mxcli#1658): MDL had no clause for the EXTRA CODE
// section and `describe java action` printed neither it nor the imports a
// developer added, so describe → exec on another project wrote a body calling
// a helper that no longer existed. `extra code $$ … $$` states the section;
// an omitted clause keeps the stored one, as before, and an empty one clears it.

func strPtr(s string) *string { return &s }

func TestRetainSections_ExtraCodeClause(t *testing.T) {
	for _, c := range []struct {
		name      string
		extraCode *string
		want      string
	}{
		{"omitted keeps the stored section", nil, storedExtra},
		{"stated replaces it", strPtr("private static int x;"), "private static int x;"},
		{"stated empty clears it", strPtr(""), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, got := RetainSections(studioProSource, nil, c.extraCode); got != c.want {
				t.Errorf("extra code = %q, want %q", got, c.want)
			}
		})
	}
}

// storedExtra is studioProSource's EXTRA CODE section as RetainedSections
// reads it.
var storedExtra = func() string { _, e := RetainedSections(studioProSource); return e }()

// StatedImports is what describe prints: the shortest leading part of the
// import list that the generator turns back into the whole list — nothing for
// the list mxbuild writes on its own, and the generated imports too when a
// developer's import follows them, since the generator appends what is missing
// rather than inserting it.
func TestStatedImports(t *testing.T) {
	gen := []string{importIContext, importUserAction}
	src := func(imports ...string) string {
		return "package m.actions;\n\n" + strings.Join(imports, "\n") + "\n\npublic class A extends UserAction<java.lang.Boolean>\n{\n}\n"
	}
	for _, c := range []struct {
		name    string
		imports []string
		want    []string
	}{
		{"generated only", gen, nil},
		{"one added after", append(append([]string{}, gen...), "import java.util.Objects;"),
			append(append([]string{}, gen...), "import java.util.Objects;")},
		{"one added before", append([]string{"import java.util.List;"}, gen...), []string{"import java.util.List;"}},
		{"no file", nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			source := ""
			if c.imports != nil {
				source = src(c.imports...)
			}
			got := StatedImports(source, nil, &BooleanType{})
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("StatedImports = %q, want %q", got, c.want)
			}
			// What describe prints regenerates the file's list.
			if source != "" {
				if regen := completeImports(got, nil, &BooleanType{}); !reflect.DeepEqual(regen, importList(source)) {
					t.Errorf("regenerated imports %q, want the file's %q", regen, importList(source))
				}
			}
		})
	}
}
