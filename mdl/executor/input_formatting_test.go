// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#968: `check` on a date picker's / text box's formatting. Before the
// fix every case below checked clean — the format keys were exempted from the
// unknown-property warning on every widget type, and nothing validated them.
func TestValidateInputFormatting(t *testing.T) {
	page := func(widget string) string {
		return `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default, Params: ($B: M.B)) {
  dataview dv (DataSource: $B) { ` + widget + ` }
};`
	}
	for _, c := range []struct {
		name, widget, rule, want string
	}{
		{"valid time", `datepicker d (Attribute: Moment, DateFormat: Time)`, "", ""},
		{"valid custom", `datepicker d (Attribute: Moment, DateFormat: Custom, CustomDateFormat: 'dd-MM-yyyy')`, "", ""},
		{"stale pattern beside DateTime", `datepicker d (Attribute: Moment, DateFormat: DateTime, CustomDateFormat: 'dd-MM-yyyy')`, "", ""},
		{"valid precision", `textbox t (Attribute: Amount, DecimalPrecision: 0, GroupDigits: true)`, "", ""},
		{"unknown date format", `datepicker d (Attribute: Moment, DateFormat: Weekly)`, "MDL-WIDGET18", "DateFormat must be one of Date, Time, DateTime, Custom"},
		{"pattern without DateFormat", `datepicker d (Attribute: Moment, CustomDateFormat: 'HH:mm')`, "MDL-WIDGET18", "applies only with `DateFormat: Custom`"},
		// Measured: mxbuild CE0493 "Date format is custom but no format string is specified."
		{"custom without pattern", `datepicker d (Attribute: Moment, DateFormat: Custom)`, "MDL-WIDGET18", "needs a non-empty CustomDateFormat"},
		{"negative precision", `textbox t (Attribute: Amount, DecimalPrecision: -1)`, "MDL-WIDGET18", "non-negative integer"},
		// A widget with no FormattingInfo still drops the key, and says so.
		{"DateFormat on a text area", `textarea a (Attribute: Notes, DateFormat: Time)`, "MDL-WIDGET07", "DateFormat"},
		// A text box binds no dates (CE2421), so DateFormat means nothing there.
		{"DateFormat on a text box", `textbox t (Attribute: Amount, DateFormat: Time)`, "MDL-WIDGET07", "DateFormat"},
	} {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build(page(c.widget))
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			vs := ValidateWidgetProperties(prog, "")
			var hit []string
			for _, v := range vs {
				if strings.HasPrefix(v.RuleID, "MDL-WIDGET18") || strings.HasPrefix(v.RuleID, "MDL-WIDGET07") {
					hit = append(hit, v.RuleID+": "+v.Message)
				}
			}
			if c.rule == "" {
				if len(hit) != 0 {
					t.Errorf("want no formatting violation, got %v", hit)
				}
				return
			}
			for _, h := range hit {
				if strings.HasPrefix(h, c.rule) && strings.Contains(h, c.want) {
					return
				}
			}
			t.Errorf("want %s containing %q, got %v", c.rule, c.want, hit)
		})
	}
}
