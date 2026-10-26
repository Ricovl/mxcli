// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#977 item 1: "`ReadOnlyStyle` is a silent drop on text box, text
// area, date picker and radio buttons. The writer hard-codes `Inherit`" — so
// `Dashboard_Fullscreen_web` `radioButtons1`, stored `ReadOnlyStyle: Text`, came
// back from describe → exec as `Inherit`. The check box was wired in #490; these
// four kept the constant.
func TestInputWidgetReadOnlyStyle_Written(t *testing.T) {
	widgets := map[string]func(style string) pages.Widget{
		"textbox": func(s string) pages.Widget {
			w := &pages.TextBox{ReadOnlyStyle: s}
			w.Name = "textBox1"
			return w
		},
		"textarea": func(s string) pages.Widget {
			w := &pages.TextArea{ReadOnlyStyle: s}
			w.Name = "textArea1"
			return w
		},
		"datepicker": func(s string) pages.Widget {
			w := &pages.DatePicker{ReadOnlyStyle: s}
			w.Name = "datePicker1"
			return w
		},
		"radiobuttons": func(s string) pages.Widget {
			w := &pages.RadioButtons{ReadOnlyStyle: s}
			w.Name = "radioButtons1"
			return w
		},
	}
	for kind, mk := range widgets {
		for _, tc := range []struct{ style, want string }{
			// Unset keeps Studio Pro's default — Inherit on 236 of 237 text
			// boxes, 57 of 58 text areas, 51 of 59 date pickers and 4 of 4
			// radio button groups across PedApp and TestApp.
			{"", "Inherit"},
			{"Text", "Text"},
			{"Control", "Control"},
		} {
			doc := encodeWidget(t, mk(tc.style))
			if got := docGet(doc, "ReadOnlyStyle"); got != tc.want {
				t.Errorf("%s with ReadOnlyStyle %q: stored %v, want %q (ako/mxcli#977)",
					kind, tc.style, got, tc.want)
			}
		}
	}
}
