// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// maskedMenuActionKeys are the properties of a menu item's client action that
// the action expression cannot spell: a show page's title override, the number
// of pages a page action closes, and an open link's link type. A rewrite that
// builds the same action otherwise keeps the stored one, so these survive
// (ako/mxcli#980) — describe flags each with a comment.
var maskedMenuActionKeys = map[string]bool{
	"TitleOverride":         true,
	"NumberOfPagesToClose2": true,
	"LinkType":              true,
}

// menuActionBSON is the BSON a menu item stores for action, a client action
// built from MDL. It goes through the widget client-action serializer, so a
// menu item's action has the shape a button's has — which is the shape Studio
// Pro gives both (measured on ako/TestApp's navigation: Forms$FormAction,
// Forms$MicroflowAction, Forms$CallNanoflowClientAction, Forms$SignOutClientAction,
// Forms$NoAction).
//
// stored is the action of the item this one replaces. When the built action
// equals it except for $IDs and maskedMenuActionKeys, stored is returned
// verbatim: everything MDL says about the action is unchanged, and what it
// cannot say is kept.
func menuActionBSON(action any, stored []byte) ([]byte, error) {
	a, ok := action.(pages.ClientAction)
	if !ok {
		return nil, fmt.Errorf("menu item action: %T is not a client action", action)
	}
	el, err := clientActionToGen(a)
	if err != nil {
		return nil, err
	}
	built, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return nil, fmt.Errorf("menu item action: encode: %w", err)
	}
	if len(stored) > 0 && sameMenuAction(built, stored) {
		return stored, nil
	}
	return built, nil
}

// sameMenuAction reports whether two client-action documents are equal apart
// from element $IDs and maskedMenuActionKeys.
func sameMenuAction(a, b []byte) bool {
	var da, db bson.D
	if bson.Unmarshal(a, &da) != nil || bson.Unmarshal(b, &db) != nil {
		return false
	}
	return reflect.DeepEqual(menuActionComparable(da), menuActionComparable(db))
}

// menuActionComparable is v as plain maps and slices, without $ID and the
// masked keys, so key order and element identity do not count.
func menuActionComparable(v any) any {
	switch t := v.(type) {
	case bson.D:
		m := make(map[string]any, len(t))
		for _, e := range t {
			if e.Key == "$ID" || maskedMenuActionKeys[e.Key] {
				continue
			}
			m[e.Key] = menuActionComparable(e.Value)
		}
		return m
	case bson.A:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = menuActionComparable(e)
		}
		return out
	case primitive.Binary:
		return string(t.Data)
	case int32:
		return int64(t)
	default:
		return v
	}
}
