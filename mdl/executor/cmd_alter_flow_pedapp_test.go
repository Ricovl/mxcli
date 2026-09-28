// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The acceptance test of plan item 4.2 (ako/mxcli#736) runs on the Studio
// Pro-authored PedApp fixture, because only a flow Studio Pro drew can show
// what a rebuild loses: its merges, its curves, its object order, its $IDs.

// openPedAppFixture opens a private copy of testdata/pedapp.
func openPedAppFixture(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyPedAppFile(filepath.Join(src, "PedApp.mpr"), filepath.Join(dir, "PedApp.mpr")); err != nil {
		t.Fatal(err)
	}
	if err := copyPedAppTree(filepath.Join(src, "mprcontents"), filepath.Join(dir, "mprcontents")); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, "PedApp.mpr")}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	return exec, out
}

func afRun(t *testing.T, exec *Executor, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs[0])
	}
	for _, s := range prog.Statements {
		if err := exec.Execute(s); err != nil {
			return err
		}
	}
	return nil
}

// valFeedbackUnit returns VAL_Feedback's unit ID and stored bytes.
func valFeedbackUnit(t *testing.T, exec *Executor) (model.ID, []byte) {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == "VAL_Feedback" && h.GetModuleName(h.FindModuleID(m.ContainerID)) == "FeedbackModule" {
			raw, err := ctx.Backend.GetRawUnitBytes(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return m.ID, append([]byte(nil), raw...)
		}
	}
	t.Fatal("FeedbackModule.VAL_Feedback not found")
	return "", nil
}

// flowView indexes a stored flow's top-level objects and flows by $ID.
type flowView struct {
	doc   bson.D
	objs  map[string]bson.D
	flows map[string]bson.D
	order []string // object ids in storage order
}

func parseFlowView(t *testing.T, raw []byte) flowView {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	v := flowView{doc: d, objs: map[string]bson.D{}, flows: map[string]bson.D{}}
	oc, _ := afGet(d, "ObjectCollection").(bson.D)
	for _, el := range afList(afGet(oc, "Objects")) {
		o := el.(bson.D)
		id := afIDOf(afGet(o, "$ID"))
		v.objs[id] = o
		v.order = append(v.order, id)
	}
	for _, el := range afList(afGet(d, "Flows")) {
		f := el.(bson.D)
		v.flows[afIDOf(afGet(f, "$ID"))] = f
	}
	return v
}

func afGet(d bson.D, k string) any {
	for _, e := range d {
		if e.Key == k {
			return e.Value
		}
	}
	return nil
}

func afList(v any) []any {
	a, _ := v.(bson.A)
	if len(a) > 0 {
		if _, ok := a[0].(int32); ok {
			return a[1:]
		}
	}
	return a
}

func afIDOf(v any) string {
	b, _ := v.(primitive.Binary)
	return types.BlobToUUID(b.Data)
}

func afMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := bson.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// without returns d minus the named keys, for comparing the rest.
func afWithout(d bson.D, keys ...string) bson.D {
	var out bson.D
	for _, e := range d {
		skip := false
		for _, k := range keys {
			skip = skip || e.Key == k
		}
		if !skip {
			out = append(out, e)
		}
	}
	return out
}

// changedKeys lists the keys whose values differ between two elements (one
// level deep, plus the Line's vectors, which is where a flow keeps its curve).
func afChangedKeys(t *testing.T, a, b bson.D) []string {
	t.Helper()
	var out []string
	keys := map[string]bool{}
	for _, e := range a {
		keys[e.Key] = true
	}
	for _, e := range b {
		keys[e.Key] = true
	}
	for k := range keys {
		av, bv := afGet(a, k), afGet(b, k)
		if k == "Line" {
			al, _ := av.(bson.D)
			bl, _ := bv.(bson.D)
			for _, lk := range []string{"OriginControlVector", "DestinationControlVector"} {
				if fmt.Sprint(afGet(al, lk)) != fmt.Sprint(afGet(bl, lk)) {
					out = append(out, "Line."+lk)
				}
			}
			if !bytes.Equal(afMarshal(t, afWithout(al, "OriginControlVector", "DestinationControlVector")),
				afMarshal(t, afWithout(bl, "OriginControlVector", "DestinationControlVector"))) {
				out = append(out, "Line")
			}
			continue
		}
		if !bytes.Equal(afMarshal(t, bson.D{{Key: "v", Value: av}}), afMarshal(t, bson.D{{Key: "v", Value: bv}})) {
			out = append(out, k)
		}
	}
	return afSort(out)
}

func afSort(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

func afPoint(d bson.D) (int, int) {
	s, _ := afGet(d, "RelativeMiddlePoint").(string)
	x, y, _ := strings.Cut(s, ";")
	px, _ := strconv.Atoi(x)
	py, _ := strconv.Atoi(y)
	return px, py
}

func afSize(d bson.D) (int, int) {
	s, _ := afGet(d, "Size").(string)
	x, y, _ := strings.Cut(s, ";")
	px, _ := strconv.Atoi(x)
	py, _ := strconv.Atoi(y)
	return px, py
}

// objectAt returns the id of the stored top-level object at (x, y).
func (v flowView) objectAt(t *testing.T, x, y int, typ string) string {
	t.Helper()
	for _, id := range v.order {
		o := v.objs[id]
		if px, py := afPoint(o); px == x && py == y && afGet(o, "$Type") == typ {
			return id
		}
	}
	t.Fatalf("no %s at (%d, %d)", typ, x, y)
	return ""
}

// The acceptance test of plan item 4.2 (ako/mxcli#736): one `log` inserted
// after $IsValidEmail in the Studio Pro-drawn VAL_Feedback. Only the new
// activity, the two flows around it and the positions moved to make room may
// differ; every other element — its $ID, its curve, every merge — must come
// through byte-identical.
func TestAlterMicroflow_PedApp_InsertAfterChangesOnlyTheSplice(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	_, raw := valFeedbackUnit(t, exec)
	before := parseFlowView(t, raw)
	javaCall := before.objectAt(t, 980, 200, "Microflows$ActionActivity")
	split := before.objectAt(t, 1155, 200, "Microflows$ExclusiveSplit")

	script := "alter microflow FeedbackModule.VAL_Feedback {\n" +
		"  insert after $IsValidEmail { log info node 'Feedback' 'Email checked'; }\n" +
		"};"
	if n := strings.Count(script, "\n") + 1; n > 5 {
		t.Fatalf("the acceptance script is %d lines; the plan allows 5", n)
	}
	if err := afRun(t, exec, script); err != nil {
		t.Fatalf("alter: %v", err)
	}
	_, rawAfter := valFeedbackUnit(t, exec)
	after := parseFlowView(t, rawAfter)

	// The document around the flow is untouched.
	if !bytes.Equal(afMarshal(t, afWithout(before.doc, "ObjectCollection", "Flows")),
		afMarshal(t, afWithout(after.doc, "ObjectCollection", "Flows"))) {
		t.Error("a property of the microflow document itself changed")
	}
	ocBefore, _ := afGet(before.doc, "ObjectCollection").(bson.D)
	ocAfter, _ := afGet(after.doc, "ObjectCollection").(bson.D)
	if !bytes.Equal(afMarshal(t, afWithout(ocBefore, "Objects")), afMarshal(t, afWithout(ocAfter, "Objects"))) {
		t.Error("a property of the object collection changed")
	}

	// Objects: every stored one survives with its $ID and in its place in the
	// list; exactly one is new, a log activity; the rest differ at most in
	// position, and only by the shift that made room.
	for i, id := range before.order {
		if after.order[i] != id {
			t.Fatalf("stored object %d moved in the list: %s became %s", i, id, after.order[i])
		}
	}
	if got := len(after.order) - len(before.order); got != 1 {
		t.Fatalf("want exactly one new object, got %d", got)
	}
	newID := after.order[len(after.order)-1]
	newObj := after.objs[newID]
	if action, _ := afGet(newObj, "Action").(bson.D); afGet(newObj, "$Type") != "Microflows$ActionActivity" ||
		afGet(action, "$Type") != "Microflows$LogMessageAction" {
		t.Fatalf("the new object is not a log activity: %v", afGet(newObj, "$Type"))
	}
	shifted := 0
	javaX, _ := afPoint(before.objs[javaCall])
	for _, id := range before.order {
		b, a := before.objs[id], after.objs[id]
		changed := afChangedKeys(t, b, a)
		if len(changed) == 0 {
			continue
		}
		if len(changed) != 1 || changed[0] != "RelativeMiddlePoint" {
			t.Errorf("object %s (%v) changed more than its position: %v", id, afGet(b, "$Type"), changed)
			continue
		}
		bx, by := afPoint(b)
		ax, ay := afPoint(a)
		if ay != by || ax <= bx || bx <= javaX {
			t.Errorf("object %s moved from (%d,%d) to (%d,%d): only objects past the insertion point may move, and only along the flow",
				id, bx, by, ax, ay)
		}
		shifted++
	}
	if shifted == 0 {
		t.Error("nothing was shifted, yet the gap after $IsValidEmail is too narrow for an activity: placement did not run")
	}
	// The new activity overlaps nothing.
	nx, ny := afPoint(newObj)
	nw, nh := afSize(newObj)
	for _, id := range after.order {
		if id == newID {
			continue
		}
		o := after.objs[id]
		ox, oy := afPoint(o)
		ow, oh := afSize(o)
		if abs(nx-ox)*2 < nw+ow && abs(ny-oy)*2 < nh+oh {
			t.Errorf("the new activity at (%d,%d) overlaps %v at (%d,%d)", nx, ny, afGet(o, "$Type"), ox, oy)
		}
	}

	// Flows: every stored flow survives; one is new (log -> split); one is
	// rewired (java call -> log), and only at its destination end.
	var added []string
	for id := range after.flows {
		if _, ok := before.flows[id]; !ok {
			added = append(added, id)
		}
	}
	if len(added) != 1 {
		t.Fatalf("want exactly one new flow, got %d", len(added))
	}
	nf := after.flows[added[0]]
	if afIDOf(afGet(nf, "OriginPointer")) != newID || afIDOf(afGet(nf, "DestinationPointer")) != split {
		t.Error("the new flow does not run from the log to 'Email is Valid?'")
	}
	rewired := 0
	for id, b := range before.flows {
		a, ok := after.flows[id]
		if !ok {
			t.Errorf("stored flow %s is gone", id)
			continue
		}
		changed := afChangedKeys(t, b, a)
		if len(changed) == 0 {
			continue
		}
		rewired++
		if afIDOf(afGet(b, "OriginPointer")) != javaCall || afIDOf(afGet(a, "DestinationPointer")) != newID {
			t.Errorf("flow %s changed but is not the flow out of $IsValidEmail: %v", id, changed)
		}
		for _, k := range changed {
			switch k {
			case "DestinationPointer", "DestinationConnectionIndex", "Line.DestinationControlVector":
			default:
				t.Errorf("the rewired flow changed %s; only its destination end may change", k)
			}
		}
	}
	if rewired != 1 {
		t.Errorf("want exactly one rewired flow, got %d", rewired)
	}
	// The new flow ends where the rewired one used to.
	var oldIn bson.D
	for id, b := range before.flows {
		if afIDOf(afGet(b, "OriginPointer")) == javaCall {
			oldIn = b
			_ = id
		}
	}
	if fmt.Sprint(afGet(nf, "DestinationConnectionIndex")) != fmt.Sprint(afGet(oldIn, "DestinationConnectionIndex")) {
		t.Error("the new flow does not enter 'Email is Valid?' on the side the old flow did")
	}

	// And the result reads back: describe shows the log between the two.
	var buf bytes.Buffer
	exec.output = &buf
	if err := afRun(t, exec, "describe microflow FeedbackModule.VAL_Feedback;"); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	iCall := strings.Index(body, "$IsValidEmail = call java action")
	iLog := strings.Index(body, "log node 'Feedback' 'Email checked';")
	iSplit := strings.Index(body, "@caption 'Email is Valid?'")
	if iCall < 0 || iLog < iCall || iSplit < iLog {
		t.Errorf("describe does not show the log between the call and the decision:\n%s", body)
	}
}

// The control: an alter with no operations reads the unit, patches nothing
// and writes nothing — the stored bytes survive the decode/encode round trip
// exactly, so any difference the test above sees is the splice's.
func TestAlterMicroflow_PedApp_EmptyAlterChangesNothing(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	_, raw := valFeedbackUnit(t, exec)
	if err := afRun(t, exec, "alter microflow FeedbackModule.VAL_Feedback { };"); err != nil {
		t.Fatalf("alter: %v", err)
	}
	_, rawAfter := valFeedbackUnit(t, exec)
	if !bytes.Equal(raw, rawAfter) {
		t.Error("an empty alter changed the stored unit")
	}
}

// Drop joins the flow into the dropped activity to the one after it, and
// takes away only the activity and the flow that left it.
func TestAlterMicroflow_PedApp_Drop(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	_, raw := valFeedbackUnit(t, exec)
	before := parseFlowView(t, raw)
	target := before.objectAt(t, 1305, 460, "Microflows$ActionActivity")
	mergeAfter := before.objectAt(t, 1460, 460, "Microflows$ExclusiveMerge")

	if err := afRun(t, exec, "alter microflow FeedbackModule.VAL_Feedback { drop set $ValidFeedback = false @3; };"); err != nil {
		t.Fatalf("alter: %v", err)
	}
	_, rawAfter := valFeedbackUnit(t, exec)
	after := parseFlowView(t, rawAfter)

	if _, ok := after.objs[target]; ok {
		t.Fatal("the dropped activity is still there")
	}
	if len(after.objs) != len(before.objs)-1 || len(after.flows) != len(before.flows)-1 {
		t.Fatalf("want one object and one flow fewer, got %d->%d objects, %d->%d flows",
			len(before.objs), len(after.objs), len(before.flows), len(after.flows))
	}
	for id, b := range before.objs {
		if id == target {
			continue
		}
		if !bytes.Equal(afMarshal(t, b), afMarshal(t, after.objs[id])) {
			t.Errorf("object %s changed", id)
		}
	}
	rewired := 0
	for id, b := range before.flows {
		a, ok := after.flows[id]
		if !ok {
			if afIDOf(afGet(b, "OriginPointer")) != target {
				t.Errorf("flow %s is gone but did not leave the dropped activity", id)
			}
			continue
		}
		if changed := afChangedKeys(t, b, a); len(changed) > 0 {
			rewired++
			if afIDOf(afGet(b, "DestinationPointer")) != target || afIDOf(afGet(a, "DestinationPointer")) != mergeAfter {
				t.Errorf("flow %s changed but is not the flow into the dropped activity: %v", id, changed)
			}
		}
	}
	if rewired != 1 {
		t.Errorf("want one rewired flow, got %d", rewired)
	}
	if bytes.Contains(rawAfter, uuidBytes(target)) {
		t.Error("the unit still contains the dropped activity's $ID")
	}
}

func uuidBytes(id string) []byte { return types.UUIDToBlob(id) }

// Replace puts a two-activity fragment where one activity was: the flows in
// and out are re-pointed, everything past it moves along to make room.
func TestAlterMicroflow_PedApp_Replace(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	_, raw := valFeedbackUnit(t, exec)
	before := parseFlowView(t, raw)
	target := before.objectAt(t, 1305, 460, "Microflows$ActionActivity")

	err := afRun(t, exec, `alter microflow FeedbackModule.VAL_Feedback {
		replace set $ValidFeedback = false @3 with {
			set $ValidFeedback = false;
			log warning node 'Feedback' 'Email rejected';
		}
	};`)
	if err != nil {
		t.Fatalf("alter: %v", err)
	}
	_, rawAfter := valFeedbackUnit(t, exec)
	after := parseFlowView(t, rawAfter)
	if _, ok := after.objs[target]; ok {
		t.Fatal("the replaced activity is still there")
	}
	if got := len(after.objs) - len(before.objs); got != 1 {
		t.Errorf("want one object more (two in, one out), got %+d", got)
	}
	if got := len(after.flows) - len(before.flows); got != 1 {
		t.Errorf("want one flow more (the fragment's own), got %+d", got)
	}
	for id := range before.flows {
		if _, ok := after.flows[id]; !ok {
			t.Errorf("stored flow %s is gone; replace keeps the flows in and out", id)
		}
	}
	if bytes.Contains(rawAfter, uuidBytes(target)) {
		t.Error("the unit still contains the replaced activity's $ID")
	}
	var buf bytes.Buffer
	exec.output = &buf
	if err := afRun(t, exec, "describe microflow FeedbackModule.VAL_Feedback;"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "log warning node 'Feedback' 'Email rejected';") {
		t.Errorf("describe does not show the replacement:\n%s", buf.String())
	}
}

// What the splice cannot do safely, it refuses — before writing anything.
func TestAlterMicroflow_PedApp_Refusals(t *testing.T) {
	cases := []struct{ name, op, want string }{
		{"insert after a decision", `insert after 'Email is Valid?' { log info 'x'; }`, "which branch"},
		{"drop a decision", `drop 'Email is Valid?';`, "cannot drop"},
		{"drop a variable still read", `drop $IsValidEmail;`, "still used"},
		{"drop the end", `drop return $ValidFeedback;`, "cannot drop"},
		{"declare an existing variable", `insert after $IsValidEmail { declare $ValidFeedback Boolean = true; }`, "already has"},
		{"use a variable not yet declared", `insert after $ValidFeedback { log info 'x {1}' with ({1} = toString($IsValidEmail)); }`, "not declared on the path"},
		{"ambiguous target", `drop set $ValidFeedback = false;`, "add an ordinal"},
		{"unknown target", `drop $Nope;`, "no activity matches"},
		{"a fragment that returns", `insert after $IsValidEmail { return false; }`, "ends the flow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, _ := openPedAppFixture(t)
			_, raw := valFeedbackUnit(t, exec)
			err := afRun(t, exec, "alter microflow FeedbackModule.VAL_Feedback { "+tc.op+" };")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
			if _, rawAfter := valFeedbackUnit(t, exec); !bytes.Equal(raw, rawAfter) {
				t.Error("a refused alter changed the stored unit")
			}
		})
	}
}

var _ = microflows.ActionActivity{}

// alter nanoflow goes through the same splice: the unit is a
// Microflows$Nanoflow with the same object collection and flows.
func TestAlterNanoflow_PedApp_InsertBefore(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	err := afRun(t, exec, `alter nanoflow FeedbackModule.ACT_Feedback_ClearImage {
		insert before call javascript action * { declare $Cleared Boolean = true; }
	};`)
	if err != nil {
		t.Fatalf("alter: %v", err)
	}
	var buf bytes.Buffer
	exec.output = &buf
	if err := afRun(t, exec, "describe nanoflow FeedbackModule.ACT_Feedback_ClearImage;"); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	iChange := strings.Index(body, "change $Feedback")
	iDeclare := strings.Index(body, "declare $Cleared Boolean = true;")
	iCall := strings.Index(body, "call javascript action")
	if iChange < 0 || iDeclare < iChange || iCall < iDeclare {
		t.Errorf("describe does not show the declare between the change and the call:\n%s", body)
	}
}

// The scope check covers the whole statement, not each operation against the
// stored flow alone. Measured with mx check 11.14 on TestApp before the fix:
// two inserts declaring the same variable gave CE0111 "Duplicate variable
// name", and an insert reading a variable a drop in the same statement took
// away gave CE0109 "Undefined variable" — both after "Altered microflow".
func TestAlterMicroflow_PedApp_ScopeSpansTheStatement(t *testing.T) {
	t.Run("two fragments declare the same variable", func(t *testing.T) {
		exec, _ := openPedAppFixture(t)
		_, raw := valFeedbackUnit(t, exec)
		err := afRun(t, exec, `alter microflow FeedbackModule.VAL_Feedback {
			insert after $IsValidEmail { declare $Dup Boolean = true; }
			insert after $ValidFeedback { declare $Dup Boolean = false; }
		};`)
		if err == nil || !strings.Contains(err.Error(), "$Dup") {
			t.Fatalf("want the clash on $Dup refused, got %v", err)
		}
		if _, after := valFeedbackUnit(t, exec); !bytes.Equal(raw, after) {
			t.Error("a refused alter changed the stored unit")
		}
	})

	// SUB_Feedback_Sanitize reads its nine $Sanitized* outputs in one change;
	// replacing that change first leaves $SanitizedPageName unread, so only a
	// fragment of the second statement reads it.
	const unread = `alter microflow FeedbackModule.SUB_Feedback_Sanitize {
		replace change $Feedback (Subject = $SanitizedSubject, Description = $SanitizedDescription, SubmitterUUID = $SanitizedSubmitterUUID, SubmitterEmail = $SanitizedSubmitterEmail, SubmitterDisplayName = $SanitizedSubmitterDisplayName, ActiveUserRoles = $SanitizedActiveUserRoles, PageName = $SanitizedPageName, Browser = $SanitizedBrowser, EnvironmentURL = $SanitizedEnvironmentURL) with { log info 'sanitized'; }
	};`
	const use = `insert after $SanitizedSubmitterUUID { log info 'page {1}' with ({1} = $SanitizedPageName); }`
	const drop = `drop $SanitizedPageName;`
	for name, ops := range map[string]string{
		"insert a reader, then drop the producer": use + "\n" + drop,
		"drop the producer, then insert a reader": drop + "\n" + use,
	} {
		t.Run(name, func(t *testing.T) {
			exec, _ := openPedAppFixture(t)
			if err := afRun(t, exec, unread); err != nil {
				t.Fatalf("setup: %v", err)
			}
			err := afRun(t, exec, "alter microflow FeedbackModule.SUB_Feedback_Sanitize {\n"+ops+"\n};")
			if err == nil || !strings.Contains(err.Error(), "$SanitizedPageName") {
				t.Fatalf("want the read of a dropped $SanitizedPageName refused, got %v", err)
			}
		})
	}
	// Control: each operation on its own is accepted.
	t.Run("control", func(t *testing.T) {
		for _, op := range []string{use, drop} {
			exec, _ := openPedAppFixture(t)
			if err := afRun(t, exec, unread); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if err := afRun(t, exec, "alter microflow FeedbackModule.SUB_Feedback_Sanitize {\n"+op+"\n};"); err != nil {
				t.Errorf("%s: %v", op, err)
			}
		}
	})
}

// A loop in the fragment declares its iterator; the scope check must count it
// as the fragment's own, or every loop fragment is refused as reading an
// undeclared variable.
func TestAlterMicroflow_PedApp_LoopFragmentDeclaresItsIterator(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	err := afRun(t, exec, `alter microflow FeedbackModule.VAL_Feedback {
		insert after $IsValidEmail {
			$Items = create list of FeedbackModule.Feedback;
			loop $Item in $Items begin log info 'item'; end loop;
		}
	};`)
	if err != nil {
		t.Fatalf("alter: %v", err)
	}
}
