// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// ako/mxcli#1563: the refusal's remedy is `drop txtOptionName;` before the
// replace. exec applies the drop first and has nothing left to refuse; check
// reads the stored document, which still holds the widget, so it has to count
// the drop itself or it refuses the very script the refusal recommends.
func TestDroppedFirstDiscountsTheRecommendedDrop(t *testing.T) {
	stale := &backend.StaleWidgetContentError{
		Widget: "cbBooking", Property: "optionsSourceAssociationCustomContent",
		DataSource: "optionsSourceAssociationDataSource", StoredEntity: "M.Booking", NewEntity: "M.Sale",
		Widgets: []string{"txtOptionName", "txtCode"}, WidgetTypes: []string{"Forms$DynamicText", "Forms$DynamicText"},
	}
	wrapped := fmt.Errorf("replace: %w", stale)

	for _, tc := range []struct {
		name    string
		err     error
		dropped map[string]bool
		want    bool
	}{
		{"every widget dropped first", wrapped, map[string]bool{"txtOptionName": true, "txtCode": true}, true},
		// Controls: each still reports.
		{"one widget left", wrapped, map[string]bool{"txtOptionName": true}, false},
		{"nothing dropped", wrapped, nil, false},
		{"another error", errors.New("replace: something else"), map[string]bool{"txtOptionName": true, "txtCode": true}, false},
		{"an unnamed widget cannot have been dropped", &backend.StaleWidgetContentError{Widgets: []string{""}},
			map[string]bool{"": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := droppedFirst(tc.err, tc.dropped); got != tc.want {
				t.Errorf("droppedFirst = %v, want %v", got, tc.want)
			}
		})
	}
}
