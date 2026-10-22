// SPDX-License-Identifier: Apache-2.0

package backend

// ObjectListItemValues is what a page mutator reports about a stored
// object-list item addressed as `owner item…` (an Accordion group), so the
// executor can evaluate the widget's visibility rules against the values the
// item actually holds. Item and Widget map lower-cased property keys to their
// stored primitive or expression value.
type ObjectListItemValues struct {
	WidgetID string
	ListKey  string
	// BodySlot is the property key of the item's body slot (BodyItemSlotKey).
	BodySlot string
	Item     map[string]string
	Widget   map[string]string
}
