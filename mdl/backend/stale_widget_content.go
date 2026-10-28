// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"fmt"
	"strings"
)

// StaleWidgetContentError refuses a REPLACE of a pluggable widget that would
// carry widgets MDL cannot show or author — a combo box's custom content — into
// a replacement whose data source for them is another entity
// (ako/mxcli#1563).
//
// A same-kind REPLACE keeps every stored property the statement does not state
// (mendixlabs/mxcli#1247), and content describe never prints is never stated,
// so it is always kept. That is right while the content's data source keeps
// its entity. Moved onto another one, the kept widgets stay bound to the old
// entity: stored, hidden from describe, and CE1365 once Mendix shows them.
// Dropping them instead would delete Studio Pro-authored content the statement
// has no words for, so the replace is refused (ADR-0012) and the author drops
// them on purpose.
type StaleWidgetContentError struct {
	// Widget is the widget the REPLACE targets.
	Widget string
	// Property holds the content; DataSource is the property it is built for.
	Property, DataSource string
	// StoredEntity is DataSource's entity as stored, NewEntity the replacement's.
	StoredEntity, NewEntity string
	// Widgets are the content's widgets, by name ("" for an unnamed one), with
	// their types in WidgetTypes at the same index.
	Widgets, WidgetTypes []string
}

func (e *StaleWidgetContentError) Error() string {
	var listed, named []string
	for i, n := range e.Widgets {
		if n == "" {
			listed = append(listed, "an unnamed "+e.widgetType(i))
			continue
		}
		listed = append(listed, n)
		named = append(named, n)
	}
	msg := fmt.Sprintf(
		"replace %s: its %s holds %s, which describe does not print and MDL cannot author, built for %s; "+
			"the replacement takes %s from %s, so the widgets would be kept, hidden and still bound to %s "+
			"(Mendix reports CE1365 once they are shown).",
		e.Widget, e.Property, strings.Join(listed, ", "), e.StoredEntity,
		e.DataSource, e.NewEntity, e.StoredEntity)
	if len(named) == len(e.Widgets) {
		msg += fmt.Sprintf(" Drop them first, in the same statement: `drop %s;` before the replace.", strings.Join(named, ", "))
	}
	return msg + fmt.Sprintf(" Or `drop %s` and `insert` the new widget, which builds it from the statement alone.", e.Widget)
}

func (e *StaleWidgetContentError) widgetType(i int) string {
	if i < len(e.WidgetTypes) && e.WidgetTypes[i] != "" {
		return e.WidgetTypes[i]
	}
	return "widget"
}
