// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// A combo box's custom content (ako/mxcli#1744).
//
// Custom content is how a combo box shows more than one attribute per option:
// a widgets-typed property per option source — optionsSourceAssociationCustomContent,
// optionsSourceDatabaseCustomContent — rendered once per option, so its
// widgets bind to the OPTION entity, not to the object the combo box edits.
// Each belongs to one mode of the definition (its `childSlots`), which is what
// makes the block writable and the other mode's block a refusal (MDL-WIDGET30).
//
// describe prints the content of the mode it describes the combo box in, as the
// block that writes it, so a describe → exec round trip restates it. It used to
// print only the content TYPE and a NOT SHOWN note: the passthrough kept the
// content of an unchanged combo box, and a changed one was refused
// (ako/mxcli#1648) — the content could not be restated, so the rebuild would
// have deleted it.

// comboBoxDescribedMode is the definition's mode a described combo box
// re-executes in: database mode when its own datasource is printed under its
// key, association mode when the generic `DataSource:` clause is printed, and
// none otherwise — an enumeration combo box, or one whose option list is not
// set, which exec cannot put back in the stored mode.
func comboBoxDescribedMode(widget *rawWidget, database bool) *WidgetMode {
	name := ""
	switch {
	case database && len(widget.NamedDataSources) > 0:
		name = "database"
	case !database && widget.DataSource != nil:
		name = "association"
	default:
		return nil
	}
	def := comboBoxDefinition()
	if def == nil {
		return nil
	}
	for i := range def.Modes {
		if def.Modes[i].Name == name {
			return &def.Modes[i]
		}
	}
	return nil
}

// comboBoxCustomContent reads the stored content of the mode's slots, under
// the keyword the definition declares for them; optionEntity is the entity of
// the mode's option list, which the content's widgets bind to.
func comboBoxCustomContent(ctx *ExecContext, w map[string]any, mode *WidgetMode, optionEntity string) []rawChildSlot {
	if mode == nil || len(mode.ChildSlots) == 0 {
		return nil
	}
	var out []rawChildSlot
	for _, cs := range extractChildSlots(ctx, w, optionEntity) {
		for _, slot := range mode.ChildSlots {
			if strings.EqualFold(cs.PropertyKey, slot.PropertyKey) {
				cs.Keyword = strings.ToLower(slot.MDLContainer)
				out = append(out, cs)
			}
		}
	}
	return out
}

// withoutDescribedContent drops from hidden the content printed as a block.
func withoutDescribedContent(hidden []rawHiddenContent, printed []rawChildSlot) []rawHiddenContent {
	if len(printed) == 0 {
		return hidden
	}
	var out []rawHiddenContent
	for _, hc := range hidden {
		described := false
		for _, cs := range printed {
			if strings.EqualFold(cs.PropertyKey, hc.Property) {
				described = true
				break
			}
		}
		if !described {
			out = append(out, hc)
		}
	}
	return out
}
