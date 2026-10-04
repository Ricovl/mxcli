// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Input-widget formatting (ako/mxcli#968, mendixlabs/mxcli#1263).
//
// A date picker and a text box each store a Forms$FormattingInfo. On a date
// picker it is the picker's mode: DateFormat Time or DateTime is what makes it
// a time or date-time picker, and Custom pairs with a CustomDateFormat pattern.
// On a text box it also carries the decimal precision and digit grouping of a
// numeric attribute. Neither was read: the builder ignored the properties, the
// writer hard-coded the defaults and describe printed nothing, so `DateFormat:
// Time` passed check and was written as Date, and a Studio Pro date-time picker
// came back from describe → exec as a date-only one.
//
// The MDL spelling is the widget-property form of the keys a dynamic-text
// parameter's `format (…)` block already uses — PascalCase, like every other
// widget property: `DateFormat: DateTime, CustomDateFormat: 'dd-MM-yyyy HH:mm'`.

// inputFormatProps lists, per MDL widget type, the formatting properties the
// widget's FormattingInfo carries (pages.FormattingProperties, keyed by the
// storage type). A widget type that is absent has none MDL can set.
var inputFormatProps = map[string][]string{
	"datepicker": pages.FormattingProperties("Forms$DatePicker"),
	"textbox":    pages.FormattingProperties("Forms$TextBox"),
}

// inputFormatProp reports whether key is a formatting property of widgetType,
// and returns its canonical spelling.
func inputFormatProp(widgetType, key string) (string, bool) {
	for _, p := range inputFormatProps[strings.ToLower(widgetType)] {
		if strings.EqualFold(p, key) {
			return p, true
		}
	}
	return "", false
}

// inputFormattingInfo reads an input widget's formatting properties into the
// FormattingInfo the writer stores. It returns nil when the widget sets none,
// so the writer keeps the defaults byte-for-byte. Unset fields start from the
// Studio Pro defaults (Date, precision 2, Text, no grouping), as a dynamic-text
// parameter's format block does. Invalid values are an error; the check-time
// rule (MDL-WIDGET18) reports the same problems from the same function.
func inputFormattingInfo(w *ast.WidgetV3) (*pages.FormattingInfo, error) {
	if problems := inputFormattingProblems(w); len(problems) > 0 {
		return nil, fmt.Errorf("%s %s: %s", strings.ToLower(w.Type), w.Name, strings.Join(problems, "; "))
	}
	var fi *pages.FormattingInfo
	get := func() *pages.FormattingInfo {
		if fi == nil {
			fi = &pages.FormattingInfo{DateFormat: "Date", DecimalPrecision: 2, EnumFormat: "Text"}
		}
		return fi
	}
	for _, key := range inputFormatProps[strings.ToLower(w.Type)] {
		v, ok := lookupPropCI(w, key)
		if !ok {
			continue
		}
		switch key {
		case "DateFormat":
			s, _ := v.(string)
			get().DateFormat, _ = pages.CanonicalDateFormat(s)
		case "CustomDateFormat":
			s, _ := v.(string)
			get().CustomDateFormat = s
		case "DecimalPrecision":
			n, _ := toFormatInt(v)
			get().DecimalPrecision = n
		case "GroupDigits":
			b, _ := propBool(v)
			get().GroupDigits = b
		}
	}
	return fi, nil
}

// inputFormattingProblems validates an input widget's formatting properties.
// Each problem is a sentence fragment naming the property; nil means valid.
func inputFormattingProblems(w *ast.WidgetV3) []string {
	keys := inputFormatProps[strings.ToLower(w.Type)]
	if len(keys) == 0 {
		return nil
	}
	var out []string
	dateFormat := ""
	hasDateFormat := false
	for _, key := range keys {
		v, ok := lookupPropCI(w, key)
		if !ok {
			continue
		}
		switch key {
		case "DateFormat":
			hasDateFormat = true
			s, isStr := v.(string)
			canon, valid := pages.CanonicalDateFormat(s)
			if !isStr || !valid {
				out = append(out, fmt.Sprintf("DateFormat must be one of Date, Time, DateTime, Custom, got `%v`", v))
				continue
			}
			dateFormat = canon
		case "CustomDateFormat":
			if _, isStr := v.(string); !isStr {
				out = append(out, fmt.Sprintf("CustomDateFormat must be a quoted pattern such as 'dd-MM-yyyy HH:mm', got `%v`", v))
			}
		case "DecimalPrecision":
			if n, isInt := toFormatInt(v); !isInt || n < 0 {
				out = append(out, fmt.Sprintf("DecimalPrecision must be a non-negative integer, got `%v`", v))
			}
		case "GroupDigits":
			if _, err := propBool(v); err != nil {
				out = append(out, fmt.Sprintf("GroupDigits must be true or false, got `%v`", v))
			}
		}
	}
	custom, hasCustom := lookupPropCI(w, "CustomDateFormat")
	// A pattern with no DateFormat never applies: the default is Date. An
	// explicit other DateFormat beside a pattern is accepted — Studio Pro keeps
	// the pattern when the format is switched away from Custom (TestApp's
	// WorkflowCommons pickers store DateTime + 'dd/MM/yyyy HH:mm'), and
	// describe prints both so the round trip preserves it.
	if hasCustom && !hasDateFormat {
		out = append(out, "CustomDateFormat applies only with `DateFormat: Custom` — add it, or the pattern is ignored and the widget shows a date")
	}
	if dateFormat == "Custom" {
		if s, _ := custom.(string); strings.TrimSpace(s) == "" {
			out = append(out, "`DateFormat: Custom` needs a non-empty CustomDateFormat pattern, e.g. `CustomDateFormat: 'dd-MM-yyyy HH:mm'`")
		}
	}
	return out
}

// toFormatInt accepts the integer forms the visitor produces.
func toFormatInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
}

// describeInputFormatting renders a stored Forms$FormattingInfo as the MDL
// properties of an input widget of the given MDL type, emitting only what
// differs from the defaults the writer fills in (Date, precision 2, no
// grouping, no pattern), so an unformatted widget describes as before.
//
// A pattern beside a DateFormat other than Custom is printed with that
// DateFormat, even Date: Studio Pro keeps the pattern when the format is
// switched away from Custom, and check refuses a bare CustomDateFormat, so
// printing it alone would make describe's own output fail check.
func describeInputFormatting(ctx *ExecContext, mdlType string, w map[string]any) []string {
	keys := inputFormatProps[mdlType]
	fi, ok := w["FormattingInfo"].(map[string]any)
	if len(keys) == 0 || !ok || fi == nil {
		return nil
	}
	var props []string
	for _, key := range keys {
		switch key {
		case "DateFormat":
			df := extractString(fi["DateFormat"])
			if df == "" {
				df = "Date"
			}
			if df != "Date" || extractString(fi["CustomDateFormat"]) != "" {
				props = append(props, "DateFormat: "+df)
			}
		case "CustomDateFormat":
			if cdf := extractString(fi["CustomDateFormat"]); cdf != "" {
				props = append(props, "CustomDateFormat: "+mdlQuote(ctx, cdf))
			}
		case "DecimalPrecision":
			if v, present := fi["DecimalPrecision"]; present {
				if dp := extractInt(v); dp != 2 {
					props = append(props, fmt.Sprintf("DecimalPrecision: %d", dp))
				}
			}
		case "GroupDigits":
			if gd, _ := fi["GroupDigits"].(bool); gd {
				props = append(props, "GroupDigits: true")
			}
		}
	}
	return props
}
