// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// PluggableWidgetState reports what the stored pluggable widget widgetRef
// holds: each property's scalar and the $Type of each property's action. ok is
// false when widgetRef does not resolve to a pluggable widget — a built-in
// widget, a data grid column or an object-list item.
func (m *Mutator) PluggableWidgetState(widgetRef string) (backend.PluggableWidgetState, bool) {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil || result.item != nil || len(result.colPropKeys) > 0 {
		return backend.PluggableWidgetState{}, false
	}
	obj := bsonnav.DGetDoc(result.widget, "Object")
	if obj == nil {
		return backend.PluggableWidgetState{}, false
	}
	keys := buildPropKeyMap(result.widget)
	st := backend.PluggableWidgetState{
		WidgetID: pluggableWidgetID(result.widget),
		Values:   map[string]string{},
		Actions:  map[string]string{},
	}
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		key := strings.ToLower(keys[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))])
		val := bsonnav.DGetDoc(pd, "Value")
		if key == "" || val == nil {
			continue
		}
		for _, field := range []string{"PrimitiveValue", "Expression", "Selection"} {
			if v := bsonnav.DGetString(val, field); v != "" {
				st.Values[key] = v
				break
			}
		}
		if act := bsonnav.DGetDoc(val, "Action"); act != nil {
			st.Actions[key] = bsonnav.DGetString(act, "$Type")
		}
	}
	return st, true
}
