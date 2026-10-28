// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func outputMappingRow(target, targetVar, source, expr string) map[string]any {
	ref := func(attr string) any {
		if attr == "" {
			return nil
		}
		return map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": attr, "EntityRef": nil}
	}
	var sv any
	if targetVar != "" {
		sv = map[string]any{"$Type": "Forms$PageVariable", "LocalVariable": targetVar,
			"PageParameter": "", "SnippetParameter": "", "SubKey": "", "UseAllPages": false, "Widget": ""}
	}
	return map[string]any{"$Type": "Forms$OutputMapping", "AttributeRef": ref(target),
		"SourceAttributeRef": ref(source), "SourceVariable": sv, "Expression": expr}
}

func TestStoredFlowReturns(t *testing.T) {
	overAssociation := outputMappingRow("M.Vehicle.Brand", "", "", "")
	overAssociation["AttributeRef"].(map[string]any)["EntityRef"] = map[string]any{"$Type": "DomainModels$IndirectEntityRef"}
	snippetVar := outputMappingRow("", "s", "", "")
	snippetVar["SourceVariable"].(map[string]any)["SnippetParameter"] = "s"
	for _, tc := range []struct {
		name string
		row  map[string]any
		want string // "" = not sayable
	}{
		{"attribute from attribute", outputMappingRow("M.Vehicle.Brand", "", "M.Vehicle.Brand", ""), "Brand = $ActionReturnValue/Brand"},
		{"variable from the value", outputMappingRow("", "stringResult", "", ""), "$stringResult = $ActionReturnValue"},
		{"attribute from an expression", outputMappingRow("M.Vehicle.Brand", "", "", "'Demo brand'"), "Brand = 'Demo brand'"},
		{"attribute from the value", outputMappingRow("M.Vehicle.Brand", "", "", ""), "Brand = $ActionReturnValue"},
		{"keyword attribute", outputMappingRow("M.Vehicle.Type", "", "M.Vehicle.Type", ""), `Type = $ActionReturnValue/"Type"`},
		{"over an association", overAssociation, ""},
		{"a target in both slots", outputMappingRow("M.Vehicle.Brand", "s", "", ""), ""},
		{"no target", outputMappingRow("", "", "M.Vehicle.Brand", ""), ""},
		{"an expression reading like the shorthand", outputMappingRow("M.Vehicle.Brand", "", "", "$ActionReturnValue/Brand"), ""},
		{"a snippet parameter slot", snippetVar, ""},
	} {
		holder := map[string]any{"OutputMappings": []any{int32(3), tc.row}}
		got, ok := storedFlowReturns(holder)
		if tc.want == "" {
			if ok {
				t.Errorf("%s: described as %q, want it reported as having no MDL form", tc.name, got)
			}
			if n := unsayableOutputMappings(map[string]any{"MicroflowSettings": holder}); n != 1 {
				t.Errorf("%s: unsayableOutputMappings = %d, want 1", tc.name, n)
			}
			continue
		}
		if want := " returns (" + tc.want + ")"; !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", tc.name, got, ok, want)
		}
		if n := unsayableOutputMappings(map[string]any{"MicroflowSettings": holder}); n != 0 {
			t.Errorf("%s: unsayableOutputMappings = %d, want 0", tc.name, n)
		}
	}
	// A call without mappings says nothing and notes nothing.
	if got, ok := storedFlowReturns(map[string]any{"OutputMappings": []any{int32(3)}}); !ok || got != "" {
		t.Errorf("empty list: got %q (ok=%v)", got, ok)
	}
}

// Below Mendix 11.5 a flow call has no OutputMappings to store a mapping in.
func TestFlowReturnsVersionFloor(t *testing.T) {
	for _, minor := range []int{4, 5} {
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ProjectVersionFunc: func() *types.ProjectVersion {
				return &types.ProjectVersion{ProductVersion: fmt.Sprintf("11.%d.0", minor), MajorVersion: 11, MinorVersion: minor}
			},
		}
		ctx, _ := newMockCtx(t, withBackend(mb))
		pb := &pageBuilder{ctx: ctx, backend: mb, localVariables: map[string]bool{"s": true}}
		a := &pages.MicroflowClientAction{MicroflowName: "M.F"}
		err := pb.applyFlowReturns(a, []ast.FlowReturnV3{{Target: "s", Variable: true, Source: "$ActionReturnValue"}})
		if minor == 4 && (err == nil || !strings.Contains(err.Error(), "11.5.0")) {
			t.Errorf("11.4: err = %v, want a refusal naming 11.5.0", err)
		}
		if minor == 5 && (err != nil || len(a.OutputMappings) != 1) {
			t.Errorf("11.5: err = %v, mappings %d, want one mapping", err, len(a.OutputMappings))
		}
	}
}

// check -p and exec's pre-flight read the same gates the builder applies, so a
// script check passes is one exec does not then refuse (mendixlabs/mxcli#1434).
func TestFlowReturnsStatementGates(t *testing.T) {
	button := func(returns string) string {
		return "actionbutton b (Caption: 'Go', Action: call microflow M.F" + returns + ")"
	}
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"page, variable target", "create page M.P (Title: 'T', Layout: A.L) { " + button(" returns ($v = $ActionReturnValue)") + " };",
			[]string{"flow_call_output_mappings"}},
		{"page, attribute target", "create page M.P (Title: 'T', Layout: A.L) { dataview dv (DataSource: $E) { " + button(" returns (Name = $ActionReturnValue/Name)") + " } };",
			[]string{"flow_call_output_mappings", "flow_call_output_mappings_to_attribute"}},
		{"snippet", "create snippet M.S { " + button(" returns ($v = $ActionReturnValue)") + " };",
			[]string{"flow_call_output_mappings"}},
		{"alter page set", "alter page M.P { set (Action: call microflow M.F returns (Name = 'x')) on b };",
			[]string{"flow_call_output_mappings", "flow_call_output_mappings_to_attribute"}},
		{"control: no returns", "create page M.P (Title: 'T', Layout: A.L) { " + button("") + " };", nil},
	} {
		prog, errs := visitor.Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.name, errs)
		}
		var got []string
		for _, g := range statementVersionGates(prog.Statements[0]) {
			got = append(got, g.name)
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: gates %v, want %v", tc.name, got, tc.want)
		}
	}
}
