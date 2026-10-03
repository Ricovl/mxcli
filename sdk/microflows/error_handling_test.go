// SPDX-License-Identifier: Apache-2.0

package microflows

import "testing"

func TestObjectErrorHandlingTypeReadsTheAction(t *testing.T) {
	// Reader-shaped: the activity field empty, the handling on the action.
	act := &ActionActivity{Action: &JavaActionCallAction{ErrorHandlingType: ErrorHandlingTypeContinue}}
	if got := ObjectErrorHandlingType(act); got != ErrorHandlingTypeContinue {
		t.Errorf("action activity: got %q, want Continue", got)
	}
	// Hand-built objects that only set the activity field keep working.
	legacy := &ActionActivity{BaseActivity: BaseActivity{ErrorHandlingType: ErrorHandlingTypeAbort}, Action: &UnknownAction{}}
	if got := ObjectErrorHandlingType(legacy); got != ErrorHandlingTypeAbort {
		t.Errorf("activity fallback: got %q, want Abort", got)
	}
	if got := ObjectErrorHandlingType(&LoopedActivity{ErrorHandlingType: ErrorHandlingTypeCustom}); got != ErrorHandlingTypeCustom {
		t.Errorf("loop: got %q", got)
	}
	var typedNil *JavaActionCallAction
	if got := ActionErrorHandlingType(typedNil); got != "" {
		t.Errorf("typed nil: got %q", got)
	}
	if got := ObjectErrorHandlingType(&Annotation{}); got != "" {
		t.Errorf("annotation: got %q", got)
	}
	// The stored spelling has a capital B; a rule comparing against
	// "CustomWithoutRollback" matches nothing.
	if ErrorHandlingTypeCustomWithoutRollback != "CustomWithoutRollBack" {
		t.Errorf("stored spelling changed: %q", ErrorHandlingTypeCustomWithoutRollback)
	}
}
