// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#898: an expression slot laid out over several lines — the last
// value of a member list before `)`, a declare's value before `;` — was stored
// with the layout whitespace that followed it (`false\n  `), because the
// visitor keeps a slot's trailing whitespace for describe's layout. The flow
// diff no longer sees it (#886), but the model carried it. It is trimmed when
// the expression is written.
const ws898Script = `create or modify persistent entity MyFirstModule.Ws898 (Name: String(50), Active: Boolean default false);
create or modify microflow MyFirstModule.Ws898_Flow ()
begin
  declare $Flag Boolean = %s
    ;
  $E = create MyFirstModule.Ws898 (
    Name = 'a',
    Active = false
  );
end;`

func ws898(value string) string { return strings.Replace(ws898Script, "%s", value, 1) }

// ws898Unit returns the test microflow's unit ID and stored bytes.
func ws898Unit(t *testing.T, exec *Executor) (model.ID, []byte) {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == "Ws898_Flow" {
			raw, err := ctx.Backend.GetRawUnitBytes(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return m.ID, append([]byte(nil), raw...)
		}
	}
	t.Fatal("MyFirstModule.Ws898_Flow not found")
	return "", nil
}

// ws898Expressions collects every InitialValue / Value string in a unit, in
// document order.
func ws898Expressions(t *testing.T, raw []byte) []string {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			for _, e := range x {
				if s, ok := e.Value.(string); ok && (e.Key == "InitialValue" || e.Key == "Value") {
					out = append(out, s)
					continue
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(d)
	return out
}

func TestExpressionSlot_TrailingWhitespaceIsNotStored(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := afRun(t, exec, ws898("false")); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	_, raw := ws898Unit(t, exec)
	got := ws898Expressions(t, raw)
	want := []string{"false", "'a'", "false"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("stored expressions = %q, want %q", got, want)
	}
}

// The idempotence side: a project whose stored expressions already carry the
// trailing whitespace — every project an earlier mxcli wrote — must not be
// rewritten by a re-run of the script that produced it. Trimming at write time
// must not turn every such expression into a difference.
func TestExpressionSlot_StoredTrailingWhitespaceDoesNotChurn(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := afRun(t, exec, ws898("false")); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}

	// Put the whitespace back, the way an earlier mxcli stored it.
	id, raw := ws898Unit(t, exec)
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	d = withTrailingWhitespace(d).(bson.D)
	patched, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	ctx := exec.newExecContext(context.Background())
	if err := ctx.Backend.UpdateRawUnit(string(id), patched); err != nil {
		t.Fatal(err)
	}
	_, raw = ws898Unit(t, exec)
	if got := ws898Expressions(t, raw); len(got) != 3 || got[0] != "false\n    " || got[2] != "false\n  " {
		t.Fatalf("setup: the stored expressions do not carry the whitespace: %q", got)
	}

	// Re-running the script writes nothing.
	before := projectFiles(t, dir)
	out.Reset()
	if err := afRun(t, exec, ws898("false")); err != nil {
		t.Fatalf("re-run: %v\n%s", err, out.String())
	}
	if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
		t.Errorf("re-running the script rewrote %v:\n%s", changed, out.String())
	}

	// Control: a real change is still written, so the check above can fail.
	before = projectFiles(t, dir)
	if err := afRun(t, exec, ws898("true")); err != nil {
		t.Fatalf("control: %v\n%s", err, out.String())
	}
	if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) == 0 {
		t.Error("control: changing the declared value wrote nothing — the re-run check cannot detect a write")
	}
}

// withTrailingWhitespace appends a line break and indentation to the stored
// declare value and to the last member's value — what the visitor captured
// before #898.
func withTrailingWhitespace(v any) any {
	switch x := v.(type) {
	case bson.D:
		out := make(bson.D, len(x))
		for i, e := range x {
			if s, ok := e.Value.(string); ok && s == "false" {
				switch e.Key {
				case "InitialValue":
					e.Value = s + "\n    "
				case "Value":
					e.Value = s + "\n  "
				}
			} else {
				e.Value = withTrailingWhitespace(e.Value)
			}
			out[i] = e
		}
		return out
	case bson.A:
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = withTrailingWhitespace(e)
		}
		return out
	}
	return v
}
