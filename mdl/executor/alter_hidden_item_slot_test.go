// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// An Accordion hides a group's headerContent while headerRenderMode is
// 'text', the default (`"text"===n.headerRenderMode?hidePropertyIn(…,
// "groups",…,"headerContent")`). Writing widgets there stored content Studio
// Pro does not render, and the project as stored failed with CE0463 at the
// Accordion — measured on a blank 11.13.0 app with --no-update-widgets: 0
// errors before the write, 1 after; the normalising default check hid it
// (PROPOSAL_alter_object_list_item_address.md, "a slot the item's
// configuration hides is refused").
func TestHiddenItemSlotIsRefused(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(accordionGetProperties)
	err := itemSlotHiddenError(rules, "acc1 item@1", "groups", "headercontent",
		map[string]string{"headerrendermode": "text"}, nil)
	if err == nil {
		t.Fatal("a write into headerContent of a text-mode group was not refused")
	}
	for _, want := range []string{"headerContent", "headerRenderMode", "'text'", "acc1 item@1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name %s: %v", want, err)
		}
	}
}

func TestVisibleItemSlotIsAccepted(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(accordionGetProperties)
	if err := itemSlotHiddenError(rules, "acc1 item@1", "groups", "headercontent",
		map[string]string{"headerrendermode": "custom"}, nil); err != nil {
		t.Errorf("headerContent of a custom-mode group must be writable: %v", err)
	}
	// The body is never hidden by this rule, whatever the mode.
	if err := itemSlotHiddenError(rules, "acc1 item@1", "groups", "content",
		map[string]string{"headerrendermode": "text"}, nil); err != nil {
		t.Errorf("content must be writable in text mode: %v", err)
	}
}

// A value the stored item does not carry is undeterminable, and an
// undeterminable rule refuses nothing — the same stance as check's
// validateWidgetItemVisibility.
func TestUndeterminableItemSlotIsAccepted(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(accordionGetProperties)
	if err := itemSlotHiddenError(rules, "acc1 item@1", "groups", "headercontent", nil, nil); err != nil {
		t.Errorf("no stored headerRenderMode: %v", err)
	}
}
