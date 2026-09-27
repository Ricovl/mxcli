// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestPedAppRoundTrip_RetrieveRange is ako/mxcli#734 on Studio Pro-authored
// content: `retrieve … first` is the object range in every language version,
// and under `mdl 1;` `retrieve … limit 1` is a list of one. describe → exec
// must keep each range as stored, for both forms.
//
// FeedbackModule.PopulateUserAttributes holds PedApp's only retrieve with a
// range: Studio Pro's "First object" (a ConstantRange with SingleObject). The
// whole microflow does not pass GetPut yet (#721, allowlisted), so this test
// judges the range element itself rather than the unit's bytes.
func TestPedAppRoundTrip_RetrieveRange(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow FeedbackModule.PopulateUserAttributes"
	const objectRange = "Microflows$ConstantRange SingleObject=true"
	const listOfOne = "Microflows$CustomRange Limit=\"1\" Offset=\"\""

	if got := h.retrieveRanges(t, "PopulateUserAttributes"); strings.Join(got, "; ") != objectRange {
		t.Fatalf("fixture changed: the retrieve's stored range is %q, want the one object range %q", got, objectRange)
	}

	// --- The object form -------------------------------------------------
	described, err := h.describe(target)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if !strings.Contains(described, "\n    first;") || strings.Contains(described, "limit 1") {
		t.Fatalf("describe must print the object range as `first` (never `limit 1`, a list under mdl 1):\n%s", described)
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		script := header + described
		if warned := limitOneWarnings(t, script); warned != 0 {
			t.Errorf("header %q: `first` warned MDL-V1-LIMIT1 %d time(s); it means the same in every version", header, warned)
		}
		if err := h.exec(script); err != nil {
			t.Fatalf("header %q: exec describe output: %v", header, err)
		}
		if got := h.retrieveRanges(t, "PopulateUserAttributes"); strings.Join(got, "; ") != objectRange {
			t.Errorf("header %q: object form stored as %q, want %q", header, got, objectRange)
		}
		if again, _ := h.describe(target); again != described {
			t.Errorf("header %q: PutGet — describe changed after exec:\n%s", header, lineDiff(described, again))
		}
	}

	// --- The list form, under mdl 1 ----------------------------------------
	listScript := "mdl 1;\n" + strings.Replace(described, "\n    first;", "\n    limit 1;", 1)
	if err := h.exec(listScript); err != nil {
		t.Fatalf("exec the list form under mdl 1: %v", err)
	}
	if got := h.retrieveRanges(t, "PopulateUserAttributes"); strings.Join(got, "; ") != listOfOne {
		t.Fatalf("`limit 1` under mdl 1 stored as %q, want a list of one %q", got, listOfOne)
	}
	listDescribed, err := h.describe(target)
	if err != nil {
		t.Fatalf("describe the list form: %v", err)
	}
	if !strings.Contains(listDescribed, "\n    limit 1;") || strings.Contains(listDescribed, "first;") {
		t.Fatalf("describe must print a Custom range with limit 1 as `limit 1`:\n%s", listDescribed)
	}
	if err := h.exec("mdl 1;\n" + listDescribed); err != nil {
		t.Fatalf("exec the list form's describe output under mdl 1: %v", err)
	}
	if got := h.retrieveRanges(t, "PopulateUserAttributes"); strings.Join(got, "; ") != listOfOne {
		t.Errorf("list form's describe output under mdl 1 stored %q, want %q", got, listOfOne)
	}
	if again, _ := h.describe(target); again != listDescribed {
		t.Errorf("PutGet under mdl 1 — describe changed after exec:\n%s", lineDiff(listDescribed, again))
	}

	// --- Control: the same text without the header keeps the alpha meaning --
	// and says so. Without this, a writer that ignored the version would pass
	// both halves above.
	if warned := limitOneWarnings(t, listDescribed); warned != 1 {
		t.Errorf("headerless `limit 1` warned MDL-V1-LIMIT1 %d time(s), want 1", warned)
	}
	if err := h.exec(listDescribed); err != nil {
		t.Fatalf("exec the list form's describe output without a header: %v", err)
	}
	if got := h.retrieveRanges(t, "PopulateUserAttributes"); strings.Join(got, "; ") != objectRange {
		t.Errorf("headerless `limit 1` stored %q, want the alpha meaning, the object range %q", got, objectRange)
	}
}

// limitOneWarnings counts the MDL-V1-LIMIT1 warnings check reports for script.
func limitOneWarnings(t *testing.T, script string) int {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	n := 0
	for _, v := range executor.ValidateLanguageVersion(prog) {
		if v.RuleID == "MDL-V1-LIMIT1" {
			n++
		}
	}
	return n
}

// retrieveRanges returns the range of every retrieve in the named microflow, as
// stored: the range's $Type and the properties that decide object or list.
func (h *harness) retrieveRanges(t *testing.T, microflow string) []string {
	t.Helper()
	snap := h.snapshot()
	var found []string
	for id, b := range snap.units {
		typ, name := typeAndName(b)
		if typ != "Microflows$Microflow" || name != microflow {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(b, &doc); err != nil {
			t.Fatalf("decode %s: %v", id, err)
		}
		collectRanges(doc, &found)
		sort.Strings(found)
		return found
	}
	t.Fatalf("microflow %s not found", microflow)
	return nil
}

func collectRanges(v any, out *[]string) {
	switch x := v.(type) {
	case bson.D:
		m := map[string]any{}
		for _, e := range x {
			m[e.Key] = e.Value
		}
		switch m["$Type"] {
		case "Microflows$ConstantRange":
			*out = append(*out, fmt.Sprintf("Microflows$ConstantRange SingleObject=%v", m["SingleObject"]))
		case "Microflows$CustomRange":
			// An absent expression reads as empty, the way Mendix treats it.
			str := func(k string) string { s, _ := m[k].(string); return s }
			*out = append(*out, fmt.Sprintf("Microflows$CustomRange Limit=%q Offset=%q", str("LimitExpression"), str("OffsetExpression")))
		}
		for _, e := range x {
			collectRanges(e.Value, out)
		}
	case bson.A:
		for _, e := range x {
			collectRanges(e, out)
		}
	}
}
