// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1407: three describe outputs that `mxcli check` could not
// take back.
//
//  1. format (decimalPrecision: -1)  -> extraneous input '-'
//  2. DecimalPrecision: -1           -> "DecimalPrecision must be a
//     non-negative integer" [MDL-WIDGET18] plus [MDL-WIDGET42]
//  3. root (Wrapper)                 -> extraneous input '('
//
// 1 and 2 are the grammar/check side: Studio Pro stores DecimalPrecision -1
// (no fixed precision), so the value describe prints is a real one and MDL
// has to accept it. 3 is the describer side: "(Wrapper)" is a generated
// marker, like "(Object)" and "(Array)", never a member a path can name.

// checkErrors runs the no-project check `mxcli check` runs and returns its
// errors, failing on a parse error.
func checkErrors(t *testing.T, script string) []string {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], script)
	}
	var out []string
	for _, v := range ValidateProgram(prog, "") {
		if v.Severity == linter.SeverityError {
			out = append(out, "["+v.RuleID+"] "+v.Message)
		}
	}
	return out
}

const precisionAutoScript = `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $T: M.T )) {
  dataview dv (DataSource: $T) {
    dynamictext dt (Content: '{1}', ContentParams: ({1} = Amount format (decimalPrecision: -1)))
    textbox tb (Label: 'Amount', Attribute: Amount, DecimalPrecision: -1)
  }
};`

func TestDecimalPrecisionAuto_ParsesAndPassesCheck(t *testing.T) {
	if errs := checkErrors(t, precisionAutoScript); len(errs) > 0 {
		t.Errorf("check refused DecimalPrecision -1:\n  %s", strings.Join(errs, "\n  "))
	}
	// Control: a precision below -1 means nothing and is still refused.
	bad := strings.Replace(precisionAutoScript, "DecimalPrecision: -1", "DecimalPrecision: -2", 1)
	if errs := checkErrors(t, bad); len(errs) == 0 {
		t.Error("check accepted DecimalPrecision -2")
	}
}

func TestDecimalPrecisionAuto_IsWrittenAsMinusOne(t *testing.T) {
	prog, errs := visitor.Build(precisionAutoScript)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	dv := page.Widgets[0]
	dt, tb := dv.Children[0], dv.Children[1]

	if fi := formattingInfoFromParamFormat(dt.GetContentParams()[0].Format); fi == nil || fi.DecimalPrecision != -1 {
		t.Errorf("dynamic text param FormattingInfo = %+v, want DecimalPrecision -1", fi)
	}
	fi, err := inputFormattingInfo(tb)
	if err != nil || fi == nil || fi.DecimalPrecision != -1 {
		t.Errorf("text box FormattingInfo = %+v (err %v), want DecimalPrecision -1", fi, err)
	}
}

// Part 3. A mapping rooted at a primitive array's item: Studio Pro stores the
// root element at the array's "(Wrapper)" item, and describe printed the
// marker as a member path.
func TestSchemaRootClause_DropsTheWrapperMarker(t *testing.T) {
	for path, want := range map[string]string{
		"(Array)|(Wrapper)":               "",
		"(Object)|tags|(Wrapper)":         " root tags",
		"(Object)|choices|(Object)|message": " root choices/message",
	} {
		if got := schemaRootClause(path); got != want {
			t.Errorf("schemaRootClause(%q) = %q, want %q", path, got, want)
		}
	}
}

// The writer half of part 3: with no root clause, a mapping over a structure
// that is a root array of primitives (Studio Pro's shape, "(Array)" ->
// "(Array)|(Wrapper)" -> "(Array)|(Wrapper)|(Value)") is rooted at the
// Wrapper — the element the describer now prints no clause for.
func TestImportMappingOverRootPrimitiveArray_RootsAtTheWrapper(t *testing.T) {
	idx := newJSONSchemaIndex([]*types.JsonElement{{
		ExposedName: "Root", Path: "(Array)", ElementType: "Array",
		Children: []*types.JsonElement{{
			ExposedName: "JsonObject", Path: "(Array)|(Wrapper)", ElementType: "Wrapper",
			Children: []*types.JsonElement{{
				ExposedName: "Value", Path: "(Array)|(Wrapper)|(Value)", ElementType: "Value", PrimitiveType: "String",
			}},
		}},
	}})
	def := &ast.ImportMappingElementDef{
		ObjectHandling: "Create", Entity: "M.Tag",
		Children: []*ast.ImportMappingElementDef{{JsonName: "Value", Attribute: "Value"}},
	}
	elem, err := buildImportMappingElementModel("M", def, "", "", nil, idx, true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if elem.JsonPath != "(Array)|(Wrapper)" {
		t.Errorf("root JsonPath = %q, want (Array)|(Wrapper)", elem.JsonPath)
	}
	if got := schemaRootClause(elem.JsonPath); got != "" {
		t.Errorf("describe would print %q for the mapping it just wrote", got)
	}
}
