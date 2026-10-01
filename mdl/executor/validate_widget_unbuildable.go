// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A widget keyword the grammar accepts and the page builder has no writer for.
//
// `referenceselector` parses, so `check --references` reported "Check passed!",
// and `exec` then refused the page — after the statements before it had been
// written — with "unsupported widget type: referenceselector — … refresh the
// project's widget definitions: 'mxcli widget init'" (ako/mxcli#563). That hint
// does not apply: the classic reference selector is a built-in Forms widget, not
// a pluggable one, so there is no .mpk and `widget init` cannot help.
//
// formsWidgetsWithoutWriter is the one answer to "can mxcli build this
// keyword?" for these: the builder's fall-through reports with it, and the
// validator refuses with it, so check and exec say the same thing.
// TestFormsWidgetsWithoutWriterAreUnbuilt keeps it honest against the
// builder's dispatch: a keyword that gains a builder case must leave the set.
var formsWidgetsWithoutWriter = map[string]struct {
	storedType  string
	replacement string
}{
	"referenceselector": {
		storedType: "Forms$ReferenceSelector",
		replacement: "use `combobox` over the association instead — it needs its own `datasource:` " +
			"(CE0642 without one) and a caption attribute",
	},
}

// unbuildableWidgetMessage is the refusal for a keyword mxcli has no writer
// for, or "" when it has one (or the keyword is not one of these).
func unbuildableWidgetMessage(widgetType string) string {
	u, ok := formsWidgetsWithoutWriter[strings.ToLower(widgetType)]
	if !ok {
		return ""
	}
	return fmt.Sprintf("`%s` is the built-in Forms widget %s, which mxcli cannot write — it is not a "+
		"pluggable widget, so `mxcli widget init` does not help; %s",
		strings.ToLower(widgetType), u.storedType, u.replacement)
}

// validateUnbuildableWidgetKind refuses (MDL-WIDGET38) a widget keyword the page
// builder would refuse. A project's own widget definition of that MDL name wins,
// exactly as it does in the builder, whose registry lookup comes first.
func validateUnbuildableWidgetKind(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	if w == nil || w.TypeIsGeneric {
		return nil
	}
	msg := unbuildableWidgetMessage(w.Type)
	if msg == "" {
		return nil
	}
	if registry != nil {
		if _, ok := registry.Get(strings.ToUpper(w.Type)); ok {
			return nil
		}
	}
	return []linter.Violation{{
		RuleID:     "MDL-WIDGET38",
		Severity:   linter.SeverityError,
		Message:    fmt.Sprintf("%s: widget `%s`: %s", locationPrefix, w.Name, msg),
		Suggestion: "replace the widget; exec refuses the page otherwise, after the statements before it were written",
	}}
}
