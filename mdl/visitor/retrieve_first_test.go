// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `retrieve … first` binds one object and `retrieve … limit 1` a list of one
// under mdl 1; without the header `limit 1` keeps its alpha meaning (an object)
// and warns MDL-V1-LIMIT1 (ako/mxcli#734, ADR-0011).

func retrieveOf(t *testing.T, src string) (*ast.RetrieveStmt, *ast.Program) {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("%q: unexpected errors: %v", src, errs)
	}
	var mf *ast.CreateMicroflowStmt
	for _, s := range prog.Statements {
		if m, ok := s.(*ast.CreateMicroflowStmt); ok {
			mf = m
		}
	}
	if mf == nil || len(mf.Body) == 0 {
		t.Fatalf("%q: no microflow body", src)
	}
	r, ok := mf.Body[0].(*ast.RetrieveStmt)
	if !ok {
		t.Fatalf("%q: body[0] = %T, want *ast.RetrieveStmt", src, mf.Body[0])
	}
	return r, prog
}

func retrieveMicroflow(rangeClause string) string {
	return "create microflow M.F () begin\n  retrieve $O from M.Order where [Status = 'Open'] " +
		rangeClause + ";\nend;"
}

func limitOneNotes(prog *ast.Program) int {
	n := 0
	for _, note := range prog.LanguageNotes {
		if note.Code == "MDL-V1-LIMIT1" {
			n++
		}
	}
	return n
}

func TestRetrieveFirst_IsAnObjectUnderBothVersions(t *testing.T) {
	for _, header := range []string{"", "mdl 1;\n"} {
		r, prog := retrieveOf(t, header+retrieveMicroflow("first"))
		if !r.First || r.Limit != "" || r.Offset != "" {
			t.Errorf("header %q: first → First=%v Limit=%q Offset=%q; want the object range and no limit",
				header, r.First, r.Limit, r.Offset)
		}
		if n := limitOneNotes(prog); n != 0 {
			t.Errorf("header %q: `first` means the same in every version, yet %d MDL-V1-LIMIT1 note(s)", header, n)
		}
	}
}

func TestRetrieveLimitOne_Mdl0IsAnObjectAndWarns(t *testing.T) {
	r, prog := retrieveOf(t, retrieveMicroflow("limit 1"))
	if !r.First || r.Limit != "" {
		t.Errorf("mdl 0 `limit 1` → First=%v Limit=%q; want the alpha meaning, the object range", r.First, r.Limit)
	}
	if n := limitOneNotes(prog); n != 1 {
		t.Fatalf("mdl 0 `limit 1`: got %d MDL-V1-LIMIT1 notes, want 1: %+v", n, prog.LanguageNotes)
	}
	if msg := prog.LanguageNotes[0].Message; !strings.Contains(msg, "first") {
		t.Errorf("the warning should name the spelling that keeps the object meaning, `first`: %s", msg)
	}
}

func TestRetrieveLimitOne_Mdl1IsAListOfOne(t *testing.T) {
	r, prog := retrieveOf(t, "mdl 1;\n"+retrieveMicroflow("limit 1"))
	if r.First || r.Limit != "1" {
		t.Errorf("mdl 1 `limit 1` → First=%v Limit=%q; want a Custom range with limit 1 (a list)", r.First, r.Limit)
	}
	if n := limitOneNotes(prog); n != 0 {
		t.Errorf("mdl 1 `limit 1` has the new meaning, nothing to warn about; got %d note(s)", n)
	}
}

// Every other range was a list before and stays one; none of them warns.
func TestRetrieveOtherRanges_AreListsInBothVersionsWithoutWarning(t *testing.T) {
	for _, header := range []string{"", "mdl 1;\n"} {
		for _, clause := range []string{"limit 2", "limit 1 offset 3", "offset 3", "limit $N"} {
			r, prog := retrieveOf(t, header+retrieveMicroflow(clause))
			if r.First {
				t.Errorf("header %q, %q: bound as an object; every range other than the first object is a list", header, clause)
			}
			if n := limitOneNotes(prog); n != 0 {
				t.Errorf("header %q, %q: %d MDL-V1-LIMIT1 note(s), want none", header, clause, n)
			}
		}
	}
}

// The object range has no offset in Mendix, so `first` does not take one.
func TestRetrieveFirst_RefusesLimitAndOffset(t *testing.T) {
	for _, clause := range []string{"first offset 2", "first limit 1", "limit 1 first"} {
		if _, errs := Build(retrieveMicroflow(clause)); len(errs) == 0 {
			t.Errorf("%q parsed; `first` is the whole range and combines with neither limit nor offset", clause)
		}
	}
}

// An association retrieve has no range in Mendix; `first` there would be dropped.
func TestRetrieveFirst_RefusedOnAnAssociation(t *testing.T) {
	src := "create microflow M.F ($P: M.Parent) begin\n  retrieve $C from $P/M.Child_Parent first;\nend;"
	if _, errs := Build(src); len(errs) == 0 {
		t.Errorf("`first` on an association retrieve was accepted")
	}
}
