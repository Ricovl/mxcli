// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#1685: `[Total > @Shop.C_Limit]` was a parse error, which reads as
// "MDL cannot spell this". It parses now, and the answer is mxbuild's: an XPath
// constraint cannot use a constant. Measured on 11.13.0 (the retrieve also on
// 11.15.0), each slot alone in a fresh app, CE0161 "Error(s) in XPath
// constraint." for an Integer, a String and a Boolean constant alike — in a
// retrieve, a list view's database source and an access rule (production
// security; with security off mxbuild does not check access rules at all). An
// unknown constant is the same CE0161, never CE1613: XPath does not resolve the
// name. The control, the constant assigned to a variable and the variable
// compared, builds clean.

// xpathConstantSlots puts `@Shop.C_Limit` in each XPath slot mxcli writes.
func xpathConstantSlots() map[string]string {
	return map[string]string{
		"retrieve": `create microflow Shop.F ()
			begin
				retrieve $Items from Shop.Item where [Total > @Shop.C_Limit];
			end;`,
		"retrieve, unbracketed": `create microflow Shop.F ()
			begin
				retrieve $Items from Shop.Item where Total > @Shop.C_Limit;
			end;`,
		"retrieve, second group": `create microflow Shop.F ()
			begin
				retrieve $Items from Shop.Item where [Name != empty][Total > @Shop.C_Limit];
			end;`,
		"retrieve, nested predicate": `create microflow Shop.F ()
			begin
				retrieve $Items from Shop.Item where [Shop.Item_Order/Shop.Order[Total > @Shop.C_Limit]];
			end;`,
		"nanoflow retrieve": `create nanoflow Shop.N ()
			begin
				retrieve $Items from Shop.Item where [Total > @Shop.C_Limit];
			end;`,
		"list view database source": `create page Shop.P (title: 'Items', layout: Atlas_Core.Atlas_Default) {
				listview lv (datasource: database from Shop.Item where [Total > @Shop.C_Limit]) {
					dynamictext t (Content: 'x')
				}
			};`,
		"data source, unbracketed": `create page Shop.P (title: 'Items', layout: Atlas_Core.Atlas_Default) {
				listview lv (datasource: database from Shop.Item where Total > @Shop.C_Limit) {
					dynamictext t (Content: 'x')
				}
			};`,
		"access rule": `grant read * on entity Shop.Item to Shop.User where [Total > @Shop.C_Limit];`,
		"user task targeting": `create workflow Shop.W
			  parameter $Context: Shop.Item
			begin
			  user task Review 'Review' page Shop.TaskPage
			    targeting users xpath [Name = @Shop.C_Limit] outcomes 'Done' { };
			end workflow;`,
	}
}

func xpathConstantErrors(vs []linter.Violation) (errs, warns []linter.Violation) {
	for _, v := range vs {
		if v.RuleID != xpathConstantRule {
			continue
		}
		if v.Severity == linter.SeverityError {
			errs = append(errs, v)
		} else {
			warns = append(warns, v)
		}
	}
	return errs, warns
}

func TestXPathConstants_RefusedInEverySlot(t *testing.T) {
	for slot, src := range xpathConstantSlots() {
		t.Run(slot, func(t *testing.T) {
			errs, _ := xpathConstantErrors(validateXPathConstants(parseMDL(t, src)))
			if len(errs) != 1 {
				t.Fatalf("got %d %s errors, want 1: %v", len(errs), xpathConstantRule, errs)
			}
			msg := errs[0].Message + " " + errs[0].Suggestion
			for _, w := range []string{"@Shop.C_Limit", "CE0161", "variable"} {
				if !strings.Contains(msg, w) {
					t.Errorf("message should mention %q: %s", w, msg)
				}
			}
		})
	}
}

// CONTROLS: an `@` that is not a constant in an XPath constraint is not one.
func TestXPathConstants_SilentOutsideAConstraint(t *testing.T) {
	for name, src := range map[string]string{
		// The workaround the message suggests.
		"constant via a variable": `create microflow Shop.F ()
			begin
				declare $Limit Integer = @Shop.C_Limit;
				retrieve $Items from Shop.Item where [Total > $Limit];
			end;`,
		// Data, not a reference.
		"inside a string literal": `create microflow Shop.F ()
			begin
				retrieve $Items from Shop.Item where [Name = '@Shop.C_Limit' or Name = 'it''s @Shop.C_Limit'];
			end;`,
		"an e-mail address in a literal": `grant read * on entity Shop.Item to Shop.User where [Name = 'info@shop.example'];`,
		// A widget condition is a client expression, where a constant is valid.
		"visible condition": `create page Shop.P (title: 'Items', layout: Atlas_Core.Atlas_Default) {
				dataview dv (datasource: $Item) {
					textbox tb (Attribute: Name, Visible: [Total > @Shop.C_Limit])
				}
			};`,
		"a microflow expression": `create microflow Shop.F () returns Integer
			begin
				return @Shop.C_Limit * 2;
			end;`,
	} {
		t.Run(name, func(t *testing.T) {
			if vs := validateXPathConstants(parseMDL(t, src)); len(vs) != 0 {
				t.Errorf("reported: %v", vs)
			}
		})
	}
}

// Mendix does not validate an excluded document: a warning, not an error.
func TestXPathConstants_ExcludedFlowWarns(t *testing.T) {
	errs, warns := xpathConstantErrors(validateXPathConstants(parseMDL(t, "@excluded\n"+xpathConstantSlots()["retrieve"])))
	if len(errs) != 0 || len(warns) != 1 {
		t.Errorf("excluded: got errors %v, warnings %v; want one warning", errs, warns)
	}
}

// Two constants in one constraint are two findings; the same one twice is one.
func TestXPathConstants_EachConstantOnce(t *testing.T) {
	src := `create microflow Shop.F ()
		begin
			retrieve $Items from Shop.Item where [Total > @Shop.C_Min and Total < @Shop.C_Max and Total != @Shop.C_Min];
		end;`
	errs, _ := xpathConstantErrors(validateXPathConstants(parseMDL(t, src)))
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2 (C_Min, C_Max): %v", len(errs), errs)
	}
}

// The check is part of ValidateProgram, the list `mxcli check` reports and
// `mxcli exec` refuses on — no project needed, the refusal does not depend on
// one.
func TestXPathConstants_RunsInValidateProgram(t *testing.T) {
	errs, _ := xpathConstantErrors(ValidateProgram(parseMDL(t, xpathConstantSlots()["access rule"]), ""))
	if len(errs) != 1 {
		t.Fatalf("ValidateProgram did not report the constant: %v", errs)
	}
}
