// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// WidgetSlotKeywordOverrides maps (widgetID, propertyKey) pairs to the MDL
// keyword used in a widget body to fill that property. Most widgets[]-typed
// properties use the uppercase property key as their MDL keyword; entries
// here cover the cases where the keyword is a different conventional word.
//
// Background: a widget's .mpk declares property keys (e.g. filtersPlaceholder)
// but not the MDL keyword users type for that slot. Studio Pro authors think
// of `controlbar { ... }` (DataGrid) and `filter { ... }` (Gallery) rather
// than `filtersplaceholder { ... }`.
//
// It lives here, like ItemPropertyAliases, because two code paths must agree on
// it: the create path and describe (mdl/executor), which spell a slot with this
// keyword, and the page mutator, which resolves the same keyword when ALTER
// PAGE addresses the slot as `<widget>.<keyword>`.
var WidgetSlotKeywordOverrides = map[string]map[string]string{
	DataGridWidgetID: {
		"filtersPlaceholder": "CONTROLBAR",
	},
	"com.mendix.widget.web.gallery.Gallery": {
		"filtersPlaceholder": "FILTER",
	},
}

// WidgetSlotKeyword returns the MDL keyword for a widgets-typed property.
// Defaults to the uppercase property key; widget-specific entries in
// WidgetSlotKeywordOverrides win over the global `content` → `TEMPLATE`
// convention.
func WidgetSlotKeyword(widgetID, propertyKey string) string {
	if widgetSpecific, ok := WidgetSlotKeywordOverrides[widgetID]; ok {
		if kw, ok := widgetSpecific[propertyKey]; ok {
			return kw
		}
	}
	if propertyKey == "content" {
		return "TEMPLATE"
	}
	return strings.ToUpper(propertyKey)
}
