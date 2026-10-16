// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1516 repro 2: `declare $X Tmp.SomeEnum = empty` got MDL043's
// OBJECT wording even when the project (or the script) says Tmp.SomeEnum is an
// enumeration. Measured on 11.15.0: with the enumeration in the project the bare
// form is written as an enumeration variable (describe prints `enum`) and builds
// at 0 errors.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

const declareEnumFlow = `
create microflow Tmp.E1 () returns Boolean as $b
begin
  declare $X Tmp.SomeEnum = empty;
  declare $b Boolean = $X = empty;
  return $b;
end;`

func mdl043(t *testing.T, prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	t.Helper()
	voids := newVoidCodeActions(prog, open)
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		if mf, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			for _, v := range validateMicroflowWith(mf, voids) {
				if v.RuleID == "MDL043" {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

func declareEnumProject() backend.FullBackend {
	modID := model.ID("mod-tmp")
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: modID}, Name: "Tmp"}}, nil
		},
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{{ContainerID: modID, Name: "SomeEnum"}}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{ContainerID: modID,
				Entities: []*domainmodel.Entity{{Name: "Item"}}}}, nil
		},
	}
}

func TestMDL043_EnumerationNamedBare(t *testing.T) {
	for _, tc := range []struct {
		name     string
		src      string
		open     func() backend.FullBackend
		severity linter.Severity
		want     string
	}{
		{"the project says it is an enumeration", declareEnumFlow, declareEnumProject,
			linter.SeverityWarning, "declare $X enum Tmp.SomeEnum = empty"},
		{"the script creates the enumeration", "create enumeration Tmp.SomeEnum ( A 'A' );" + declareEnumFlow, nil,
			linter.SeverityWarning, "names the enumeration Tmp.SomeEnum"},
		// Controls: the object wording and the error stay where the name is not
		// an enumeration, or nothing says what it is.
		{"control: no project", declareEnumFlow, nil,
			linter.SeverityError, "creates an object variable of type Tmp.SomeEnum"},
		{"control: the project says it is an entity",
			strings.Replace(declareEnumFlow, "Tmp.SomeEnum", "Tmp.Item", 1), declareEnumProject,
			linter.SeverityError, "creates an object variable of type Tmp.Item"},
		{"control: a name that is both is not guessed",
			"create persistent entity Tmp.SomeEnum ( N: Integer );" + declareEnumFlow, declareEnumProject,
			linter.SeverityError, "creates an object variable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vs := mdl043(t, parseMDL(t, tc.src), tc.open)
			if len(vs) != 1 {
				t.Fatalf("want one MDL043, got %d: %+v", len(vs), vs)
			}
			if vs[0].Severity != tc.severity {
				t.Errorf("severity = %v, want %v", vs[0].Severity, tc.severity)
			}
			if !strings.Contains(vs[0].Message, tc.want) {
				t.Errorf("message should contain %q:\n%s", tc.want, vs[0].Message)
			}
		})
	}
}

// Repro 3: the conditional create. A create per branch into one name is CE0111
// (measured 11.15.0), and MDL063 names the two forms that build.
func TestMDL063_ConditionalCreateNamesTheIdiom(t *testing.T) {
	prog := parseMDL(t, `
create microflow Tmp.SameName ($big: Boolean) returns Integer as $n
begin
  if $big then
    $L = create Tmp.Line (Qty = 10);
  else
    $L = create Tmp.Line (Qty = 1);
  end if;
  declare $n Integer = $L/Qty;
  return $n;
end;`)
	var got []linter.Violation
	for _, v := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
		if v.RuleID == "MDL063" {
			got = append(got, v)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want one MDL063, got %+v", got)
	}
	for _, want := range []string{"sub-microflow", "if $c then", "microflow.variables"} {
		if !strings.Contains(got[0].Suggestion, want) {
			t.Errorf("suggestion should mention %q:\n%s", want, got[0].Suggestion)
		}
	}
}
