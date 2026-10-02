// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#565, the part left open after the Float half was fixed:
//
//	Analytics.SystemMonthlyKPI declared TotalWafers: Long for
//	sum(r.WaferCount), where WaferCount is an Integer. check --references
//	passed; mx check rejected the view:
//
//	  [error] [CE6770] "View Entity is out of sync with the OQL Query."
//
// Re-measured on 11.14.0 against a blank app (one view per probe): the same view
// declared Integer builds with 0 errors, and so does sum() over a Long column
// declared Long. These tests run the --references tier on that shape — the
// source entity in the project, as it is on every run after the first.

// waferRun is the project entity the views aggregate over.
func waferRun() *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "ent-run", TypeName: "DomainModels$Entity"},
		Name:        "Run",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{BaseElement: model.BaseElement{ID: "a-wafer"}, Name: "WaferCount", Type: &domainmodel.IntegerAttributeType{}},
			{BaseElement: model.BaseElement{ID: "a-big"}, Name: "BigCount", Type: &domainmodel.LongAttributeType{}},
		},
	}
}

func sumView(column string, declared ast.DataTypeKind) *ast.CreateViewEntityStmt {
	return &ast.CreateViewEntityStmt{
		Name:       ast.QualifiedName{Module: "ServiceCore", Name: "KPI"},
		Attributes: []ast.ViewAttribute{{Name: "Total", Type: ast.DataType{Kind: declared}}},
		Query: ast.OQLQuery{
			RawQuery: "select sum(r." + column + ") as Total from ServiceCore.Run as r",
		},
	}
}

func TestViewEntity_SumWidthMismatchIsRefused(t *testing.T) {
	for _, c := range []struct {
		name     string
		column   string
		declared ast.DataTypeKind
		refused  bool
	}{
		// The reported case.
		{"sum(Integer) declared Long", "WaferCount", ast.TypeLong, true},
		{"sum(Long) declared Integer", "BigCount", ast.TypeInteger, true},
		{"sum(Integer) declared Decimal", "WaferCount", ast.TypeDecimal, true},
		// Controls: the forms mx check builds with 0 errors must stay clean.
		{"sum(Integer) declared Integer", "WaferCount", ast.TypeInteger, false},
		{"sum(Long) declared Long", "BigCount", ast.TypeLong, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := dropCheckCtx(t, waferRun())
			err := validateWithContext(ctx, sumView(c.column, c.declared), newScriptContext())
			if got := err != nil; got != c.refused {
				t.Fatalf("refused=%v, want %v (mx check 11.14.0); err=%v", got, c.refused, err)
			}
			if c.refused && !strings.Contains(err.Error(), "Total") {
				t.Errorf("the refusal should name the attribute: %v", err)
			}
		})
	}
}
