// SPDX-License-Identifier: Apache-2.0

package backend

// PluggableWidgetState is what a page mutator reports about a stored pluggable
// widget, so the executor can judge the widget as a whole after an ALTER … SET
// changed part of it — a Gallery's row click is legal or ambiguous depending
// on three properties at once, and a SET may name only one of them
// (ako/mxcli#854).
type PluggableWidgetState struct {
	WidgetID string
	// Values maps each lower-cased property key to its stored scalar: the
	// PrimitiveValue, else the Expression, else the Selection.
	Values map[string]string
	// Actions maps each lower-cased property key to the $Type of the client
	// action its value holds. Every stored value carries one, a Forms$NoAction
	// by default.
	Actions map[string]string
}
