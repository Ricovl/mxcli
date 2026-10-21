// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#199 (ako/mxcli#1231): `return $Parent/Module.Parent_Child;`
// passed `mxcli check --references` and was CE0117 "Error(s) in expression." at
// End event, and `loop $c in $Parent/Module.Parent_Child` was written with no
// list — CE0099 "The 'Iterate over' property is required." Measured on mxbuild
// 11.12.1, one microflow each into a fresh project:
//
//	return $Parent/M.Child_Parent             (reverse, list)       CE0117
//	return $Child/M.Child_Parent              (forward, object)     CE0117
//	return $Child/M.Child_Tag                 (reference set, list) CE0117
//	return $Child/M.Child_Parent/Name                                0 errors
//	return $Child/M.Child_Parent/M.Parent     (entity step, object)  0 errors
//	return $Child/M.Child_Tag/M.Tag           (entity step, list)    0 errors
//	loop $c in $Parent/M.Child_Parent                                CE0099
//
// The return value is the same shape MDL049 refuses as a call argument; the
// loop's list is a variable name in the model, so no path can be stored there.

func microflowRuleFires(t *testing.T, src, rule string) bool {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v\n%s", errs, src)
	}
	for _, vi := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
		if vi.RuleID == rule {
			return true
		}
	}
	return false
}

func TestValidateMicroflow_AssociationPathReturn(t *testing.T) {
	cases := []struct {
		name, params, returns, value string
		want                         bool
	}{
		{"reverse traversal into a list", "$Parent: M.Parent", "list of M.Child", "$Parent/M.Child_Parent", true},
		{"reference into an object", "$Child: M.Child", "M.Parent", "$Child/M.Child_Parent", true},
		{"reference set into a list", "$Child: M.Child", "list of M.Tag", "$Child/M.Child_Tag", true},
		// Controls, each 0 errors on 11.12.1.
		{"attribute over the association", "$Child: M.Child", "String", "$Child/M.Child_Parent/Name", false},
		{"target entity named", "$Child: M.Child", "M.Parent", "$Child/M.Child_Parent/M.Parent", false},
		{"reference set, target entity named", "$Child: M.Child", "list of M.Tag", "$Child/M.Child_Tag/M.Tag", false},
		{"a variable", "$Child: M.Child", "M.Child", "$Child", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "mdl 1;\ncreate microflow M.F (" + c.params + ")\nreturns " + c.returns + "\nbegin\n  return " + c.value + ";\nend;"
			if got := microflowRuleFires(t, src, "MDL049"); got != c.want {
				t.Errorf("MDL049 fired=%v, want %v", got, c.want)
			}
		})
	}
}

func TestValidateMicroflow_LoopOverAssociationPath(t *testing.T) {
	cases := []struct {
		name, source string
		want         bool
	}{
		{"association path", "$Parent/M.Child_Parent", true},
		{"target entity named", "$Parent/M.Child_Parent/M.Child", true},
		{"a list variable", "$Children", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "mdl 1;\ncreate microflow M.F ($Parent: M.Parent, $Children: list of M.Child)\nbegin\n  loop $c in " + c.source +
				" begin\n    log info node 'x' 'y';\n  end loop;\nend;"
			if got := microflowRuleFires(t, src, "MDL094"); got != c.want {
				t.Errorf("MDL094 fired=%v, want %v", got, c.want)
			}
		})
	}
}

// A nanoflow's return and loop are built the same way: measured on 11.12.1, a
// nanoflow returning `$Child/M.Child_Parent` is CE0117 at the end event, and
// `$Child/M.Child_Parent/M.Parent` is 0 errors.
func TestValidateNanoflow_AssociationPathValues(t *testing.T) {
	for _, c := range []struct {
		name, body, rule string
		want             bool
	}{
		{"bare path returned", "return $Child/M.Child_Parent;", "MDL049", true},
		{"target entity named", "return $Child/M.Child_Parent/M.Parent;", "MDL049", false},
		{"loop over a path", "loop $p in $Child/M.Child_Tag begin\n    log info node 'x' 'y';\n  end loop;\n  return empty;", "MDL094", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build("mdl 1;\ncreate nanoflow M.N ($Child: M.Child)\nreturns M.Parent\nbegin\n  " + c.body + "\nend;")
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			var got bool
			for _, vi := range ValidateNanoflow(prog.Statements[0].(*ast.CreateNanoflowStmt)) {
				got = got || vi.RuleID == c.rule
			}
			if got != c.want {
				t.Errorf("%s fired=%v, want %v", c.rule, got, c.want)
			}
		})
	}
}
