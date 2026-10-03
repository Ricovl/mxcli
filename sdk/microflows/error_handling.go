// SPDX-License-Identifier: Apache-2.0

package microflows

import "reflect"

var errorHandlingTypeType = reflect.TypeOf(ErrorHandlingType(""))

// ActionErrorHandlingType reads the ErrorHandlingType field off any action that
// declares one, and returns "" for an action that has none.
//
// Mendix stores an action activity's error handling on the ACTION
// (Microflows$JavaActionCallAction.ErrorHandlingType, …), not on the activity:
// the model reader never fills BaseActivity.ErrorHandlingType. Code that reads
// the activity field therefore sees "" for every action — which is how CONV013
// reported an empty handling on handled calls and CONV014 never fired
// (mendixlabs/mxcli#1202).
//
// Reflection rather than a case per action type: 38 action types carry the
// field, and a hand-maintained switch over them drifted to 17 once already
// (mendixlabs/mxcli#1078). Actions embed model.BaseElement, which has no such
// field, so a promoted field cannot be picked up by accident.
func ActionErrorHandlingType(action MicroflowAction) ErrorHandlingType {
	v := reflect.ValueOf(action)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName("ErrorHandlingType")
	if !f.IsValid() || f.Type() != errorHandlingTypeType {
		return ""
	}
	return ErrorHandlingType(f.String())
}

// ObjectErrorHandlingType returns the stored error handling of a flow object:
// the action's for an ActionActivity (falling back to the activity field, which
// only hand-built objects set), and the object's own field for a loop or an
// exclusive split. "" for an object that has none (events, merges,
// annotations, an inheritance split).
func ObjectErrorHandlingType(obj MicroflowObject) ErrorHandlingType {
	switch o := obj.(type) {
	case *ActionActivity:
		if o == nil {
			return ""
		}
		if eh := ActionErrorHandlingType(o.Action); eh != "" {
			return eh
		}
		return o.ErrorHandlingType
	case *LoopedActivity:
		if o == nil {
			return ""
		}
		return o.ErrorHandlingType
	case *ExclusiveSplit:
		if o == nil {
			return ""
		}
		return o.ErrorHandlingType
	}
	return ""
}

// IsCustomErrorHandling reports whether t routes errors to a custom handler
// flow, with or without rollback.
func IsCustomErrorHandling(t ErrorHandlingType) bool {
	return t == ErrorHandlingTypeCustom || t == ErrorHandlingTypeCustomWithoutRollback
}
