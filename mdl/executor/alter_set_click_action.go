// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `ALTER PAGE … set (Action: …) on <pluggable widget>` (ako/mxcli#854).
//
// A built-in widget keeps its click action in a property of its own
// (Action / OnClickAction / ClickAction), which SetWidgetAction writes. A
// pluggable widget has none: its click is the action-typed property its
// definition reads `onClick:` from — a Gallery's or a data grid's `onClick`.
// So `set (Action: …) on gallery1` was refused ("has no on-click action")
// while `set ('onClick': …)` wrote the very same slot, and the keyword
// `create page` documents for it worked only in its quoted, storage-key form.

// pluggableStateReader is the optional capability the BSON page mutator offers:
// the values a stored pluggable widget holds. An engine without it (the MCP
// mutator) keeps the built-in route, and its own refusal.
type pluggableStateReader interface {
	PluggableWidgetState(widgetRef string) (backend.PluggableWidgetState, bool)
}

// isClickActionKey reports whether a SET key is MDL's click keyword — `Action`
// or its alias `onClick`, in any case, as `create page` reads them.
func isClickActionKey(name string) bool {
	src, ok := actionKeywordSource(name)
	return ok && src == "OnClick"
}

// pluggableClickSlot returns the stored key of a pluggable widget's click slot:
// the one action-typed property the generic matcher reads `onClick:` from.
// pluggable is false for a built-in widget, or when the mutator cannot tell;
// slot is "" for a pluggable widget with no single click slot.
func pluggableClickSlot(mutator backend.PageMutator, widgetRef string) (slot string, pluggable bool) {
	typer, ok := mutator.(widgetStorageTyper)
	if !ok {
		return "", false
	}
	if bsonType, _ := typer.WidgetStorageType(widgetRef); bsonType != "CustomWidgets$CustomWidget" {
		return "", false
	}
	reader, ok := mutator.(pluggableStateReader)
	if !ok {
		return "", true
	}
	st, ok := reader.PluggableWidgetState(widgetRef)
	if !ok {
		return "", true
	}
	var found []string
	for key := range st.Actions {
		if actionSourceForKey(key) == "OnClick" {
			found = append(found, key)
		}
	}
	if len(found) != 1 {
		return "", true
	}
	return found[0], true
}

// setClickAction writes a SET's click action: into a pluggable widget's click
// slot, or a built-in widget's own click property. A pluggable widget without
// a click slot is sent to the named-slot setter under the author's key, which
// refuses it naming the slots the widget does have.
func setClickAction(mutator backend.PageMutator, widgetRef, propName string, action pages.ClientAction) error {
	if slot, pluggable := pluggableClickSlot(mutator, widgetRef); pluggable {
		if slot == "" {
			slot = propName
		}
		return mutator.SetWidgetNamedAction(widgetRef, slot, action)
	}
	return mutator.SetWidgetAction(widgetRef, action)
}

// touchesGalleryClick reports whether a SET changes one of the three settings
// that make a Gallery's row click legal or ambiguous: the click itself, the
// selection, the click trigger.
func touchesGalleryClick(op *ast.SetPropertyOp) bool {
	if op.Target.Widget == "" || op.Target.IsColumn() || op.Target.Item != nil {
		return false
	}
	for name := range op.Properties {
		switch strings.ToLower(name) {
		case "action", "onclick", "selection", "itemselection", "onclicktrigger":
			return true
		}
	}
	return false
}

// storedGalleryClickAmbiguity judges a stored Gallery as validateGalleryClickAmbiguity
// judges a statement's, after a SET has been applied: the Gallery's own editor
// check (Gallery 3.x editorConfig) is `"None" !== itemSelection && onClick &&
// "single" === onClickTrigger`, which `mx check` reports as "The item click
// action is ambiguous". Returns the violation message, or "" when the gallery is
// legal or is not a gallery.
func storedGalleryClickAmbiguity(mutator backend.PageMutator, label, widgetRef string) string {
	reader, ok := mutator.(pluggableStateReader)
	if !ok {
		return ""
	}
	st, ok := reader.PluggableWidgetState(widgetRef)
	if !ok || st.WidgetID != pages.WidgetIDGallery {
		return ""
	}
	if act := st.Actions["onclick"]; act == "" || act == "Forms$NoAction" {
		return ""
	}
	selection := st.Values["itemselection"]
	if selection == "None" || st.Values["onclicktrigger"] != "single" {
		return ""
	}
	return fmt.Sprintf(
		"%s: gallery %s has an onClick and a selection (%s) on the single-click trigger — the gallery "+
			"cannot tell a click from a selection, and the build fails with \"The item click action is ambiguous\"",
		label, widgetRef, selection)
}

// galleryClickSuggestion is what to write instead, for an ALTER. A SET cannot
// write a selection (it has no plain-value form), so dropping the selection
// takes a `replace`.
const galleryClickSuggestion = "add onClickTrigger: 'double' to the set to run the action on a double click, " +
	"or `replace` the gallery with Selection: None"

// checkGalleryClickAfterSet applies MDL-WIDGET36 to an ALTER … SET that changes
// a Gallery's click, selection or trigger: refused under `mdl 1;` — before the
// document is saved, so nothing is written — and written with the
// MDL-V1-GALLERYCLICK warning under mdl 0, as CREATE does.
func checkGalleryClickAfterSet(ctx *ExecContext, mutator backend.PageMutator, op *ast.SetPropertyOp) error {
	if !touchesGalleryClick(op) {
		return nil
	}
	msg := storedGalleryClickAmbiguity(mutator, "set on "+op.Target.Name(), op.Target.Widget)
	if msg == "" {
		return nil
	}
	if galleryClickRefused.Applies(ctx.LanguageVersion) {
		return mdlerrors.NewValidationf("%s — nothing was written; %s [MDL-WIDGET36]", msg, galleryClickSuggestion)
	}
	fmt.Fprintf(ctx.progress(), "Warning [%s]: %s; %s. %s\n",
		galleryClickRefused.Code, msg, galleryClickSuggestion, galleryClickRefused.Warning(ctx.LanguageVersion))
	return nil
}
