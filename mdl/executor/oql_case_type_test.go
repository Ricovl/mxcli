// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1294 — MDL031 demanded String(200) for a CASE column that Mendix
// derives as String(8), and following its "Fix:" produced CE6770.
//
// inferCaseType took the first branch it COULD type, not the first branch. In
//
//	CASE WHEN … THEN l.Code WHEN … THEN '0' + l.Code ELSE '0000' END
//
// `l.Code` cannot be typed without a project, so the concatenation in the
// second branch decided — String(200) — while Mendix takes a string CASE's
// type from its FIRST branch: String(8), Code's length.
package executor

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// caseColumnMeasurements are one-column view entities over ServiceCore.Lane
// (Code String(8), Name String(100), Unl String) and ServiceCore.Num (IntV
// Integer, LngV Long, DecV Decimal). Each was declared at every length 1–16,
// 100, 108, 112, 200 and unlimited (strings) or as Integer, Long and Decimal
// (numbers), and checked with mx check 11.12.5: `accepted` is the only
// declaration that was not CE6770 "View Entity is out of sync with the OQL
// Query".
//
// Two rules fall out. A string CASE takes its FIRST branch's type, length
// included — not the longest branch (`then l.Code else l.Name` is String(8)),
// and not 200 unless that first branch is itself derived. A numeric CASE
// widens across ALL branches, Integer < Long < Decimal.
//
// staticJudges marks the rows a check without a project can decide: those
// whose deciding branches are literals or derived expressions. A branch that
// is a bare attribute needs the source entity.
var caseColumnMeasurements = []struct {
	name         string
	expr         string
	accepted     ast.DataType
	staticJudges bool
}{
	{"the reported case", "case when length(l.Code) >= 4 then l.Code when length(l.Code) = 3 then '0' + l.Code else '0000' end", str(8), false},
	{"attribute then literal", "case when length(l.Code) >= 4 then l.Code else '0000' end", str(8), false},
	{"attribute without else", "case when length(l.Code) >= 4 then l.Code end", str(8), false},
	{"short attribute then long one", "case when length(l.Code) >= 4 then l.Code else l.Name end", str(8), false},
	{"long attribute then short one", "case when length(l.Code) >= 4 then l.Name else l.Code end", str(100), false},
	{"attribute then unlimited attribute", "case when length(l.Code) >= 4 then l.Code else l.Unl end", str(8), false},
	{"literal then attribute", "case when length(l.Code) >= 4 then '0000' else l.Code end", str(200), true},
	{"long literal then attribute", "case when length(l.Code) >= 4 then 'abcdefghijkl' else l.Code end", str(200), true},
	{"concatenation then attribute", "case when length(l.Code) >= 4 then '0' + l.Code else l.Code end", str(200), true},
	{"concatenation then literal", "case when length(l.Code) >= 4 then '0' + l.Code else '0000' end", str(200), true},
	{"literals only", "case when length(l.Code) >= 4 then 'ab' else 'abcdef' end", str(200), true},

	{"integer then decimal literal", "case when l.IntV > 4 then l.IntV else 1.5 end", num(ast.TypeDecimal), true},
	{"integer then decimal", "case when l.IntV > 4 then l.IntV else l.DecV end", num(ast.TypeDecimal), false},
	{"decimal then integer", "case when l.IntV > 4 then l.DecV else l.IntV end", num(ast.TypeDecimal), false},
	{"integer literal then decimal", "case when l.IntV > 4 then 1 else l.DecV end", num(ast.TypeDecimal), false},
	{"integer then long", "case when l.IntV > 4 then l.IntV else l.LngV end", num(ast.TypeLong), false},
	{"long then integer", "case when l.IntV > 4 then l.LngV else l.IntV end", num(ast.TypeLong), false},
	{"integer and decimal literals", "case when l.IntV > 4 then 1 else 2.5 end", num(ast.TypeDecimal), true},
}

func str(n int) ast.DataType              { return ast.DataType{Kind: ast.TypeString, Length: n} }
func num(k ast.DataTypeKind) ast.DataType { return ast.DataType{Kind: k} }

// caseColumnCandidates are the declarations every measured row is tried with.
func caseColumnCandidates(accepted ast.DataType) []ast.DataType {
	if accepted.Kind == ast.TypeString {
		return []ast.DataType{str(4), str(8), str(9), str(100), str(200), str(0)}
	}
	return []ast.DataType{num(ast.TypeInteger), num(ast.TypeLong), num(ast.TypeDecimal)}
}

func sameDeclaration(a, b ast.DataType) bool {
	return a.Kind == b.Kind && (a.Kind != ast.TypeString || a.Length == b.Length)
}

var fixSuggestionRe = regexp.MustCompile(`Fix: change to 'C: ([^']+)'`)

func caseView(expr, source string, declared ast.DataType) *ast.CreateViewEntityStmt {
	return &ast.CreateViewEntityStmt{
		Name:       ast.QualifiedName{Module: "ServiceCore", Name: "V"},
		Attributes: []ast.ViewAttribute{{Name: "C", Type: declared}},
		Query:      ast.OQLQuery{RawQuery: "select " + expr + " as C from ServiceCore." + source + " as l"},
	}
}

func caseSource(expr string) string {
	if regexp.MustCompile(`IntV|LngV|DecV`).MatchString(expr) {
		return "Num"
	}
	return "Lane"
}

// Without a project — the reported shape: the source entity is created by the
// same script, so `check` cannot type its attributes. The declaration mxbuild
// accepts must never be refused, and where the CASE can be typed from its
// literals the wrong ones must be, with the accepted type as the fix.
func TestCaseColumnType_WithoutProject(t *testing.T) {
	for _, m := range caseColumnMeasurements {
		t.Run(m.name, func(t *testing.T) {
			for _, declared := range caseColumnCandidates(m.accepted) {
				vs := ValidateOQLTypes("select "+m.expr+" as C from ServiceCore."+caseSource(m.expr)+" as l",
					[]ast.ViewAttribute{{Name: "C", Type: declared}})
				right := sameDeclaration(declared, m.accepted)
				switch {
				case right && len(vs) > 0:
					t.Errorf("declared %s — the one declaration mxbuild accepts — was refused: %s %s",
						formatDataTypeForError(declared), vs[0].Message, vs[0].Suggestion)
				case !right && m.staticJudges && len(vs) == 0:
					t.Errorf("declared %s was accepted; mxbuild reports CE6770 for it",
						formatDataTypeForError(declared))
				}
				for _, v := range vs {
					if got := fixSuggestionRe.FindStringSubmatch(v.Suggestion); got == nil ||
						got[1] != formatDataTypeForMDL(m.accepted) {
						t.Errorf("declared %s: suggestion %q, want the accepted %s",
							formatDataTypeForError(declared), v.Suggestion, formatDataTypeForMDL(m.accepted))
					}
				}
			}
		})
	}
}

// With the source entity in the project, every branch can be typed, so every
// wrong declaration is refused — and the fix names the type mxbuild accepts,
// never one it rejects.
func TestCaseColumnType_WithProject(t *testing.T) {
	attr := func(name string, typ domainmodel.AttributeType) *domainmodel.Attribute {
		return &domainmodel.Attribute{BaseElement: model.BaseElement{ID: model.ID("a-" + name)}, Name: name, Type: typ}
	}
	lane := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-lane", TypeName: "DomainModels$Entity"},
		Name:        "Lane", Persistable: true,
		Attributes: []*domainmodel.Attribute{
			attr("Code", &domainmodel.StringAttributeType{Length: 8}),
			attr("Name", &domainmodel.StringAttributeType{Length: 100}),
			attr("Unl", &domainmodel.StringAttributeType{}),
		},
	}
	nums := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-num", TypeName: "DomainModels$Entity"},
		Name:        "Num", Persistable: true,
		Attributes: []*domainmodel.Attribute{
			attr("IntV", &domainmodel.IntegerAttributeType{}),
			attr("LngV", &domainmodel.LongAttributeType{}),
			attr("DecV", &domainmodel.DecimalAttributeType{}),
		},
	}
	ctx := dropCheckCtx(t, lane, nums)
	for _, m := range caseColumnMeasurements {
		t.Run(m.name, func(t *testing.T) {
			for _, declared := range caseColumnCandidates(m.accepted) {
				err := validateWithContext(ctx, caseView(m.expr, caseSource(m.expr), declared), newScriptContext())
				right := sameDeclaration(declared, m.accepted)
				switch {
				case right && err != nil:
					t.Errorf("declared %s — the one declaration mxbuild accepts — was refused: %v",
						formatDataTypeForError(declared), err)
				case !right && err == nil:
					t.Errorf("declared %s was accepted; mxbuild reports CE6770 for it",
						formatDataTypeForError(declared))
				}
				if err == nil {
					continue
				}
				for _, got := range fixSuggestionRe.FindAllStringSubmatch(err.Error(), -1) {
					if got[1] != formatDataTypeForMDL(m.accepted) {
						t.Errorf("declared %s: the fix says %s, mxbuild accepts only %s",
							formatDataTypeForError(declared), got[1], formatDataTypeForMDL(m.accepted))
					}
				}
				if strings.Contains(err.Error(), "pass-through column 'case") {
					t.Errorf("a CASE is not a pass-through column: %v", err)
				}
			}
		})
	}
}

// The first branch decides a string CASE whatever comes after it, so an
// unlimited first branch is unlimited (measured: `then l.Unl else l.Code`
// builds only as `String`). The project path cannot refuse String(8) here —
// a source length of 0 is not judged (passthroughStringLengthMismatch, #585)
// — but it must not refuse the right answer either.
func TestCaseColumnType_UnlimitedFirstBranch(t *testing.T) {
	lane := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-lane", TypeName: "DomainModels$Entity"},
		Name:        "Lane", Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{BaseElement: model.BaseElement{ID: "a-code"}, Name: "Code", Type: &domainmodel.StringAttributeType{Length: 8}},
			{BaseElement: model.BaseElement{ID: "a-unl"}, Name: "Unl", Type: &domainmodel.StringAttributeType{}},
		},
	}
	ctx := dropCheckCtx(t, lane)
	const expr = "case when length(l.Code) >= 4 then l.Unl else l.Code end"
	if err := validateWithContext(ctx, caseView(expr, "Lane", str(0)), newScriptContext()); err != nil {
		t.Errorf("declared String — what mxbuild accepts — was refused: %v", err)
	}
	if vs := ValidateOQLTypes("select "+expr+" as C from ServiceCore.Lane as l",
		[]ast.ViewAttribute{{Name: "C", Type: str(0)}}); len(vs) > 0 {
		t.Errorf("declared String was refused without a project: %v", vs)
	}
}
