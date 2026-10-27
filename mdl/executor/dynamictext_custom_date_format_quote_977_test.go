// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#977 item 4: "Describe does not escape `'` in a dynamic-text
// `customDateFormat`, so a pattern with quoted literal text (`'at'`) does not
// round-trip." A Java date pattern quotes literal text — `h 'at' mm` — and
// describe printed it inside a single-quoted literal as it was:
// `customDateFormat: 'h 'at' mm'`, which ends the string at `h `.
package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// storedTemplateCustomDateFormats lists every template parameter's stored
// CustomDateFormat on the page.
func storedTemplateCustomDateFormats(t *testing.T, exec *Executor, page string) []string {
	t.Helper()
	_, raw := pageUnitByName(t, exec, page)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	walkStoredDocs(doc, func(d bson.D) {
		if afGet(d, "$Type") != "Forms$ClientTemplateParameter" {
			return
		}
		fi, _ := afGet(d, "FormattingInfo").(bson.D)
		cdf, _ := afGet(fi, "CustomDateFormat").(string)
		out = append(out, cdf)
	})
	return out
}

func TestDynamicTextCustomDateFormatWithQuotesRoundTrips(t *testing.T) {
	exec, out := openPedAppFixture(t)
	src := `create persistent entity MyFirstModule.Shift (Starts: DateTime);
create page MyFirstModule.Shift_View (Title: 'Shift', Layout: Atlas_Core.Atlas_Default, Params: ($Shift: MyFirstModule.Shift)) {
  dataview dvShift (DataSource: $Shift) {
    dynamictext textStarts (Content: '{1}', ContentParams: ({1} = Starts format (dateFormat: Custom, customDateFormat: 'h ''at'' mm')))
  }
};`
	if err := afRun(t, exec, src); err != nil {
		t.Fatalf("exec: %v", err)
	}
	const want = "h 'at' mm"
	if got := storedTemplateCustomDateFormats(t, exec, "Shift_View"); len(got) != 1 || got[0] != want {
		t.Fatalf("stored CustomDateFormat = %q, want [%q]", got, want)
	}

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Shift_View;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	if !strings.Contains(described, `customDateFormat: 'h ''at'' mm'`) {
		t.Fatalf("describe did not escape the quotes in the pattern:\n%s", described)
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}
	if got := storedTemplateCustomDateFormats(t, exec, "Shift_View"); len(got) != 1 || got[0] != want {
		t.Errorf("after describe → exec CustomDateFormat = %q, want [%q]", got, want)
	}
}
