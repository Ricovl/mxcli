// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1418: `import from mapping X` written before `create import
// mapping X` stored the activity with VariableType.Entity = "", and Mendix
// cannot load the project ("ArgumentNullException ... EntityType.set_EntityId").
// The lookup error was dropped and an empty result entity written. The builder
// must refuse the statement instead.
func TestImportFromMappingRefusesUnresolvedMapping(t *testing.T) {
	tests := []struct {
		name   string
		lookup func(moduleName, name string) (*model.ImportMapping, error)
		want   string
	}{
		{
			name: "mapping not found",
			lookup: func(moduleName, name string) (*model.ImportMapping, error) {
				return nil, errors.New("import mapping not found: " + moduleName + "." + name)
			},
			want: "not found",
		},
		{
			name: "mapping has no root entity",
			lookup: func(moduleName, name string) (*model.ImportMapping, error) {
				return &model.ImportMapping{Elements: []*model.ImportMappingElement{{Kind: "Object"}}}, nil
			},
			want: "no root entity",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &flowBuilder{
				varTypes: map[string]string{},
				backend:  &mock.MockBackend{GetImportMappingByQualifiedNameFunc: tt.lookup},
			}
			fb.addImportFromMappingAction(&ast.ImportFromMappingStmt{
				OutputVariable: "R",
				SourceVariable: "P",
				Mapping:        ast.QualifiedName{Module: "MyFirstModule", Name: "IMM_D"},
			})
			if len(fb.errors) == 0 {
				var entity model.ID
				for _, o := range fb.objects {
					if act, ok := o.(*microflows.ActionActivity); ok {
						if ix, ok := act.Action.(*microflows.ImportXmlAction); ok && ix.ResultHandling != nil {
							entity = ix.ResultHandling.ResultEntityID
						}
					}
				}
				t.Fatalf("no builder error; activity written with ResultEntityID=%q (Mendix cannot load an empty entity)", entity)
			}
			msg := strings.Join(fb.errors, "\n")
			if !strings.Contains(msg, "MyFirstModule.IMM_D") || !strings.Contains(msg, tt.want) {
				t.Fatalf("error = %q, want it to name MyFirstModule.IMM_D and %q", msg, tt.want)
			}
		})
	}
}

// MDL-ORDER01 must report the forward reference at check time, before exec
// writes the statements ahead of it.
func TestValidateScriptDefinitionOrder_ImportMappingForwardRef(t *testing.T) {
	msgs := buildOrderProgram(t, `create json structure MyFirstModule.JSON_D sample '{"code": "NL"}';
create persistent entity MyFirstModule.CountryD ( Code: string );
create microflow MyFirstModule.MF_D ( $P: string )
begin
  $R = import from mapping MyFirstModule.IMM_D($P);
end;
create import mapping MyFirstModule.IMM_D with json structure MyFirstModule.JSON_D
{ create MyFirstModule.CountryD { Code = code } };`)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "import mapping MyFirstModule.IMM_D") {
		t.Fatalf("violations = %q, want one naming import mapping MyFirstModule.IMM_D", msgs)
	}

	// The backward order is fine.
	msgs = buildOrderProgram(t, `create json structure MyFirstModule.JSON_D sample '{"code": "NL"}';
create persistent entity MyFirstModule.CountryD ( Code: string );
create import mapping MyFirstModule.IMM_D with json structure MyFirstModule.JSON_D
{ create MyFirstModule.CountryD { Code = code } };
create microflow MyFirstModule.MF_D ( $P: string )
begin
  $R = import from mapping MyFirstModule.IMM_D($P);
end;`)
	if len(msgs) != 0 {
		t.Fatalf("backward reference flagged: %q", msgs)
	}
}
