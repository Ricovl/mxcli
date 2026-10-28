// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1651 (mendixlabs/mxcli#1642): a page parameter named `Context`
// passed check and exec, and mx check reported CE7247 "The name 'Context' is a
// reserved word." The platform's reserved-word list — the one attributes and
// enumeration values are already checked against — applies to the parameters
// of pages, snippets, Java actions and JavaScript actions too, case-insensitively,
// and quoting does not escape it. It does NOT apply to microflow or nanoflow
// parameters, which mxbuild judges against a different list.
func TestReservedParameterNames(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"page parameter", `create page M.Sale_View (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( $Context: M.Sale )) { };`,
			"page parameter $Context of M.Sale_View is a reserved word"},
		{"any case", `create page M.P (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( $CLASS: M.Sale )) { };`,
			"page parameter $CLASS of M.P is a reserved word"},
		{"quoted", `create page M.P (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( "Context": M.Sale )) { };`,
			"page parameter $Context of M.P is a reserved word"},
		{"primitive page parameter", `create page M.P (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( $Type: String )) { };`,
			"page parameter $Type of M.P is a reserved word"},
		{"Object, measured CE7247 though not a Java keyword", `create page M.P (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( $Object: M.Sale )) { };`,
			"page parameter $Object of M.P is a reserved word"},
		{"snippet parameter", `create snippet M.S ( params: ( $Owner: M.Sale ) ) { };`,
			"snippet parameter $Owner of M.S is a reserved word"},
		{"added to a page", `alter page M.P { add parameters $Id: M.Sale; };`,
			"page parameter $Id of M.P is a reserved word"},
		{"added to a snippet", `alter snippet M.S { add parameters $Context: M.Sale; };`,
			"snippet parameter $Context of M.S is a reserved word"},
		{"java action parameter", "create java action M.JA(Context: string not null) returns boolean as $$\nreturn true;\n$$;",
			"java action parameter Context of M.JA is a reserved word"},
		{"quoted java action parameter", "create java action M.JA(\"Context\": string not null) returns boolean as $$\nreturn true;\n$$;",
			"java action parameter Context of M.JA is a reserved word"},
		{"javascript action parameter", "create javascript action M.JS(Type: String not null) returns Boolean as $$\nreturn Promise.resolve(true);\n$$;",
			"javascript action parameter Type of M.JS is a reserved word"},
		// Controls.
		{"another name", `create page M.P (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( $LaneContext: M.Sale, $Sale: M.Sale )) { };`, ""},
		{"a system variable name is not on this list", `create page M.P (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( $currentObject: M.Sale, $latestError: M.Sale )) { };`, ""},
		{"microflow parameters are judged by another list", "create microflow M.MF ($Context: String, $Type: String) begin end;", ""},
		{"nanoflow parameters too", "create nanoflow M.NF ($Context: String) begin end;", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build("mdl 1;\n" + c.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs[0])
			}
			var got []linter.Violation
			for _, v := range ValidateProgram(prog, "") {
				if v.RuleID == "MDL-PARAM01" {
					got = append(got, v)
				}
			}
			if c.want == "" {
				if len(got) != 0 {
					t.Errorf("control must pass, got %v", got[0].Message)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want one MDL-PARAM01, got %d: %v", len(got), got)
			}
			v := got[0]
			if v.Severity != linter.SeverityError || !strings.Contains(v.Message, c.want) || !strings.Contains(v.Message, "CE7247") {
				t.Errorf("want an error naming %q and CE7247, got %s: %s", c.want, v.Severity, v.Message)
			}
			if !strings.Contains(v.Suggestion, "rename") {
				t.Errorf("want a rename suggestion, got %q", v.Suggestion)
			}
		})
	}
}

// The suggestion names a usable alternative: the entity a parameter carries,
// which is how Studio Pro names a fresh one.
func TestReservedParameterNames_SuggestsEntityName(t *testing.T) {
	prog, errs := visitor.Build("mdl 1;\ncreate page M.Sale_View (title: 'S', layout: Atlas_Core.Atlas_Default, params: ( $Context: M.Sale )) { };")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	vs := validateReservedParameterNames(prog.Statements[0])
	if len(vs) != 1 || !strings.Contains(vs[0].Suggestion, "$Sale") {
		t.Errorf("want a suggestion of $Sale, got %v", vs)
	}
}

// The list attributes and enumeration values are checked against gains
// `Object`: measured CE7247 on 11.13.0 for an attribute, an enumeration value, a
// page, snippet, Java action and JavaScript action parameter, and an entity.
func TestReservedWords_Object(t *testing.T) {
	prog, errs := visitor.Build(`mdl 1;
create non-persistent entity M.E ("Object": String(10));
create enumeration M.Kind (Object 'Object', Other 'Other');`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	seen := map[string]bool{}
	for _, v := range ValidateProgram(prog, "") {
		seen[v.RuleID] = true
	}
	if !seen["MDL021"] || !seen["MDL010"] {
		t.Errorf("want MDL021 (attribute) and MDL010 (enumeration value) for Object, got %v", seen)
	}
}

// check -p and exec agree on the reporter's page (exec --no-check included, as
// agreeExec runs it), with the reporter's workaround as the control.
func TestCheckExecAgree_ReservedPageParameter(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, "mdl 1;\ncreate persistent entity MyFirstModule.Sale (Name: String(100));"); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	page := func(param string) string {
		return `mdl 1;
create page MyFirstModule.Sale_View (
  Title: 'Sale',
  Layout: Atlas_Core.Atlas_Default,
  Params: ( $` + param + `: MyFirstModule.Sale )
) {
  layoutgrid lg { row { column (DesktopWidth: AutoFill) {
    dataview dvSale (DataSource: $` + param + `) { textbox tbName (Attribute: Name) }
  } } }
};`
	}
	assertAgree(t, exec, dir, page("Context"), "page parameter $Context of MyFirstModule.Sale_View is a reserved word")
	assertAgree(t, exec, dir, page("LaneContext"), "")
	assertAgree(t, exec, dir, "mdl 1;\ncreate java action MyFirstModule.JA_Reserved(Context: string not null) returns boolean as $$\nreturn true;\n$$;",
		"java action parameter Context of MyFirstModule.JA_Reserved is a reserved word")
}
