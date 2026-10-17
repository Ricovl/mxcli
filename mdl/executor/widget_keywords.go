// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// builderOwnedWidgetKeywords are the widget keywords buildWidgetV3 builds with
// its own switch case, before (and instead of) any widget-registry lookup.
// `image` is absent on purpose: its case resolves through the registry.
//
// The registry must never answer for one of these names: check resolves a
// keyword through the registry (lookupWidgetDef), so a definition registered
// under `radiobuttons` made check validate a pluggable widget while exec wrote
// a Forms$RadioButtonGroup (mendixlabs/mxcli#1463).
// TestBuilderOwnedWidgetKeywordsMatchSwitch keeps this in step with the switch.
var builderOwnedWidgetKeywords = map[string]bool{
	"dataview": true, "legacydatagrid": true, "listview": true, "layoutgrid": true,
	"row": true, "column": true, "container": true, "customcontainer": true,
	"slot": true, "textbox": true, "textarea": true, "datepicker": true,
	"dropdown": true, "checkbox": true, "filemanager": true, "text": true,
	"statictext": true, "dynamictext": true, "title": true, "label": true,
	"button": true, "actionbutton": true, "linkbutton": true, "tabcontainer": true,
	"tabpage": true, "groupbox": true, "scrollcontainer": true, "region": true,
	"navigationtree": true, "menubar": true, "simplemenubar": true, "sidebartoggle": true,
	"placeholder": true, "radiobuttons": true, "navigationlist": true, "item": true,
	"snippetcall": true, "footer": true, "header": true, "controlbar": true,
	"template": true, "filter": true, "staticimage": true, "dynamicimage": true,
}

// isBuilderOwnedWidgetKeyword reports whether keyword is built by the page
// builder's own switch rather than through the widget registry.
func isBuilderOwnedWidgetKeyword(keyword string) bool {
	return builderOwnedWidgetKeywords[strings.ToLower(keyword)]
}
