// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// ako/mxcli-ledger FINDINGS #170, end to end over Studio Pro-authored pages.
// PedApp stores SubmitBehaviour WhileEditing on three inputs of
// FeedbackModule.ShareFeedback and Width FixedWidth on two layout grids of
// MyFirstModule.Home_Web. describe → exec rewrote both to the defaults with
// `mx check` clean, because MDL had no spelling for either.

// storedWidgetProps returns, per named widget of the stored page, the values
// of the given keys.
func storedWidgetProps(t *testing.T, exec *Executor, page string, keys ...string) map[string]map[string]any {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	unit, err := ctx.Backend.GetRawUnitByName("page", page)
	if err != nil || unit == nil {
		t.Fatalf("raw unit of %s: %v", page, err)
	}
	var doc bson.D
	if err := bson.Unmarshal(unit.Contents, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", page, err)
	}
	out := map[string]map[string]any{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			m := map[string]any{}
			for _, e := range x {
				m[e.Key] = e.Value
			}
			if name, _ := m["Name"].(string); name != "" {
				for _, k := range keys {
					if v, ok := m[k]; ok {
						if out[name] == nil {
							out[name] = map[string]any{}
						}
						out[name][k] = v
					}
				}
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

func TestSubmitBehaviourAndGridWidth_StudioProPageRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		page    string
		key     string
		widgets []string
		value   string
		mdl     string
	}{
		{"FeedbackModule.ShareFeedback", "SubmitBehaviour",
			[]string{"feedback_subject", "textBox1", "textArea2"}, "WhileEditing", "SubmitBehaviour: WhileEditing"},
		{"MyFirstModule.Home_Web", "Width",
			[]string{"layoutGrid2", "layoutGrid3"}, "FixedWidth", "Width: FixedWidth"},
	} {
		t.Run(tc.page, func(t *testing.T) {
			exec, out := openPedAppWithWidgets(t)
			before := storedWidgetProps(t, exec, tc.page, tc.key)
			for _, w := range tc.widgets {
				if before[w][tc.key] != tc.value {
					t.Fatalf("fixture: %s.%s = %v, want the Studio Pro value %s", w, tc.key, before[w][tc.key], tc.value)
				}
			}

			described := describeOn(t, exec, out, tc.page)
			if n := strings.Count(described, tc.mdl); n != len(tc.widgets) {
				t.Errorf("describe printed %q %d times, want %d:\n%s", tc.mdl, n, len(tc.widgets), described)
			}

			out.Reset()
			if err := afRun(t, exec, described); err != nil {
				t.Fatalf("re-exec describe output: %v\n%s", err, out.String())
			}
			after := storedWidgetProps(t, exec, tc.page, tc.key)
			for _, w := range tc.widgets {
				if after[w][tc.key] != tc.value {
					t.Errorf("describe → exec rewrote %s.%s %v → %v", w, tc.key, tc.value, after[w][tc.key])
				}
			}

			// Control: the stored value is what the description says, not
			// something the writer cannot change — an edited description lands.
			edited := strings.Replace(described, tc.mdl, map[string]string{
				"SubmitBehaviour": "SubmitBehaviour: OnEndEditing",
				"Width":           "Width: FullWidth",
			}[tc.key], 1)
			if edited == described {
				t.Fatal("control edit did not apply")
			}
			if err := afRun(t, exec, edited); err != nil {
				t.Fatalf("exec edited description: %v", err)
			}
			changed := 0
			stored := storedWidgetProps(t, exec, tc.page, tc.key)
			for _, w := range tc.widgets {
				if stored[w][tc.key] != tc.value {
					changed++
				}
			}
			if changed != 1 {
				t.Errorf("the control edit changed %d widgets, want 1", changed)
			}
		})
	}
}

// Authored in MDL, then described: the values reach storage and come back.
func TestSubmitBehaviourAndGridWidth_AuthoredInMDL(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const page = "MyFirstModule.Submit170_Edit"
	script := `create non-persistent entity MyFirstModule.Submit170 (Note: String(200));
create page ` + page + ` (
  Title: 'Submit', Layout: Atlas_Core.Atlas_Default,
  Params: ( $Item: MyFirstModule.Submit170 )
) {
  layoutgrid lgMain (Width: FixedWidth) {
    row {
      column (DesktopWidth: 12) {
        dataview dv (DataSource: $Item) {
          textbox tbLive (Label: 'Live', Attribute: Note, SubmitBehaviour: WhileEditing, SubmitOnInputDelay: 500)
          textarea taLive (Label: 'Live', Attribute: Note, SubmitBehaviour: WhileEditing)
          textbox tbPlain (Label: 'Plain', Attribute: Note)
        }
      }
    }
  }
};`
	if err := afRun(t, exec, script); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	got := storedWidgetProps(t, exec, page, "SubmitBehaviour", "SubmitOnInputDelay", "Width")
	for name, want := range map[string]map[string]any{
		"lgMain":  {"Width": "FixedWidth"},
		"tbLive":  {"SubmitBehaviour": "WhileEditing"},
		"taLive":  {"SubmitBehaviour": "WhileEditing"},
		"tbPlain": {"SubmitBehaviour": "OnEndEditing"},
	} {
		for k, v := range want {
			if got[name][k] != v {
				t.Errorf("%s.%s = %v, want %v", name, k, got[name][k], v)
			}
		}
	}
	if d, _ := toFormatInt(asGoInt(got["tbLive"]["SubmitOnInputDelay"])); d != 500 {
		t.Errorf("tbLive.SubmitOnInputDelay = %v, want 500", got["tbLive"]["SubmitOnInputDelay"])
	}

	described := describeOn(t, exec, out, page)
	for _, want := range []string{"Width: FixedWidth", "SubmitBehaviour: WhileEditing", "SubmitOnInputDelay: 500"} {
		if !strings.Contains(described, want) {
			t.Errorf("describe should print %q:\n%s", want, described)
		}
	}
	// GetPut: executing the description writes nothing.
	before := projectFiles(t, dir)
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec describe output: %v\n%s", err, out.String())
	}
	if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
		t.Errorf("executing the describe output wrote %v", changed)
	}
}

func TestSubmitBehaviourAndGridWidth_AlterPageSet(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	const page = "FeedbackModule.ShareFeedback"
	if err := afRun(t, exec, `alter page `+page+` { set (SubmitBehaviour: OnEndEditing, SubmitOnInputDelay: 800) on textBox1; };`); err != nil {
		t.Fatalf("alter page set: %v\n%s", err, out.String())
	}
	got := storedWidgetProps(t, exec, page, "SubmitBehaviour", "SubmitOnInputDelay")
	if got["textBox1"]["SubmitBehaviour"] != "OnEndEditing" {
		t.Errorf("textBox1.SubmitBehaviour = %v, want OnEndEditing", got["textBox1"]["SubmitBehaviour"])
	}
	if d, _ := toFormatInt(asGoInt(got["textBox1"]["SubmitOnInputDelay"])); d != 800 {
		t.Errorf("textBox1.SubmitOnInputDelay = %v, want 800", got["textBox1"]["SubmitOnInputDelay"])
	}
	// The neighbour is untouched.
	if got["textArea2"]["SubmitBehaviour"] != "WhileEditing" {
		t.Errorf("textArea2.SubmitBehaviour = %v, want the stored WhileEditing", got["textArea2"]["SubmitBehaviour"])
	}

	const home = "MyFirstModule.Home_Web"
	if err := afRun(t, exec, `alter page `+home+` { set Width = FullWidth on layoutGrid2; };`); err != nil {
		t.Fatalf("alter page set Width: %v", err)
	}
	if w := storedWidgetProps(t, exec, home, "Width")["layoutGrid2"]["Width"]; w != "FullWidth" {
		t.Errorf("layoutGrid2.Width = %v, want FullWidth", w)
	}

	for _, c := range []struct{ page, stmt, want string }{
		{page, `set SubmitBehaviour = Immediately on textBox1`, "OnEndEditing or WhileEditing"},
		{page, `set SubmitOnInputDelay = fast on textBox1`, "non-negative"},
		{home, `set Width = Wide on layoutGrid3`, "FullWidth or FixedWidth"},
	} {
		err := afRun(t, exec, `alter page `+c.page+` { `+c.stmt+`; };`)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error containing %q, got %v", c.stmt, c.want, err)
		}
	}
	if w := storedWidgetProps(t, exec, home, "Width")["layoutGrid3"]["Width"]; w != "FixedWidth" {
		t.Errorf("a refused set changed layoutGrid3.Width to %v", w)
	}
}

// asGoInt normalises a stored BSON integer for toFormatInt.
func asGoInt(v any) any {
	if n, ok := v.(int32); ok {
		return int(n)
	}
	return v
}
