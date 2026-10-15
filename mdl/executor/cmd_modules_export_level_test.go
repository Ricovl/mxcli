// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1498: nothing mxcli printed said a module was Protected, nor
// what an entity's export level was, so the reporter found both by a raw read.
// MDL has no syntax for either, so they print as comments — the output still
// re-runs, and a rewrite keeps the stored values. Only a Protected module's
// output changes; Open is the control.
func TestDescribe_ProtectedModuleExportLevels(t *testing.T) {
	ctx := protectedModuleFixture(t, nil)
	buf := ctx.Output.(*bytes.Buffer)

	describe := func(f func() error) string {
		t.Helper()
		buf.Reset()
		if err := f(); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		parseMDL(t, out) // still re-runnable
		return out
	}

	out := describe(func() error { return describeModule(ctx, "MyAddOn", false) })
	if !strings.Contains(out, "-- export level: Protected (AddOn module)") {
		t.Errorf("describe module of a protected module does not say so:\n%s", out)
	}
	out = describe(func() error { return describeModule(ctx, "Open", false) })
	if strings.Contains(out, "export level") {
		t.Errorf("describe module of an ordinary module mentions an export level:\n%s", out)
	}

	for _, c := range []struct{ entity, want string }{
		{"Request", "-- export level: Hidden"},
		{"Pub", "-- export level: Usable"},
	} {
		out = describe(func() error { return describeEntity(ctx, ast.QualifiedName{Module: "MyAddOn", Name: c.entity}) })
		if !strings.Contains(out, c.want) {
			t.Errorf("describe entity MyAddOn.%s: want %q in\n%s", c.entity, c.want, out)
		}
	}
	out = describe(func() error { return describeEntity(ctx, ast.QualifiedName{Module: "Open", Name: "E"}) })
	if strings.Contains(out, "export level") {
		t.Errorf("describe entity in an ordinary module mentions an export level:\n%s", out)
	}
}

func TestShowModules_MarksProtectedModule(t *testing.T) {
	ctx := protectedModuleFixture(t, nil)
	buf := ctx.Output.(*bytes.Buffer)
	if err := listModules(ctx); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		switch {
		case strings.Contains(line, "| MyAddOn "):
			if !strings.Contains(line, "Protected AddOn") {
				t.Errorf("MyAddOn row does not say Protected: %s", line)
			}
		case strings.Contains(line, "| Open "):
			if strings.Contains(line, "Protected") {
				t.Errorf("Open row says Protected: %s", line)
			}
		}
	}
}
