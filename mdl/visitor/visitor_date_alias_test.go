// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// `date` as a type (ako/mxcli#714 rehearsal U1, ako/mxcli#706). Mendix has no
// date-only attribute type, and mxcli always stored `date` as a DateTime. #706
// made it a hard error in every version, so a headerless script that ran —
// mxcli-ledger's domain model — stopped running, and `fmt --upgrade` could not
// get past it although `date` -> `DateTime` is exactly what was stored.
//
// Without the header it is a deprecated alias of DateTime again (MDL-DEPR160):
// it builds the statement `DateTime` builds, and warns. Under mdl 1 it is
// refused.
func TestDateTypeIsADateTimeAlias(t *testing.T) {
	cases := []struct {
		old, canonical string
		uses           int
	}{
		{"create persistent entity M.E ( Born: date );",
			"create persistent entity M.E ( Born: DateTime );", 1},
		{"create persistent entity M.E ( TxDate: date not null error message 'required', LastImport: Date );",
			"create persistent entity M.E ( TxDate: DateTime not null error message 'required', LastImport: DateTime );", 2},
		{"alter entity M.E add attribute Born: DATE;",
			"alter entity M.E add attribute Born: DateTime;", 1},
		{"create microflow M.MF ($D: date) returns Date begin return $D; end;",
			"create microflow M.MF ($D: DateTime) returns DateTime begin return $D; end;", 2},
		{"create microflow M.MF () begin declare $d date = empty; end;",
			"create microflow M.MF () begin declare $d DateTime = empty; end;", 1},
		{"create microflow M.MF () begin $d = create date; end;",
			"create microflow M.MF () begin $d = create DateTime; end;", 1},
	}
	for _, c := range cases {
		t.Run(c.old, func(t *testing.T) {
			old := mustBuild(t, c.old)
			canon := mustBuild(t, c.canonical)
			want := deprecationCodes(canon)
			for i := 0; i < c.uses; i++ {
				want = append(want, deprecation.DateType)
			}
			if got := deprecationCodes(old); !sameCodes(got, want) {
				t.Errorf("old form recorded %v, want %v", got, want)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("different statements:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
			}
		})
	}
}

// The attribute is built as a DateTime: the type every build since `date`
// existed has stored.
func TestDateTypeBuildsDateTime(t *testing.T) {
	e := mustBuild(t, "create persistent entity M.E ( Born: date );").Statements[0].(*ast.CreateEntityStmt)
	if got := e.Attributes[0].Type.Kind; got != ast.TypeDateTime {
		t.Errorf("attribute kind %v, want DateTime", got)
	}
}

// Under mdl 1 the alias is refused: the header is the opt-in to a language
// that never had it.
func TestDateTypeRefusedUnderMdl1(t *testing.T) {
	for _, stmt := range []string{
		"create persistent entity M.E ( Born: date );",
		"create microflow M.MF ($D: Date) begin log info 'x'; end;",
	} {
		_, errs := Build("mdl 1;\n" + stmt)
		if len(errs) == 0 {
			t.Fatalf("%s: no error under mdl 1", stmt)
		}
		msg := errs[0].Error()
		for _, want := range []string{"line 2", "DateTime", deprecation.DateType} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: error %q does not mention %q", stmt, msg, want)
			}
		}
	}
}

// The fix fmt --upgrade applies replaces exactly the word.
func TestDateTypeFixReplacesTheWord(t *testing.T) {
	src := "create persistent entity M.E (\n  LastImport: date,\n  TxDate: DATE not null\n);"
	prog := mustBuild(t, src)
	runes := []rune(src)
	var got []string
	for _, d := range prog.Deprecations {
		if d.Code != deprecation.DateType {
			continue
		}
		if d.Fix == nil || len(d.Fix.Edits) != 1 {
			t.Fatalf("line %d: want one edit, got %+v", d.Line, d.Fix)
		}
		e := d.Fix.Edits[0]
		got = append(got, string(runes[e.Start:e.Stop])+"->"+e.Text)
	}
	if want := []string{"date->DateTime", "DATE->DATETIME"}; !reflect.DeepEqual(got, want) {
		t.Errorf("edits %v, want %v", got, want)
	}
}
