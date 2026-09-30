// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"errors"
	"strings"
	"testing"
)

// fakeFlows is a project's flows and whether each returns a String, keyed
// "microflow M.G" / "nanoflow M.G".
type fakeFlows map[string]bool

func (f fakeFlows) ReturnsString(nanoflow bool, qualifiedName string) (isString, found bool) {
	kind := "microflow "
	if nanoflow {
		kind = "nanoflow "
	}
	isString, found = f[kind+qualifiedName]
	return isString, found
}

// `find(…)` over a call's result is the string function or the List
// operation depending on what the called flow returns (ako/mxcli#860,
// CapTrackV6 11-viewstate and 26-admin-goals). The flow builder reads that
// return type when it builds the caller: from a flow the script created
// earlier, else from the project. The upgrade reads it from the same two
// places, in the same order.
func TestUpgrade_FindOnACallResult(t *testing.T) {
	const listFlow = "create or modify microflow M.Regions () returns List of M.E as $R begin\n" +
		"  retrieve $R from M.E;\n  return $R;\nend;\n"
	const stringFlow = "create or modify microflow M.Label () returns String begin\n  return 'x';\nend;\n"
	const caller = "create microflow M.F ($o: M.E) begin\n"
	for _, c := range []struct {
		name, src string
		flows     fakeFlows // nil: no project
		want      string    // the upgraded caller body line; "" when refused
		reason    string    // the refusal, when want is ""
	}{
		{"callee earlier in the script returns a list, no project",
			listFlow + caller + "  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			nil, "  $F = find $R where $currentObject = $o;", ""},
		{"callee earlier in the script returns a String, no project",
			stringFlow + caller + "  declare $F Integer = 0;\n  $S = call microflow M.Label();\n  $F = find($S, 'x');\nend;\n",
			nil, "  set $F = find($S, 'x');", ""},
		{"the script's callee wins over the project's",
			listFlow + caller + "  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{"microflow M.Regions": true}, "  $F = find $R where $currentObject = $o;", ""},
		{"callee in the project returns a list",
			caller + "  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{"microflow M.Regions": false}, "  $F = find $R where $currentObject = $o;", ""},
		{"callee in the project returns a String",
			caller + "  declare $F Integer = 0;\n  $S = call microflow M.Label();\n  $F = find($S, 'x');\nend;\n",
			fakeFlows{"microflow M.Label": true}, "  set $F = find($S, 'x');", ""},
		{"nanoflow callee in the project",
			caller + "  $R = call nanoflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{"nanoflow M.Regions": false, "microflow M.Regions": true}, "  $F = find $R where $currentObject = $o;", ""},
		{"contains on a call's result",
			caller + "  $R = call microflow M.Regions();\n  $B = contains($R, $o);\nend;\n",
			fakeFlows{"microflow M.Regions": false}, "  $B = contains $o in $R;", ""},

		{"no project",
			caller + "  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			nil, "", "pass the project"},
		{"callee defined after the caller, no project",
			caller + "  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n" + listFlow,
			nil, "", "pass the project"},
		{"the project has no such flow",
			caller + "  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{}, "", "has no microflow M.Regions"},
		{"a microflow is not a nanoflow",
			caller + "  $R = call nanoflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{"microflow M.Regions": false}, "", "has no nanoflow M.Regions"},
		{"callee dropped earlier in the script",
			"drop microflow M.Regions;\n" + caller + "  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{"microflow M.Regions": false}, "", "drops, renames or moves"},
		{"two definitions that disagree",
			caller + "  $R = call microflow M.Regions();\n  if true then\n    $R = call microflow M.Label();\n  end if;\n" +
				"  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{"microflow M.Regions": false, "microflow M.Label": true}, "", "a String on one path"},
		{"a call and a definition the project cannot answer",
			caller + "  $R = call java action M.J();\n  $R = call microflow M.Regions();\n  $F = find($R, $currentObject = $o);\nend;\n",
			fakeFlows{"microflow M.Regions": false}, "", "does not state"},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := Options{AddHeader: true}
			if c.flows != nil {
				opts.Flows = c.flows
			}
			res, err := Upgrade(c.src, opts)
			if c.want == "" {
				var hb *HeaderBlockedError
				if !errors.As(err, &hb) {
					t.Fatalf("header added: %v\n%s", err, res.Source)
				}
				found := false
				for _, b := range hb.Constructs {
					if b.Code == "MDL-V1-LIST" && strings.Contains(b.Reason, c.reason) {
						found = true
					}
				}
				if !found {
					t.Errorf("want MDL-V1-LIST (%q), got %+v", c.reason, hb.Constructs)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(res.Source, "\n"+c.want+"\n") {
				t.Fatalf("want line %q in:\n%s", c.want, res.Source)
			}
			again, err := Upgrade(res.Source, opts)
			if err != nil || again.Source != res.Source || again.Changed() {
				t.Errorf("not idempotent: %v %q", err, again.Source)
			}
		})
	}
}
