// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"os"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// A file-level `-- @version:` floor must survive a later `-- @version: any`.
// Without that, PART H of 24-workflow-examples.mdl ran on the nightly's Mendix
// 10.24 leg against a project whose fixtures had been skipped, and failed with
// "entity 'WFTest.OrderContext' not found for parameter 'OrderContext'".
func TestFilterByVersion_FileBaselineSurvivesAny(t *testing.T) {
	const script = `-- @version: 11.0+
create persistent entity WFTest.OrderContext ( Name: string )
/
-- @version: any
create workflow WFTest.CompletionRules parameter $OrderContext: WFTest.OrderContext
/
`
	mx10 := &types.ProjectVersion{ProductVersion: "10.24.24.119349", MajorVersion: 10, MinorVersion: 24}
	mx11 := &types.ProjectVersion{ProductVersion: "11.13.0", MajorVersion: 11, MinorVersion: 13}

	got10, skipped10 := filterByVersion(script, mx10)
	if strings.Contains(got10, "create workflow") {
		t.Errorf("Mendix 10: the `any` section ran although the file floor is 11.0+ "+
			"and its fixtures were skipped.\nfiltered:\n%s", got10)
	}
	if strings.Contains(got10, "create persistent entity") {
		t.Errorf("Mendix 10: the gated fixture should have been skipped too")
	}
	if skipped10 == 0 {
		t.Errorf("Mendix 10: expected skipped lines, got 0")
	}

	got11, _ := filterByVersion(script, mx11)
	for _, want := range []string{"create persistent entity", "create workflow"} {
		if !strings.Contains(got11, want) {
			t.Errorf("Mendix 11: %q was dropped; the floor is satisfied so everything must run.\n%s", want, got11)
		}
	}
}

// The other six fixtures use `any` MID-file to close a gated section. There is
// no baseline there, so the section must still run on every version -- the
// control that keeps the fix above from over-firing.
func TestFilterByVersion_MidFileAnyStillReopens(t *testing.T) {
	const script = `create entity M.Always ( Name: string )
/
-- @version: 10.18+
create view entity M.Gated ( Name: string )
/
-- @version: any
create entity M.AfterAny ( Name: string )
/
`
	mx10 := &types.ProjectVersion{ProductVersion: "10.6.0", MajorVersion: 10, MinorVersion: 6}

	got, _ := filterByVersion(script, mx10)
	if !strings.Contains(got, "M.AfterAny") {
		t.Errorf("a mid-file `any` must reopen for every version:\n%s", got)
	}
	if strings.Contains(got, "M.Gated") {
		t.Errorf("the 10.18+ section must still be skipped on 10.6:\n%s", got)
	}
	if !strings.Contains(got, "M.Always") {
		t.Errorf("content before any directive must always run:\n%s", got)
	}
}

// The floor is still the file's when a language header precedes it. #901 put
// `mdl 1;` on line 1 of every doctype script, above 24-workflow-examples.mdl's
// `-- @version: 11.0+`; the header counted as a statement, so the directive
// stopped being a floor and the nightly's 10.24 leg failed exactly as before
// (TestFilterByVersion_FileBaselineSurvivesAny's comment).
func TestFilterByVersion_FileBaselineAfterLanguageHeader(t *testing.T) {
	const script = `mdl 1;
-- @version: 11.0+
create persistent entity WFTest.OrderContext ( Name: string );
-- @version: any
create workflow WFTest.CompletionRules parameter $OrderContext: WFTest.OrderContext;
`
	mx10 := &types.ProjectVersion{ProductVersion: "10.24.24.119349", MajorVersion: 10, MinorVersion: 24}
	mx11 := &types.ProjectVersion{ProductVersion: "11.13.0", MajorVersion: 11, MinorVersion: 13}

	got10, _ := filterByVersion(script, mx10)
	if strings.Contains(got10, "create workflow") {
		t.Errorf("Mendix 10: the `any` section ran although the file floor (after the header) is 11.0+.\nfiltered:\n%s", got10)
	}
	if !strings.HasPrefix(got10, "mdl 1;\n") {
		t.Errorf("Mendix 10: the language header must be kept:\n%s", got10)
	}
	got11, _ := filterByVersion(script, mx11)
	for _, want := range []string{"create persistent entity", "create workflow"} {
		if !strings.Contains(got11, want) {
			t.Errorf("Mendix 11: %q was dropped:\n%s", want, got11)
		}
	}
}

// The script the nightly failed on: on Mendix 10.24 nothing of
// 24-workflow-examples.mdl may run, since it creates WFTest.OrderContext only
// under its 11.0+ floor.
func TestFilterByVersion_WorkflowExamplesOn1024(t *testing.T) {
	content, err := os.ReadFile("../../mdl-examples/doctype-tests/24-workflow-examples.mdl")
	if err != nil {
		t.Fatal(err)
	}
	mx10 := &types.ProjectVersion{ProductVersion: "10.24.24.119349", MajorVersion: 10, MinorVersion: 24}
	got, _ := filterByVersion(string(content), mx10)
	if strings.Contains(got, "WFTest.OrderContext") {
		t.Errorf("Mendix 10.24: a line naming WFTest.OrderContext survived the file's 11.0+ floor")
	}
}
