// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#950: three describe outputs that did not survive describe → check →
// exec → describe. Each case creates the subject, then executes its own
// description, and compares what is stored — not only the text — before and
// after:
//
//   - `delete close page` was stored as ClosePage=false, so the button stopped
//     closing its page. Plain `delete` is the control: it must stay false.
//   - `return [%CurrentUser%];` was described as `return $[%CurrentUser%];`,
//     which does not parse.
//   - a navigation-list item's page action was described as the legacy
//     `show_page 'M.P'`, which does not parse. (TestApp's Studio Pro-authored
//     Rules.Entity_Menu, whose items are also unnamed, is covered by the
//     whole-fixture round trip.)
const describe950 = `create persistent entity MyFirstModule.Thing950 (Name: String(100));
create or modify page MyFirstModule.Thing950_Edit (Title: 'Edit', Layout: Atlas_Core.PopupLayout, Params: { $Thing: MyFirstModule.Thing950 }) {
  dataview dv (DataSource: $Thing) {
    actionbutton bDelClose (Caption: 'Delete and close', Action: delete close page)
    actionbutton bDel (Caption: 'Delete', Action: delete)
  }
};
create or modify page MyFirstModule.Thing950_Overview (Title: 'Things', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: 'Things')
};
create or modify page MyFirstModule.Menu950 (Title: 'Menu', Layout: Atlas_Core.Atlas_Default) {
  navigationlist nav {
    item (Action: show page MyFirstModule.Thing950_Overview) {
      dynamictext t1 (Content: 'Things')
    }
    item i2 (Action: sign out) {
      dynamictext t2 (Content: 'Sign out')
    }
  }
};
create or modify microflow MyFirstModule.CurrentUser950 ()
returns System.User
begin
  return [%CurrentUser%];
end;
`

func TestDescribeReExecutes_950(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	if err := h.exec(describe950); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}
	wantClose := map[string]bool{"bDelClose": true, "bDel": false}
	if got := deleteClosePage(t, h.pageUnit(t, "Thing950_Edit")); !mapsEqual(got, wantClose) {
		t.Fatalf("as created, delete actions store ClosePage %v, want %v", got, wantClose)
	}

	for _, c := range []struct{ target, want string }{
		{"page MyFirstModule.Thing950_Edit", "Action: delete close page"},
		{"page MyFirstModule.Menu950", "item (Action: show page MyFirstModule.Thing950_Overview)"},
		{"microflow MyFirstModule.CurrentUser950", "return [%CurrentUser%];"},
	} {
		t.Run(c.target, func(t *testing.T) {
			first := h.mustDescribeMdl0(t, c.target)
			if !strings.Contains(first, c.want) {
				t.Fatalf("describe has no %q:\n%s", c.want, first)
			}
			if errs := h.checkReferences(first); len(errs) > 0 {
				t.Fatalf("check of the description: %v\n%s", errs[0], first)
			}
			before := h.snapshot()
			if err := h.exec(first); err != nil {
				t.Fatalf("exec the description: %v\n%s", err, first)
			}
			if changed := before.diff(h.snapshot()); len(changed) > 0 {
				t.Errorf("executing the description wrote:\n  %s", strings.Join(changed, "\n  "))
			}
			if again := h.mustDescribeMdl0(t, c.target); again != first {
				t.Errorf("describe after exec differs:\n--- before ---\n%s\n--- after ---\n%s", first, again)
			}
		})
	}
	if got := deleteClosePage(t, h.pageUnit(t, "Thing950_Edit")); !mapsEqual(got, wantClose) {
		t.Errorf("after describe → exec, delete actions store ClosePage %v, want %v", got, wantClose)
	}
}

// deleteClosePage maps each action button's name to the ClosePage its
// Forms$DeleteClientAction stores.
func deleteClosePage(t *testing.T, unit []byte) map[string]bool {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(unit, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			name := ""
			for _, e := range x {
				if e.Key == "Name" {
					name, _ = e.Value.(string)
				}
			}
			for _, e := range x {
				if a, ok := e.Value.(bson.D); ok && e.Key == "Action" {
					typ, closePage := "", false
					for _, f := range a {
						switch f.Key {
						case "$Type":
							typ, _ = f.Value.(string)
						case "ClosePage":
							closePage, _ = f.Value.(bool)
						}
					}
					if typ == "Forms$DeleteClientAction" {
						out[name] = closePage
					}
				}
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

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
