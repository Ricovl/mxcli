// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#570. The probe app (11.14.0, mx check) behind these cases:
//
//	object-rooted  first        0 errors — and throws when it runs
//	object-rooted  limit 5      0 errors
//	object-rooted  all / none   0 errors   (Studio Pro's own default)
//	object-rooted  offset       CE6100 "This entity does not support offset."
//	list-rooted    first        0 errors
const importRangeScript = `mdl 1;
create persistent entity Probe.Item (Name: String(100), Qty: Integer);
create json structure Probe.JS_Obj sample '{"name":"a","qty":1}';
create json structure Probe.JS_List sample '[{"name":"a","qty":1}]';
create import mapping Probe.IMM_Obj with json structure Probe.JS_Obj {
  create Probe.Item { Name = name, Qty = qty }
};
create import mapping Probe.IMM_List with json structure Probe.JS_List {
  create Probe.Item { Name = name, Qty = qty }
};
`

func importRangeFlow(body string) string {
	return importRangeScript + "create microflow Probe.MF ($Json: String)\nbegin\n" + body + "\nend;\n"
}

func TestImportRange_FirstOnObjectRootedMappingIsRefused(t *testing.T) {
	got := ValidateImportMappingRange(mustBuild(t, importRangeFlow(
		`  $A = import from mapping Probe.IMM_Obj($Json) first;`)), "")
	if len(got) != 1 || got[0].RuleID != "MDL-MAP04" {
		t.Fatalf("want one MDL-MAP04, got %v", rulesOf(got))
	}
	if !strings.Contains(got[0].Message, "Probe.IMM_Obj") || !strings.Contains(got[0].Suggestion, "Drop `first`") {
		t.Errorf("the diagnostic should name the mapping and the fix: %q / %q", got[0].Message, got[0].Suggestion)
	}
	if got[0].Location.DocumentName != "MF" {
		t.Errorf("located at %q, want the microflow MF", got[0].Location.DocumentName)
	}
}

func TestImportRange_OffsetOnObjectRootedMappingIsRefused(t *testing.T) {
	got := ValidateImportMappingRange(mustBuild(t, importRangeFlow(
		`  $A = import from mapping Probe.IMM_Obj($Json) limit 5 offset 1;`)), "")
	if len(got) != 1 || got[0].RuleID != "MDL-MAP04" {
		t.Fatalf("want one MDL-MAP04, got %v", rulesOf(got))
	}
	if !strings.Contains(got[0].Message, "CE6100") {
		t.Errorf("the message should name the build error it prevents: %q", got[0].Message)
	}
}

// Nested in a branch and in a custom error handler, the activity is the same
// activity.
func TestImportRange_FirstInsideNestedBlocksIsRefused(t *testing.T) {
	got := ValidateImportMappingRange(mustBuild(t, importRangeFlow(`  if $Json != empty then
    $A = import from mapping Probe.IMM_Obj($Json) first;
  end if;`)), "")
	if len(got) != 1 {
		t.Fatalf("want one MDL-MAP04 for the import inside the IF, got %v", rulesOf(got))
	}
}

// The controls: every form mx check builds clean and that runs must stay
// silent — the rule refuses, so a false positive blocks exec.
func TestImportRange_ValidFormsPass(t *testing.T) {
	for name, body := range map[string]string{
		"object-rooted, no range":   `  $A = import from mapping Probe.IMM_Obj($Json);`,
		"object-rooted, all":        `  $A = import from mapping Probe.IMM_Obj($Json) all;`,
		"object-rooted, limit only": `  $A = import from mapping Probe.IMM_Obj($Json) limit 5;`,
		"list-rooted, first":        `  $A = import from mapping Probe.IMM_List($Json) first;`,
		"list-rooted, limit":        `  $A = import from mapping Probe.IMM_List($Json) limit 5;`,
		"mapping of unknown shape":  `  $A = import from mapping Other.IMM_Elsewhere($Json) first;`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := ValidateImportMappingRange(mustBuild(t, importRangeFlow(body)), ""); len(got) != 0 {
				t.Errorf("MDL-MAP04 fired on a valid form: %s", got[0].Message)
			}
		})
	}
}

// A mapping rooted below the structure's root (`root a/b`) can cross an array,
// which makes it list-rooted whatever the sample's first token says — so the
// rule must not decide from the sample.
func TestImportRange_RootPathIsNotJudgedFromTheSample(t *testing.T) {
	src := `mdl 1;
create persistent entity Probe.Item (Name: String(100));
create json structure Probe.JS_Wrapped sample '{"items":[{"name":"a"}]}';
create import mapping Probe.IMM_Items with json structure Probe.JS_Wrapped root items {
  create Probe.Item { Name = name }
};
create microflow Probe.MF ($Json: String)
begin
  $A = import from mapping Probe.IMM_Items($Json) first;
end;
`
	if got := ValidateImportMappingRange(mustBuild(t, src), ""); len(got) != 0 {
		t.Errorf("MDL-MAP04 judged a ROOT-path mapping from the sample: %s", got[0].Message)
	}
}

func TestJsonSampleRootShape(t *testing.T) {
	for sample, want := range map[string]mappingShape{
		`{"a":1}`:     shapeObject,
		"  \n[1,2]":   shapeList,
		``:            shapeUnknown,
		`"just text"`: shapeUnknown,
	} {
		if got := jsonSampleRootShape(sample); got != want {
			t.Errorf("jsonSampleRootShape(%q) = %v, want %v", sample, got, want)
		}
	}
}
