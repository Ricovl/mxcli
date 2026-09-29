// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#818: under `mdl 1` a `create or modify` of a stored flow refused
// every header or document-property change and every node a stated @position
// or @start moved — and `describe` states both, always. They are patches now:
// a changed property is set on the stored document, a stated position moves
// the stored node, and a parameter is added, retyped or removed in place. The
// proof is the stored unit itself: a describe edited only there writes exactly
// that, and every other byte — every element $ID, flow and curve — stays.
//
// Every flow here is Studio Pro-authored (PedApp). ShowPasswordForm:
//
//	create or modify microflow Administration.ShowPasswordForm (
//	  @position(100, 0)
//	  $Account: Administration.Account
//	)
//	folder 'User Management/Admin'
//	begin
//	  @start(100, 200)
//	  @position(265, 200) … $AccountPasswordData = create …;
//	  @position(515, 200) … show page …;
//	  @position(695, 200)
//	  return;
//	end;

const pwdForm = "ShowPasswordForm"

// strictDiff lists every leaf that differs between two units, $IDs and
// pointers included, with list elements matched by $ID: an added element is
// one line, and a property changed in place names its element. It is the
// "exactly these changes" measure; bsonDiff, which skips binaries, is not.
func strictDiff(t *testing.T, a, b []byte) []string {
	t.Helper()
	var da, db bson.D
	if err := bson.Unmarshal(a, &da); err != nil {
		t.Fatal(err)
	}
	if err := bson.Unmarshal(b, &db); err != nil {
		t.Fatal(err)
	}
	var out []string
	strictWalk("", da, db, &out)
	sort.Strings(out)
	return out
}

func idOf(v any) string {
	if d, ok := v.(bson.D); ok {
		for _, e := range d {
			if e.Key == "$ID" {
				if b, ok := e.Value.(bson.Binary); ok {
					return uuidOf(b.Data)
				}
			}
		}
	}
	return ""
}

func typeOf(v any) string {
	if d, ok := v.(bson.D); ok {
		for _, e := range d {
			if e.Key == "$Type" {
				s, _ := e.Value.(string)
				return strings.TrimPrefix(strings.TrimPrefix(s, "Microflows$"), "DataTypes$")
			}
		}
	}
	return ""
}

func strictWalk(path string, a, b any, out *[]string) {
	switch av := a.(type) {
	case bson.D:
		bv, ok := b.(bson.D)
		if !ok {
			*out = append(*out, path+": replaced")
			return
		}
		bm := map[string]any{}
		for _, e := range bv {
			bm[e.Key] = e.Value
		}
		seen := map[string]bool{}
		for _, e := range av {
			seen[e.Key] = true
			v, ok := bm[e.Key]
			if !ok {
				*out = append(*out, path+"/"+e.Key+": removed")
				continue
			}
			strictWalk(path+"/"+e.Key, e.Value, v, out)
		}
		for _, e := range bv {
			if !seen[e.Key] {
				*out = append(*out, path+"/"+e.Key+": added")
			}
		}
	case bson.A:
		bv, ok := b.(bson.A)
		if !ok {
			*out = append(*out, path+": replaced")
			return
		}
		byID := map[string]any{}
		for _, el := range bv {
			if id := idOf(el); id != "" {
				byID[id] = el
			}
		}
		if len(byID) == 0 {
			if fmt.Sprint(av) != fmt.Sprint(bv) {
				*out = append(*out, path+": changed")
			}
			return
		}
		inA := map[string]bool{}
		for _, el := range av {
			id := idOf(el)
			if id == "" {
				continue // a list marker
			}
			inA[id] = true
			if o, ok := byID[id]; ok {
				strictWalk(path+"/"+typeOf(el), el, o, out)
			} else {
				*out = append(*out, path+"/"+typeOf(el)+": removed")
			}
		}
		for _, el := range bv {
			if id := idOf(el); id != "" && !inA[id] {
				*out = append(*out, path+"/"+typeOf(el)+": added")
			}
		}
	default:
		ab, aBin := a.(bson.Binary)
		bb, bBin := b.(bson.Binary)
		if aBin && bBin {
			if !bytes.Equal(ab.Data, bb.Data) {
				*out = append(*out, path+": changed")
			}
			return
		}
		if fmt.Sprint(a) != fmt.Sprint(b) {
			*out = append(*out, path+": changed")
		}
	}
}

// requireOnly fails unless the differences are exactly want (sorted).
func requireOnly(t *testing.T, got []string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the write changed:\n  %s\nwant exactly:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// editDescribe applies replacements to a description, failing when one does
// not apply (the fixture changed shape).
func editDescribe(t *testing.T, described string, pairs ...string) string {
	t.Helper()
	out := described
	for i := 0; i < len(pairs); i += 2 {
		next := strings.Replace(out, pairs[i], pairs[i+1], 1)
		if next == out {
			t.Fatalf("describe output has no %q:\n%s", pairs[i], described)
		}
		out = next
	}
	return out
}

// execUnchanged is the control each test starts from: the unedited
// description writes nothing.
func execUnchanged(t *testing.T, h *harness, described string) {
	t.Helper()
	before := h.flowUnit(t, pwdForm)
	if err := h.exec("mdl 1;\n" + described); err != nil {
		t.Fatalf("control: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, pwdForm), before) {
		t.Fatal("control: the unedited description rewrote the unit")
	}
}

// requireFixedPoint re-executes the description of the written flow: it must
// write nothing, or the patch left something describe does not state.
func requireFixedPoint(t *testing.T, h *harness) string {
	t.Helper()
	again := h.mustDescribe(t, showPasswordForm)
	settled := h.flowUnit(t, pwdForm)
	if err := h.exec("mdl 1;\n" + again); err != nil {
		t.Fatalf("re-exec of the new description: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, pwdForm), settled) {
		t.Fatalf("re-executing the description of the patched flow rewrote it: %v",
			strictDiff(t, settled, h.flowUnit(t, pwdForm)))
	}
	return again
}

// Document properties: export level, concurrency and documentation are set on
// the stored document and nothing else changes — under mdl 1 and mdl 0 alike
// (the rebuild mdl 0 used to fall back to is what reset Studio Pro's layout).
func TestFlowModify_DocumentPropertiesArePatched(t *testing.T) {
	for _, header := range []string{"mdl 1;\n", ""} {
		t.Run(fmt.Sprintf("header %q", header), func(t *testing.T) {
			h := newHarness(t)
			defer h.close()
			described := h.mustDescribe(t, showPasswordForm)
			execUnchanged(t, h, described)

			edited := "/** Shows the change-password form. */\n" + editDescribe(t, described,
				"folder 'User Management/Admin'\n",
				"folder 'User Management/Admin'\nexport level api\ndisallow concurrent execution error message 'Busy, try again'\n")
			before := h.flowUnit(t, pwdForm)
			if err := h.exec(header + edited); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if strings.Contains(h.out.String(), "MDL-V1-REBUILD") {
				t.Fatalf("the header change fell back to the rebuild:\n%s", h.out.String())
			}
			got := strictDiff(t, before, h.flowUnit(t, pwdForm))
			requireOnly(t, got,
				"/AllowConcurrentExecution: changed",
				"/ConcurrenyErrorMessage/Items/Texts$Translation/Text: changed",
				"/Documentation: changed",
				"/ExportLevel: changed",
			)
			again := requireFixedPoint(t, h)
			for _, want := range []string{"export level api", "disallow concurrent execution error message 'Busy, try again'", "Shows the change-password form."} {
				if !strings.Contains(again, want) {
					t.Errorf("describe does not show %q:\n%s", want, again)
				}
			}
			if changed := h.orig.diff(h.snapshot()); len(changed) != 1 {
				t.Errorf("want only the flow written, got: %s", strings.Join(changed, "; "))
			}
		})
	}
}

// A parameter is added in place (one new object, nothing else), retyped in
// place (its type only), and removed again — which gives the stored bytes
// back exactly.
func TestFlowModify_ParametersArePatched(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	described := h.mustDescribe(t, showPasswordForm)
	execUnchanged(t, h, described)
	orig := h.flowUnit(t, pwdForm)

	added := editDescribe(t, described, "  $Account: Administration.Account\n", "  $Account: Administration.Account,\n  $Note: String\n")
	if err := h.exec("mdl 1;\n" + added); err != nil {
		t.Fatalf("add: %v", err)
	}
	requireOnly(t, strictDiff(t, orig, h.flowUnit(t, pwdForm)), "/ObjectCollection/Objects/MicroflowParameter: added")
	withNote := h.flowUnit(t, pwdForm)

	retyped := editDescribe(t, added, "$Note: String", "$Note: Integer")
	if err := h.exec("mdl 1;\n" + retyped); err != nil {
		t.Fatalf("retype: %v", err)
	}
	// The type is a new element of another $Type, so it has its own $ID; the
	// parameter keeps its.
	requireOnly(t, strictDiff(t, withNote, h.flowUnit(t, pwdForm)),
		"/ObjectCollection/Objects/MicroflowParameter/VariableType/$ID: changed",
		"/ObjectCollection/Objects/MicroflowParameter/VariableType/$Type: changed")
	if !strings.Contains(requireFixedPoint(t, h), "$Note: Integer") {
		t.Error("describe does not show the retyped parameter")
	}

	if err := h.exec("mdl 1;\n" + described); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, pwdForm), orig) {
		t.Fatalf("removing the added parameter did not give the stored unit back: %v", strictDiff(t, orig, h.flowUnit(t, pwdForm)))
	}

	// A parameter the flow still uses cannot be removed: the create action
	// names $Account.
	before := h.flowUnit(t, pwdForm)
	gone := editDescribe(t, described, "  @position(100, 0)\n  $Account: Administration.Account\n", "")
	err := h.exec("mdl 1;\n" + gone)
	if err == nil || !strings.Contains(err.Error(), "$Account is removed, but the flow still uses it") {
		t.Fatalf("want a refusal naming the use, got %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, pwdForm), before) {
		t.Fatal("the refused removal wrote")
	}
}

// Stated positions move the stored nodes: an activity, the start event, the
// end event and a parameter. Only their positions change — no flow, no curve,
// no $ID.
func TestFlowModify_StatedPositionsMoveStoredNodes(t *testing.T) {
	for _, header := range []string{"mdl 1;\n", ""} {
		t.Run(fmt.Sprintf("header %q", header), func(t *testing.T) {
			h := newHarness(t)
			defer h.close()
			described := h.mustDescribe(t, showPasswordForm)
			execUnchanged(t, h, described)

			edited := editDescribe(t, described,
				"@position(100, 0)", "@position(40, 0)",
				"@start(100, 200)", "@start(100, 120)",
				"@position(515, 200)", "@position(515, 300)",
				"@position(695, 200)", "@position(695, 300)",
			)
			before := h.flowUnit(t, pwdForm)
			if err := h.exec(header + edited); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if strings.Contains(h.out.String(), "MDL-V1-REBUILD") {
				t.Fatalf("the moves fell back to the rebuild:\n%s", h.out.String())
			}
			requireOnly(t, strictDiff(t, before, h.flowUnit(t, pwdForm)),
				"/ObjectCollection/Objects/ActionActivity/RelativeMiddlePoint: changed",
				"/ObjectCollection/Objects/EndEvent/RelativeMiddlePoint: changed",
				"/ObjectCollection/Objects/MicroflowParameter/RelativeMiddlePoint: changed",
				"/ObjectCollection/Objects/StartEvent/RelativeMiddlePoint: changed",
			)
			again := requireFixedPoint(t, h)
			for _, want := range []string{"@position(40, 0)", "@start(100, 120)", "@position(515, 300)", "@position(695, 300)"} {
				if !strings.Contains(again, want) {
					t.Errorf("describe does not show %s:\n%s", want, again)
				}
			}
		})
	}
}

// A connector the script redraws is not a position: the splice moves nodes
// and does not reroute flows, so under mdl 1 it is refused and nothing is
// written.
func TestFlowModify_RedrawnConnectorIsRefused(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	described := h.mustDescribe(t, showPasswordForm)
	edited := editDescribe(t, described, "@curve(from: (30, 0), to: (-15, 0))", "@curve(from: (30, 0), to: (-15, 20))")
	before := h.flowUnit(t, pwdForm)
	err := h.exec("mdl 1;\n" + edited)
	if err == nil || !strings.Contains(err.Error(), "cannot be spliced") || !strings.Contains(err.Error(), "connector") {
		t.Fatalf("want a refusal naming the connector, got %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, pwdForm), before) {
		t.Fatal("the refused statement wrote")
	}
}

// An inserted statement with a stated @position is written there, and nothing
// around it is moved to make room (the splice used to place it itself and
// ignore the position).
func TestFlowModify_InsertHonoursStatedPosition(t *testing.T) {
	for _, header := range []string{"mdl 1;\n", ""} {
		t.Run(fmt.Sprintf("header %q", header), func(t *testing.T) {
			h := newHarness(t)
			defer h.close()
			described := h.mustDescribe(t, showPasswordForm)
			edited := editDescribe(t, described,
				"  @position(515, 200)\n",
				"  @position(390, 330)\n  log info node 'Pwd' 'shown';\n  @position(515, 200)\n")
			before := h.flowUnit(t, pwdForm)
			if err := h.exec(header + edited); err != nil {
				t.Fatalf("exec: %v", err)
			}
			after := h.flowUnit(t, pwdForm)
			requireKept(t, before, after, "")
			again := h.mustDescribe(t, showPasswordForm)
			if i, j := strings.Index(again, "@position(390, 330)"), strings.Index(again, "log node 'Pwd' 'shown';"); i < 0 || j < i ||
				strings.Contains(again[i:j], ";") {
				t.Errorf("the inserted log is not where the script put it:\n%s", again)
			}
			for _, want := range []string{"@position(265, 200)", "@position(515, 200)", "@position(695, 200)"} {
				if !strings.Contains(again, want) {
					t.Errorf("a stored node moved (no %s):\n%s", want, again)
				}
			}
		})
	}
}

// An insert that states @position on a later statement but not on its first
// cannot be placed: the fragment is translated into the gap as a whole, so the
// stated position was lost and the two logs were written on top of each other
// (found reviewing ako/mxcli#824). It is refused under mdl 1 and nothing is
// written; stated on every statement, both land where stated.
func TestFlowModify_InsertPositionOnLaterStatementOnly(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	described := h.mustDescribe(t, showPasswordForm)
	edited := editDescribe(t, described,
		"  @position(515, 200)\n",
		"  log info node 'Pwd' 'one';\n  @position(390, 330)\n  log info node 'Pwd' 'two';\n  @position(515, 200)\n")
	before := h.flowUnit(t, pwdForm)
	err := h.exec("mdl 1;\n" + edited)
	if err == nil || !strings.Contains(err.Error(), "@position") {
		t.Fatalf("want a refusal naming @position, got %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, pwdForm), before) {
		t.Fatal("a refused insert wrote the unit")
	}

	both := editDescribe(t, described,
		"  @position(515, 200)\n",
		"  @position(390, 330)\n  log info node 'Pwd' 'one';\n  @position(390, 450)\n  log info node 'Pwd' 'two';\n  @position(515, 200)\n")
	if err := h.exec("mdl 1;\n" + both); err != nil {
		t.Fatalf("exec: %v", err)
	}
	again := h.mustDescribe(t, showPasswordForm)
	for _, st := range []struct{ pos, log string }{
		{"@position(390, 330)", "log node 'Pwd' 'one';"}, {"@position(390, 450)", "log node 'Pwd' 'two';"},
	} {
		if i, j := strings.Index(again, st.pos), strings.Index(again, st.log); i < 0 || j < i || strings.Contains(again[i:j], ";") {
			t.Errorf("%s is not at %s:\n%s", st.log, st.pos, again)
		}
	}
}

// The same on TestApp (Studio Pro-authored), on a flow with a decision: a
// canonical description — derived positions left out — edited in its header
// and in three positions (the split, an activity in a branch, the end of the
// other branch) writes exactly those five values.
func TestFlowModify_TestAppHeaderAndPositions(t *testing.T) {
	h := newFixtureHarness(t, testApp)
	defer h.close()
	const target = "microflow Administration.ChangePassword"
	described := h.mustDescribe(t, target)
	before := h.flowUnit(t, "ChangePassword")
	if err := h.exec("mdl 1;\n" + described); err != nil {
		t.Fatalf("control: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, "ChangePassword"), before) {
		t.Fatal("control: the unedited description rewrote the unit")
	}

	edited := "/** Changes an account's password. */\n" + editDescribe(t, described,
		"folder 'User Management/Admin'\n", "folder 'User Management/Admin'\nexport level api\n",
		"  @position(425, 200)\n", "  @position(425, 230)\n",
		"    @position(960, 200)\n", "    @position(960, 320)\n",
		"    @position(425, -15)\n", "    @position(500, -15)\n",
	)
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec: %v", err)
	}
	requireOnly(t, strictDiff(t, before, h.flowUnit(t, "ChangePassword")),
		"/Documentation: changed",
		"/ExportLevel: changed",
		"/ObjectCollection/Objects/ActionActivity/RelativeMiddlePoint: changed",
		"/ObjectCollection/Objects/EndEvent/RelativeMiddlePoint: changed",
		"/ObjectCollection/Objects/ExclusiveSplit/RelativeMiddlePoint: changed",
	)
	settled := h.flowUnit(t, "ChangePassword")
	again := h.mustDescribe(t, target)
	for _, want := range []string{"export level api", "@position(425, 230)", "@position(960, 320)", "@position(500, -15)"} {
		if !strings.Contains(again, want) {
			t.Errorf("describe does not show %s:\n%s", want, again)
		}
	}
	if err := h.exec("mdl 1;\n" + again); err != nil {
		t.Fatalf("re-exec: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, "ChangePassword"), settled) {
		t.Fatal("re-executing the description of the patched flow rewrote it")
	}
}
