// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// The value of a pluggable widget's enumeration property (ako/mxcli#854).
//
// A widget reads its enumeration keys exactly. The Gallery runs its row click
// for `"single" === clickTrigger` and `"double" === clickTrigger` (Gallery 3.x
// runtime bundle), so `onClickTrigger: Double` — stored as written — matched
// neither and the row action never ran on a click; a data grid's
// `pagination: 'Buttons'` was the same. `mx check` reports 0 errors for both
// (measured on 11.13.0): it does not compare the value with the keys either.
//
// So a value that names a declared key in another case is written as the key,
// under every language version — the script says what it means, the write was
// wrong. A value that names no key is a new rejection and gated (ADR-0011):
// refused under `mdl 1;` as MDL-WIDGET08, the rule an object-list item's
// enumeration already has, and written as given with the MDL-V1-WIDGETENUM
// warning under mdl 0.

// declaredEnumKey matches value against keys: the key itself when one matches
// exactly or in another case, ok false when none does. With no keys (not an
// enumeration, or a schema that cannot say) the value is returned unchanged.
func declaredEnumKey(keys []string, value string) (string, bool) {
	if len(keys) == 0 || value == "" {
		return value, true
	}
	for _, k := range keys {
		if k == value {
			return k, true
		}
	}
	for _, k := range keys {
		if strings.EqualFold(k, value) {
			return k, true
		}
	}
	return value, false
}

// undeclaredEnumMessage words the finding for every reader: the engine, check
// and ALTER.
func undeclaredEnumMessage(where, key, value string, keys []string) string {
	return fmt.Sprintf("%s property `%s` has invalid value `%s` — valid values are %s; "+
		"the widget compares its keys exactly and would ignore it", where, key, value, strings.Join(keys, ", "))
}

// enumerationValue resolves the value the engine writes into propKey: the
// declared key it names, or — for a value that names none — an error under
// mdl 1 and the value with a warning under mdl 0.
func (e *PluggableWidgetEngine) enumerationValue(builder backend.WidgetObjectBuilder, def *WidgetDefinition, w *ast.WidgetV3, propKey, value string) (string, error) {
	declarer, ok := builder.(backend.WidgetEnumerationDeclarer)
	if !ok {
		return value, nil
	}
	keys := declarer.EnumerationKeys(propKey)
	key, ok := declaredEnumKey(keys, value)
	if ok {
		return key, nil
	}
	var ctx *ExecContext
	if e.pageBuilder != nil {
		ctx = e.pageBuilder.ctx
	}
	// A key the definition maps with a constant and the script names is
	// written twice — by the mapping pass and by the explicit-property pass —
	// so it is reported once.
	seen := w.Name + "\x00" + propKey + "\x00" + value
	if e.enumReported[seen] && ctx != nil && !widgetEnumRefused.Applies(ctx.LanguageVersion) {
		return value, nil
	}
	if e.enumReported == nil {
		e.enumReported = map[string]bool{}
	}
	e.enumReported[seen] = true
	where := fmt.Sprintf("widget `%s` (%s)", w.Name, strings.ToLower(def.MDLName))
	return gateUndeclaredEnum(ctx, undeclaredEnumMessage(where, propKey, value, keys), value)
}

// gateUndeclaredEnum applies MDL-V1-WIDGETENUM: an error under mdl 1, the value
// unchanged and a warning under mdl 0.
func gateUndeclaredEnum(ctx *ExecContext, msg, value string) (string, error) {
	version := langver.V0
	if ctx != nil {
		version = ctx.LanguageVersion
	}
	if widgetEnumRefused.Applies(version) {
		return "", mdlerrors.NewValidationf("%s — nothing was written [MDL-WIDGET08]", msg)
	}
	if ctx != nil {
		fmt.Fprintf(ctx.progress(), "Warning [%s]: %s. %s\n", widgetEnumRefused.Code, msg, widgetEnumRefused.Warning(version))
	}
	return value, nil
}

// validatePluggableEnumValues is the check-time twin of enumerationValue: a
// value an enumeration property of the installed widget declares no key for
// (MDL-WIDGET08 under mdl 1, MDL-V1-WIDGETENUM under mdl 0 — gated by
// GateWidgetViolations). Only with a project: the keys are the installed
// package's, and a check without one cannot know a newer widget's keys, so it
// says nothing rather than refuse a value the installed widget may declare.
func validatePluggableEnumValues(w *ast.WidgetV3, def *WidgetDefinition, locationPrefix string) []linter.Violation {
	if w == nil || def == nil || len(def.enumKeys) == 0 {
		return nil
	}
	var out []linter.Violation
	for _, key := range sortedPropertyKeys(w) {
		value, ok := w.Properties[key].(string)
		if !ok || value == "" {
			continue // only a plain value names a key; other shapes have their own rules
		}
		target := enumTargetKey(def, key)
		keys := def.enumKeys[strings.ToLower(target)]
		if len(keys) == 0 {
			continue
		}
		if _, ok := declaredEnumKey(keys, value); ok {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   widgetEnumRefused.Code,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("%s: %s", locationPrefix,
				undeclaredEnumMessage(widgetLabel(w.Name, def.MDLName), target, value, keys)),
		})
	}
	return out
}

// enumTargetKey is the widget property an authored key writes a plain value
// into: the property of the primitive mapping read under that name (its source,
// key or alias), else — the explicit-property pass — the key itself. "" when
// the definition's modes route the name to different properties, or when it is
// an MDL keyword no primitive mapping reads.
func enumTargetKey(def *WidgetDefinition, authored string) string {
	sets := [][]PropertyMapping{def.PropertyMappings}
	for _, m := range def.Modes {
		sets = append(sets, m.PropertyMappings)
	}
	target := ""
	for _, set := range sets {
		for _, m := range set {
			if m.Operation != "primitive" || !mappingReadsName(m, authored) {
				continue
			}
			if target != "" && !strings.EqualFold(target, m.PropertyKey) {
				return ""
			}
			target = m.PropertyKey
		}
	}
	if target != "" || isBuiltinPropName(authored) {
		return target
	}
	return authored
}

// mappingReadsName reports whether m is authored under name.
func mappingReadsName(m PropertyMapping, name string) bool {
	if strings.EqualFold(m.Source, name) || strings.EqualFold(m.PropertyKey, name) {
		return true
	}
	for _, a := range m.MdlAliases {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}
