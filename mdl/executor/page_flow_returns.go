// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// actionReturnValue is how Studio Pro's "Return value mappings" table names
// the flow's result, and how `returns ( … )` reads it.
const actionReturnValue = "$ActionReturnValue"

// applyFlowReturns builds a flow call's `returns ( … )` list into the action's
// output mappings: Forms$OutputMapping, Mendix 11.5 (ako/mxcli#977 item 3).
//
// Each entry is target = source, and which target and which source it names
// decide the stored slots. Measured on ako/TestApp's Pages.OutputMappingButton
// (Studio Pro 11.15), one row of each kind:
//
//	Brand = $ActionReturnValue/Brand     AttributeRef + SourceAttributeRef
//	$stringResult = $ActionReturnValue   SourceVariable alone
//	Brand = 'Demo brand'                 AttributeRef + Expression
//
// An attribute target belongs to the data container's object, a `$name` target
// is a page variable. The return value itself fills no source slot; one of its
// attributes is SourceAttributeRef, qualified with the flow's return entity;
// anything else is an Expression.
//
// The attribute target has its own, later floor: mx 11.13 and 11.14 report
// CE7006 on the same bytes 11.15 accepts (measured; sdk/versions/mendix-11.yaml).
//
// Every target and source is checked here because nothing later will: the
// mapping is stored by name, and a wrong one is a valid-looking page that maps
// nothing.
func (pb *pageBuilder) applyFlowReturns(a pages.ClientAction, returns []ast.FlowReturnV3) error {
	var flow, returnEntity string
	var out *[]*pages.FlowOutputMapping
	switch x := a.(type) {
	case *pages.MicroflowClientAction:
		flow, out = x.MicroflowName, &x.OutputMappings
		returnEntity = pb.getMicroflowReturnEntityName(flow)
	case *pages.NanoflowClientAction:
		flow, out = x.NanoflowName, &x.OutputMappings
		returnEntity = pb.getNanoflowReturnEntityName(flow)
	default:
		return mdlerrors.NewValidation("returns ( … ) maps a flow's result back onto the page: it belongs to `call microflow` and `call nanoflow` only")
	}
	if err := pb.checkGate(flowReturnsGate); err != nil {
		return err
	}
	index := pb.attributeIndex()
	for _, r := range returns {
		m := &pages.FlowOutputMapping{}
		if r.Variable {
			if !pb.localVariables[r.Target] {
				return mdlerrors.NewValidationf("returns: $%s is not a variable of this page — "+
					"a mapping writes into a page variable (Variables: ( $%s: … )) or an attribute of the data container's object", r.Target, r.Target)
			}
			m.TargetVariable = r.Target
		} else {
			if err := pb.checkGate(flowReturnsToAttributeGate); err != nil {
				return err
			}
			if pb.entityContext == "" {
				return mdlerrors.NewValidationf("returns: %s = …: an attribute target needs the widget inside a data container, "+
					"whose object it writes; outside one, write into a page variable ($name = …)", r.Target)
			}
			if found, known := index.declares(pb.entityContext, r.Target); known && !found {
				return mdlerrors.NewValidationf("returns: %s has no attribute %s", pb.entityContext, r.Target)
			}
			m.TargetAttribute = pb.resolveAttributePath(r.Target)
		}
		src := strings.TrimSpace(r.Source)
		if attr, ok := returnValueAttribute(src); ok {
			if returnEntity == "" {
				return mdlerrors.NewValidationf("returns: %s returns no object, so %s/%s has nothing to read — "+
					"map %s itself, or an expression", flow, actionReturnValue, attr, actionReturnValue)
			}
			if found, known := index.declares(returnEntity, attr); known && !found {
				return mdlerrors.NewValidationf("returns: %s returns a %s, which has no attribute %s", flow, returnEntity, attr)
			}
			m.SourceAttribute = pb.resolveAttributePathForEntity(attr, returnEntity)
		} else if !strings.EqualFold(src, actionReturnValue) {
			m.Expression = src
		}
		*out = append(*out, m)
	}
	return nil
}

// checkGate applies one of the statement's version gates again at build time,
// for widgets the statement-level walk does not see (a fragment's).
func (pb *pageBuilder) checkGate(g versionGate) error {
	if pb.ctx == nil {
		return nil
	}
	return checkGate(pb.ctx, g)
}

// returnValueAttribute splits `$ActionReturnValue/Attr` into Attr. Anything
// longer — a path over an association, an operator — is an expression.
func returnValueAttribute(src string) (string, bool) {
	if len(src) <= len(actionReturnValue)+1 || !strings.EqualFold(src[:len(actionReturnValue)+1], actionReturnValue+"/") {
		return "", false
	}
	attr := unquoteIdentifier(src[len(actionReturnValue)+1:])
	for _, c := range attr {
		if c != '_' && !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9') {
			return "", false
		}
	}
	return attr, attr != ""
}

// formatFlowReturnsV3 renders a parsed `returns ( … )` list as written, ""
// when there is none.
func formatFlowReturnsV3(returns []ast.FlowReturnV3) string {
	if len(returns) == 0 {
		return ""
	}
	parts := make([]string, 0, len(returns))
	for _, r := range returns {
		target := visitor.ParameterNameSpelling(r.Target)
		if r.Variable {
			target = "$" + r.Target
		}
		parts = append(parts, target+" = "+r.Source)
	}
	return " returns (" + strings.Join(parts, ", ") + ")"
}

// storedFlowReturns reads a flow call's stored output mappings back as the
// `returns ( … )` that builds them; holder is a microflow call's
// MicroflowSettings or the nanoflow call itself. "" with ok is a call without
// mappings. ok is false when a row has a shape this syntax does not write — a
// path over an association, a target in both slots, an expression that reads
// like the shorthand — and then the caller prints none of them and says so,
// rather than describe a different mapping.
func storedFlowReturns(holder map[string]any) (string, bool) {
	var parts []string
	for _, e := range getBsonArrayElements(holder["OutputMappings"]) {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		part, ok := storedFlowReturn(m)
		if !ok {
			return "", false
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", true
	}
	return " returns (" + strings.Join(parts, ", ") + ")", true
}

// storedFlowReturn is one Forms$OutputMapping as `Target = Source`.
func storedFlowReturn(m map[string]any) (string, bool) {
	targetAttr, ok := plainAttributeRef(m, "AttributeRef")
	if !ok {
		return "", false
	}
	sourceAttr, ok := plainAttributeRef(m, "SourceAttributeRef")
	if !ok {
		return "", false
	}
	targetVar := ""
	if sv := actionMapForKey(m, "SourceVariable"); sv != nil {
		targetVar = extractString(sv["LocalVariable"])
		if useAll, _ := sv["UseAllPages"].(bool); targetVar == "" || useAll ||
			extractString(sv["PageParameter"]) != "" || extractString(sv["SnippetParameter"]) != "" ||
			extractString(sv["Widget"]) != "" || extractString(sv["SubKey"]) != "" {
			return "", false
		}
	}
	expr := strings.TrimSpace(extractString(m["Expression"]))

	var target, source string
	switch {
	case targetVar != "" && targetAttr == "":
		target = "$" + targetVar
	case targetAttr != "" && targetVar == "":
		target = visitor.ParameterNameSpelling(shortAttributeName(targetAttr))
	default:
		return "", false
	}
	switch {
	case sourceAttr != "" && expr != "":
		return "", false
	case sourceAttr != "":
		source = actionReturnValue + "/" + mdlIdent(shortAttributeName(sourceAttr))
	case expr != "":
		// Built again, an expression that reads like the shorthand would land
		// in the shorthand's slot instead of this one.
		if _, ok := returnValueAttribute(expr); ok || strings.EqualFold(expr, actionReturnValue) {
			return "", false
		}
		source = expr
	default:
		source = actionReturnValue
	}
	return target + " = " + source, true
}

// plainAttributeRef is the qualified attribute of m[key], "" when the slot is
// null; ok is false for a reference this syntax cannot write — one over an
// association (an EntityRef beside it) or one that is not qualified.
func plainAttributeRef(m map[string]any, key string) (string, bool) {
	ref := actionMapForKey(m, key)
	if ref == nil {
		return "", true
	}
	attr := extractString(ref["Attribute"])
	if actionMapForKey(ref, "EntityRef") != nil || strings.Count(attr, ".") < 2 {
		return "", false
	}
	return attr, true
}

// unsayableOutputMappings counts the output mappings of a widget's flow call
// when describe could not print them as `returns ( … )` — 0 when there are
// none or when it could.
func unsayableOutputMappings(action any) int {
	a, ok := action.(map[string]any)
	if !ok {
		return 0
	}
	holder := a
	if ms, ok := a["MicroflowSettings"].(map[string]any); ok {
		holder = ms
	}
	if _, ok := storedFlowReturns(holder); ok {
		return 0
	}
	n := 0
	for _, e := range getBsonArrayElements(holder["OutputMappings"]) {
		if _, ok := e.(map[string]any); ok {
			n++
		}
	}
	return n
}
