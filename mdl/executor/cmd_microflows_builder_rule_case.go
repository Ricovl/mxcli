// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// checkRuleCaseCoverage judges a `case Mod.Rule(…)` split against the
// enumeration the rule returns, when the project says what that is.
//
// An enumeration split needs an outgoing flow for every value of its
// enumeration — mxbuild reports a missing one as CE0079 "The 'X' condition
// value should be configured in properties for an outgoing flow" — and a case
// naming a value the enumeration does not have can never be taken. Where the
// rule or its enumeration cannot be resolved (no project, a rule created by the
// same script, an ambiguous name) nothing is said: guessing the member list
// would trade one false positive for another (mendixlabs/mxcli#1414).
func (fb *flowBuilder) checkRuleCaseCoverage(s *ast.EnumSplitStmt) {
	call := unwrapParenCall(s.Condition)
	if call == nil || fb.backend == nil {
		return
	}
	enumQN, values := fb.ruleReturnEnumeration(call.Name)
	if enumQN == ruleReturnsBoolean {
		fb.addError("case %s: the rule returns Boolean, and a Boolean decision has true/false flows, not "+
			"enumeration cases — write `if %s(…) then … else … end if;`", call.Name, call.Name)
		return
	}
	if enumQN == "" {
		return
	}
	members := make(map[string]bool, len(values))
	for _, v := range values {
		members[v] = true
	}
	covered := map[string]bool{}
	for _, c := range s.Cases {
		for _, v := range enumSplitCaseValues(c) {
			if v == "(empty)" {
				continue
			}
			covered[v] = true
			if !members[v] {
				fb.addError("case %s: when %s — %s has no value %s (its values: %s)",
					call.Name, v, enumQN, v, strings.Join(values, ", "))
			}
		}
	}
	var missing []string
	for _, v := range values {
		if !covered[v] {
			missing = append(missing, v)
		}
	}
	if len(missing) > 0 {
		fb.addError("case %s: no branch for %s of %s — every value needs an outgoing flow, or mxbuild reports CE0079 "+
			"\"The '%s' condition value should be configured in properties for an outgoing flow\"; add `when %s then …`",
			call.Name, strings.Join(missing, ", "), enumQN, missing[0], strings.Join(missing, ", "))
	}
}

// ruleReturnsBoolean is ruleReturnEnumeration's answer for a Boolean rule.
const ruleReturnsBoolean = "\x00boolean"

// ruleReturnEnumeration returns the qualified name and member names of the
// enumeration rule qn returns, or "" when the rule is not found, returns
// something else, or either name is ambiguous.
func (fb *flowBuilder) ruleReturnEnumeration(qn string) (string, []string) {
	module, name, ok := strings.Cut(qn, ".")
	if !ok {
		return "", nil
	}
	rules, err := fb.backend.ListRules()
	if err != nil {
		return "", nil
	}
	modID := model.ID("")
	if mod, err := fb.backend.GetModuleByName(module); err == nil && mod != nil {
		modID = mod.ID
	}
	var named []*microflows.Rule
	for _, r := range rules {
		if r != nil && r.Name == name {
			named = append(named, r)
		}
	}
	rule := pickByContainer(named, modID, func(r *microflows.Rule) model.ID { return r.ContainerID })
	if rule == nil {
		return "", nil
	}
	if _, isBool := rule.ReturnType.(*microflows.BooleanType); isBool {
		return ruleReturnsBoolean, nil
	}
	et, ok := rule.ReturnType.(*microflows.EnumerationType)
	if !ok || et == nil || et.EnumerationQualifiedName == "" {
		return "", nil
	}
	enumModule, enumName, ok := strings.Cut(et.EnumerationQualifiedName, ".")
	if !ok {
		return "", nil
	}
	enums, err := fb.backend.ListEnumerations()
	if err != nil {
		return "", nil
	}
	enumModID := model.ID("")
	if mod, err := fb.backend.GetModuleByName(enumModule); err == nil && mod != nil {
		enumModID = mod.ID
	}
	var named2 []*model.Enumeration
	for _, e := range enums {
		if e != nil && e.Name == enumName {
			named2 = append(named2, e)
		}
	}
	match := pickByContainer(named2, enumModID, func(e *model.Enumeration) model.ID { return e.ContainerID })
	if match == nil {
		return "", nil
	}
	values := make([]string, 0, len(match.Values))
	for _, v := range match.Values {
		values = append(values, v.Name)
	}
	return et.EnumerationQualifiedName, values
}

// pickByContainer resolves a same-named candidate list: the only one, or the
// only one stored directly in the module. Anything else is ambiguous, and an
// ambiguous name is reported as unknown (nil) rather than guessed.
func pickByContainer[T any](cands []*T, moduleID model.ID, container func(*T) model.ID) *T {
	if len(cands) == 1 {
		return cands[0]
	}
	var found *T
	for _, c := range cands {
		if moduleID != "" && container(c) == moduleID {
			if found != nil {
				return nil
			}
			found = c
		}
	}
	return found
}

// ruleCaseCoverageErrors is checkRuleCaseCoverage for `mxcli check -p`, which
// validates a new flow's body without building it: the same judgement against
// the project's rules and enumerations, so check and exec agree.
func ruleCaseCoverageErrors(ctx *ExecContext, body []ast.MicroflowStatement) []string {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	fb := &flowBuilder{backend: ctx.Backend}
	walkFlowStatements(body, func(st ast.MicroflowStatement) {
		if s, ok := st.(*ast.EnumSplitStmt); ok && s.Condition != nil {
			fb.checkRuleCaseCoverage(s)
		}
	})
	return fb.GetErrors()
}
