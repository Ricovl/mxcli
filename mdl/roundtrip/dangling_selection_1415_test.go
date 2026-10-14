// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1415: a data view listening to a widget describe cannot
// emit (a legacy Forms$DataGrid on the FSM template) was described as
// `DataSource: selection grid1`, which check -p and exec refused as a
// reference to a widget the script does not contain.
//
// mxcli cannot write a Forms$DataGrid, so the fixture gets one by retyping a
// list view in place: "Forms$ListView" and "Forms$DataGrid" are the same
// length, so the BSON string is replaced byte for byte and nothing else in the
// unit moves. The describer then treats it exactly as it treats the real one.
func TestDanglingSelectionSourceIsCommentedOut_1415(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.addToFixture(`create module RT1415;
create persistent entity RT1415.Item (Name: String(50));
create page RT1415.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  listview lv1 (DataSource: database from RT1415.Item) {
    dynamictext t1 (Content: '{1}', ContentParams: ({1} = Name))
  }
  dataview dv1 (DataSource: selection lv1) {
    textbox tb1 (Label: 'Name', Attribute: Name)
  }
};`)

	var pageID string
	for id, label := range h.snapshot().label {
		if label == "Forms$Page P" {
			pageID = id
		}
	}
	if pageID == "" {
		t.Fatal("page RT1415.P not found in the fixture")
	}
	path := unitPath(pageID)
	from, to := []byte("Forms$ListView\x00"), []byte("Forms$DataGrid\x00")
	if n := bytes.Count(h.fixture[path], from); n != 1 {
		t.Fatalf("page holds %d list views, want 1", n)
	}
	h.fixture[path] = bytes.Replace(h.fixture[path], from, to, 1)
	h.restore()

	described, err := h.describe("page RT1415.P")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if !strings.Contains(described, "Forms$DataGrid (lv1)") {
		t.Fatalf("the retyped grid is not described as an unauthorable widget:\n%s", described)
	}
	prog, errs := visitor.Build(described)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs[0], described)
	}
	for _, v := range executor.ValidateProgram(prog, h.mpr) {
		if v.Severity == linter.SeverityError {
			t.Errorf("check refuses the describe output: [%s] %s", v.RuleID, v.Message)
		}
	}
	if errs := h.exe.ValidateProgram(prog); len(errs) > 0 {
		t.Errorf("check --references refuses the describe output: %v\n%s", errs, described)
	}
	if err := h.exe.ExecuteProgram(prog); err != nil {
		t.Errorf("exec the describe output: %v\n%s", err, described)
	}
}
