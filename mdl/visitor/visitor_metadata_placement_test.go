// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"
)

// R9 (ako/mxcli#755): a statement's documentation, however it is spelt, must
// say that it was written. `create or modify` keeps the stored documentation
// when a statement states none and replaces it when it states one (#1018), so
// a doc comment whose DocumentationSet stayed false was read as "none stated"
// and never reached the model on a rewrite — which is what the JSON structure,
// association, regular expression, task queue and scheduled event visitors did.
// The alias and the doc comment must build the same statement for the alias to
// be one, so both set it.
func TestDocumentationIsStatedWhateverItsSpelling(t *testing.T) {
	for _, src := range []string{
		"/** Shape */ create json structure M.J snippet '{}';",
		"create json structure M.J comment 'Shape' snippet '{}';",
		"/** Links */ create association M.A_B from M.A to M.B;",
		"create association M.A_B from M.A to M.B comment 'Links';",
		"/** Zip */ create regular expression M.Zip (Expression: 'x');",
		"create regular expression M.Zip (Expression: 'x', Documentation: 'Zip');",
		"/** Jobs */ create task queue M.Q (Parallelism: 2);",
		"create task queue M.Q (Parallelism: 2, Documentation: 'Jobs');",
		"/** Tick */ create scheduled event M.T (Microflow: M.F, Repeat: Daily);",
		"create scheduled event M.T (Microflow: M.F, Repeat: Daily, Documentation: 'Tick');",
		"/** Base */ create constant M.C type string default 'x';",
		"create constant M.C type string default 'x' comment 'Base';",
		"/** Icons */ create image collection M.I;",
		"create image collection M.I comment 'Icons';",
	} {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Errorf("%s: %v", src, errs)
			continue
		}
		v := reflect.ValueOf(prog.Statements[0]).Elem()
		set := v.FieldByName("DocumentationSet")
		if !set.IsValid() || !set.Bool() {
			t.Errorf("%s: DocumentationSet is not true on %T", src, prog.Statements[0])
		}
	}
}
