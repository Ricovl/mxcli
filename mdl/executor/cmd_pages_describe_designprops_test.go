// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// dpv builds a Studio Pro nested Forms$DesignPropertyValue map (Key + typed Value).
func dpv(key string, value map[string]any) map[string]any {
	return map[string]any{"$Type": "Forms$DesignPropertyValue", "Key": key, "Value": value}
}

// TestExtractDesignProperties_Compound guards the describe-read of compound
// (nested) design properties — issue #668. Before the fix, a
// Forms$CompoundDesignPropertyValue was dropped entirely (only flat props
// survived), so DESCRIBE PAGE / DESCRIBE STYLING silently lost it.
func TestExtractDesignProperties_Compound(t *testing.T) {
	appearance := map[string]any{
		// DesignProperties is a versioned BSON array: [version, prop1, prop2, …].
		"DesignProperties": []any{
			int32(1),
			dpv("Column gap", map[string]any{
				"$Type":  "Forms$OptionDesignPropertyValue",
				"Option": "Medium",
			}),
			dpv("Spacing", map[string]any{
				"$Type": "Forms$CompoundDesignPropertyValue",
				"Properties": []any{
					int32(1),
					dpv("margin-top", map[string]any{
						"$Type": "Forms$OptionDesignPropertyValue", "Option": "Large",
					}),
					dpv("margin-bottom", map[string]any{
						"$Type": "Forms$OptionDesignPropertyValue", "Option": "Medium",
					}),
				},
			}),
		},
	}

	got := extractDesignProperties(appearance)
	if len(got) != 2 {
		t.Fatalf("got %d design properties, want 2", len(got))
	}
	if got[0].Key != "Column gap" || got[0].ValueType != "option" || got[0].Option != "Medium" {
		t.Errorf("flat prop = %+v, want Column gap/option/Medium", got[0])
	}
	c := got[1]
	if c.Key != "Spacing" || c.ValueType != "compound" {
		t.Fatalf("compound prop = %+v, want Spacing/compound", c)
	}
	if len(c.Nested) != 2 {
		t.Fatalf("compound has %d nested props, want 2", len(c.Nested))
	}
	if c.Nested[0].Key != "margin-top" || c.Nested[0].Option != "Large" {
		t.Errorf("nested[0] = %+v, want margin-top/Large", c.Nested[0])
	}
	if c.Nested[1].Key != "margin-bottom" || c.Nested[1].Option != "Medium" {
		t.Errorf("nested[1] = %+v, want margin-bottom/Medium", c.Nested[1])
	}
}

// TestFormatDesignPropertiesMDL_Compound guards the emit side: compound props
// render as a nested MDL list that re-parses (verified end-to-end by the
// describe round-trip; this pins the exact string).
func TestFormatDesignPropertiesMDL_Compound(t *testing.T) {
	dps := []rawDesignProp{
		{Key: "Column gap", ValueType: "option", Option: "Medium"},
		{Key: "Spacing", ValueType: "compound", Nested: []rawDesignProp{
			{Key: "margin-top", ValueType: "option", Option: "Large"},
			{Key: "margin-bottom", ValueType: "option", Option: "Medium"},
		}},
		{Key: "Show divider", ValueType: "toggle"},
	}
	got := formatDesignPropertiesMDL(dps)
	want := "DesignProperties: ('Column gap': 'Medium', 'Spacing': ('margin-top': 'Large', 'margin-bottom': 'Medium'), 'Show divider': on)"
	if got != want {
		t.Errorf("formatDesignPropertiesMDL:\n got  %s\n want %s", got, want)
	}
}
