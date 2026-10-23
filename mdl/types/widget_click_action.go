// SPDX-License-Identifier: Apache-2.0

package types

// WidgetClickActionKeys are, in priority order, the keys under which a built-in
// widget stores its on-click action. The metamodel spells it per type:
//
//   - Action — the buttons (Forms$ActionButton and its data view, grid and
//     drop-down item variants) and a navigation list item
//   - OnClickAction — Forms$DivContainer
//   - ClickAction — Forms$ListView, Forms$StaticImageViewer,
//     Forms$DynamicImageViewer
//
// No type declares two of them, so the first key a stored widget holds is its
// click action. Two readers need the list and must not disagree: the catalog,
// which reports a widget's primary action, and ALTER PAGE's `set (Action: …)`,
// which wrote only Action and refused every container (ako/mxcli#1503).
var WidgetClickActionKeys = []string{"Action", "OnClickAction", "ClickAction"}
