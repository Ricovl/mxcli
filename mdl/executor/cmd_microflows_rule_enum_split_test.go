// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1414: "Describe of a merge/decision cannot be re-created".
// OpenAIConnector.EmbeddingsRequestMapping_POST decides on a RULE that returns
// an enumeration (Rule_Configuration_KeyTypeToUse → Bearer_Token / API_key /
// (empty)). DESCRIBE printed it as a boolean `if` with empty branches and an
// orphan `merge shared1;`, which check refuses (MDL-FLOW02 "Every merge needs at
// least one incoming path"). MDL had no way to say it: `case` took only a
// variable or an attribute path. It now takes a rule call too, spelled exactly
// as a rule call in an `if`.
const ruleCaseScript = `create microflow Mod.Route ($Configuration: Mod.Configuration)
returns String
begin
  case Mod.KeyTypeToUse(Configuration = $Configuration)
    when Bearer_Token then
      return 'bearer';
    when API_key then
      return 'key';
    when (empty) then
      return 'none';
  end case;
end;`

func ruleCaseBackend(values ...string) *mock.MockBackend {
	enumValues := make([]model.EnumerationValue, 0, len(values))
	for _, v := range values {
		enumValues = append(enumValues, model.EnumerationValue{Name: v})
	}
	return &mock.MockBackend{
		IsRuleFunc: func(qn string) (bool, error) { return qn == "Mod.KeyTypeToUse", nil },
		ListRulesFunc: func() ([]*microflows.Rule, error) {
			return []*microflows.Rule{{
				BaseElement: model.BaseElement{ID: "rule"},
				ContainerID: "mod",
				Name:        "KeyTypeToUse",
				ReturnType:  &microflows.EnumerationType{EnumerationQualifiedName: "Mod.KeyType"},
			}}, nil
		},
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{{BaseElement: model.BaseElement{ID: "enum"}, ContainerID: "mod", Name: "KeyType", Values: enumValues}}, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			return &model.Module{BaseElement: model.BaseElement{ID: "mod"}, Name: name}, nil
		},
	}
}

func parseRuleCase(t *testing.T, script string) *ast.EnumSplitStmt {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("`case <rule call>` does not parse: %v", errs)
	}
	body := prog.Statements[0].(*ast.CreateMicroflowStmt).Body
	split, ok := body[0].(*ast.EnumSplitStmt)
	if !ok {
		t.Fatalf("first statement is %T, want *ast.EnumSplitStmt", body[0])
	}
	return split
}

func buildRuleCase(t *testing.T, split *ast.EnumSplitStmt, mb *mock.MockBackend) (*flowBuilder, *microflows.MicroflowObjectCollection) {
	t.Helper()
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing, backend: mb,
		measurer:     &layoutMeasurer{varTypes: map[string]string{}},
		varTypes:     map[string]string{"Configuration": "Mod.Configuration"},
		declaredVars: map[string]string{"Configuration": "Mod.Configuration"},
	}
	oc := fb.buildFlowGraph([]ast.MicroflowStatement{split}, nil)
	return fb, oc
}

func TestCaseOnRuleCall_ParsesBuildsAndDescribesBack(t *testing.T) {
	split := parseRuleCase(t, ruleCaseScript)
	if split.Condition == nil {
		t.Fatalf("EnumSplitStmt.Condition is nil; the rule call was dropped (Variable=%q)", split.Variable)
	}

	fb, oc := buildRuleCase(t, split, ruleCaseBackend("Bearer_Token", "API_key"))
	if errs := fb.GetErrors(); len(errs) > 0 {
		t.Fatalf("build errors: %s", strings.Join(errs, "\n"))
	}
	var es *microflows.ExclusiveSplit
	for _, o := range oc.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			es = s
		}
	}
	if es == nil {
		t.Fatal("no ExclusiveSplit built")
	}
	rule, ok := es.SplitCondition.(*microflows.RuleSplitCondition)
	if !ok {
		t.Fatalf("split condition is %T, want *RuleSplitCondition (an expression naming a rule is CE0117)", es.SplitCondition)
	}
	if rule.RuleQualifiedName != "Mod.KeyTypeToUse" || len(rule.ParameterMappings) != 1 ||
		rule.ParameterMappings[0].ParameterName != "Mod.KeyTypeToUse.Configuration" ||
		rule.ParameterMappings[0].Argument != "$Configuration" {
		t.Errorf("rule call = %+v / %+v", rule, rule.ParameterMappings)
	}
	cases := map[string]bool{}
	for _, f := range oc.Flows {
		if v, ok := enumCaseValue(f); ok {
			cases[v] = true
		}
	}
	for _, want := range []string{"Bearer_Token", "API_key", "(empty)"} {
		if !cases[want] {
			t.Errorf("no outgoing flow with case %q (have %v)", want, cases)
		}
	}

	out := strings.Join(formatMicroflowActivities(newTestExecutor().newExecContext(t.Context()), &microflows.Microflow{ObjectCollection: oc}, nil, nil), "\n")
	for _, want := range []string{"case Mod.KeyTypeToUse(Configuration = $Configuration)", "when Bearer_Token then", "when API_key then", "when (empty) then", "end case;"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "if Mod.KeyTypeToUse") || strings.Contains(out, "merge shared") {
		t.Errorf("describe still prints the enumeration rule split as a boolean if:\n%s", out)
	}
}

// Coverage: an enumeration split needs a flow per value (CE0079). Where the
// rule's return enumeration is known, a missing value and a value the
// enumeration does not have are both refused at build time.
func TestCaseOnRuleCall_CoverageAgainstTheReturnEnumeration(t *testing.T) {
	split := parseRuleCase(t, ruleCaseScript)

	fb, _ := buildRuleCase(t, split, ruleCaseBackend("Bearer_Token", "API_key", "Oauth"))
	if errs := strings.Join(fb.GetErrors(), "\n"); !strings.Contains(errs, "Oauth") || !strings.Contains(errs, "CE0079") {
		t.Errorf("a missing enumeration value was not reported as CE0079: %q", errs)
	}

	fb, _ = buildRuleCase(t, split, ruleCaseBackend("Bearer_Token"))
	if errs := strings.Join(fb.GetErrors(), "\n"); !strings.Contains(errs, "API_key") || !strings.Contains(errs, "Mod.KeyType") {
		t.Errorf("a value the enumeration does not have was not reported: %q", errs)
	}

	// A Boolean rule has true/false flows; `case` on it is refused.
	mb := ruleCaseBackend("Bearer_Token")
	mb.ListRulesFunc = func() ([]*microflows.Rule, error) {
		return []*microflows.Rule{{ContainerID: "mod", Name: "KeyTypeToUse", ReturnType: &microflows.BooleanType{}}}, nil
	}
	fb, _ = buildRuleCase(t, split, mb)
	if errs := strings.Join(fb.GetErrors(), "\n"); !strings.Contains(errs, "returns Boolean") {
		t.Errorf("case on a Boolean rule was not refused: %q", errs)
	}

	// Control: a rule whose return enumeration cannot be resolved is not judged.
	mb = ruleCaseBackend("Bearer_Token")
	mb.ListEnumerationsFunc = func() ([]*model.Enumeration, error) { return nil, nil }
	fb, _ = buildRuleCase(t, split, mb)
	if errs := fb.GetErrors(); len(errs) > 0 {
		t.Errorf("unknown return enumeration must not be judged, got %v", errs)
	}
}
