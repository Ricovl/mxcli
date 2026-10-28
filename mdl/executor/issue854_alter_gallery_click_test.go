// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#854: `ALTER PAGE … set (Action: …) on <gallery>` was refused —
// "has no on-click action — Action can only be set on a widget that has one" —
// while `set ('onClick': …)` wrote the same slot. `Action:` reached the setter
// for a built-in widget's click action, which a pluggable widget does not have;
// its click is the property its definition sources from `onClick:`. And no
// ALTER spelling applied MDL-WIDGET36, so a row click on a single-click
// selection was written and `mx check` then failed the page with "The item
// click action is ambiguous".

const issue854GallerySetup = `create persistent entity MyFirstModule.Card (Title: String(100));
create nanoflow MyFirstModule.ACT_Open () begin end;
create page MyFirstModule.Cards (Title: 'Cards', Layout: Atlas_Core.Atlas_Default) {
  gallery gNone (DataSource: database from MyFirstModule.Card, Selection: None) {
    template { dynamictext t1 (Content: 'x') }
  }
  gallery gSingle (DataSource: database from MyFirstModule.Card) {
    template { dynamictext t2 (Content: 'y') }
  }
  actionbutton btnGo (Caption: 'Go')
};`

func galleryClickType(t *testing.T, exec *Executor, id, gallery string) string {
	t.Helper()
	return bsonnav.DGetString(pluggableSlotAction(t, namedDoc(t, storedUnitDoc(t, exec, id), gallery), "onClick"), "$Type")
}

func TestIssue854_AlterSetActionOnGallery(t *testing.T) {
	for _, spelling := range []string{"Action", "onClick", "'onClick'"} {
		t.Run(spelling, func(t *testing.T) {
			exec, _ := pageFixtureExecutor(t)
			run(t, exec, issue854GallerySetup)
			id := unitIDByName(t, exec, "page", "Cards")
			if err := runProgErr(t, exec, "alter page MyFirstModule.Cards { set ("+spelling+": call nanoflow MyFirstModule.ACT_Open()) on gNone };"); err != nil {
				t.Fatalf("set %s on a gallery: %v", spelling, err)
			}
			if got := galleryClickType(t, exec, id, "gNone"); got != "Forms$CallNanoflowClientAction" {
				t.Errorf("gNone.onClick holds %s, want the nanoflow call", got)
			}
		})
	}
}

// Controls: a built-in widget's click keeps its own setter, and a pluggable
// widget with no click slot is still refused.
func TestIssue854_AlterSetActionControls(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue854GallerySetup)
	id := unitIDByName(t, exec, "page", "Cards")
	run(t, exec, "alter page MyFirstModule.Cards { set (Action: call nanoflow MyFirstModule.ACT_Open()) on btnGo };")
	btn := namedDoc(t, storedUnitDoc(t, exec, id), "btnGo")
	if got := bsonnav.DGetString(bsonnav.DGetDoc(btn, "Action"), "$Type"); got != "Forms$CallNanoflowClientAction" {
		t.Errorf("btnGo.Action holds %s, want the nanoflow call", got)
	}

	run(t, exec, `create enumeration MyFirstModule.Colour (Red 'Red');
create persistent entity MyFirstModule.Thing (Colour: Enumeration(MyFirstModule.Colour));
create page MyFirstModule.ThingEdit (Title: 'Thing', Layout: Atlas_Core.Atlas_Default, Params: ($Thing: MyFirstModule.Thing)) {
  dataview dv (DataSource: $Thing) { combobox cb (Label: 'Colour', Attribute: Colour) }
};`)
	err := runProgErr(t, exec, "alter page MyFirstModule.ThingEdit { set (Action: call nanoflow MyFirstModule.ACT_Open()) on cb };")
	if err == nil {
		t.Fatal("a combo box has no click slot, and the set was accepted")
	}
}

// A row click on the default single-click selection is the gallery's own
// "ambiguous" error: refused under mdl 1 with nothing written, written with
// the MDL-V1-GALLERYCLICK warning under mdl 0 — what CREATE does.
func TestIssue854_AlterSetGalleryClickAmbiguity(t *testing.T) {
	for _, spelling := range []string{"Action", "'onClick'"} {
		t.Run("mdl 1 "+spelling, func(t *testing.T) {
			exec, _ := pageFixtureExecutor(t)
			run(t, exec, issue854GallerySetup)
			id := unitIDByName(t, exec, "page", "Cards")
			err := runProgErr(t, exec, "mdl 1;\nalter page MyFirstModule.Cards { set ("+spelling+": call nanoflow MyFirstModule.ACT_Open()) on gSingle };")
			if err == nil || !strings.Contains(err.Error(), "MDL-WIDGET36") {
				t.Fatalf("mdl 1: got %v, want the MDL-WIDGET36 refusal", err)
			}
			if got := galleryClickType(t, exec, id, "gSingle"); got != "Forms$NoAction" {
				t.Errorf("the refused set still wrote %s", got)
			}
		})
	}
	t.Run("mdl 0", func(t *testing.T) {
		exec, out := pageFixtureExecutor(t)
		run(t, exec, issue854GallerySetup)
		id := unitIDByName(t, exec, "page", "Cards")
		out.Reset()
		run(t, exec, "alter page MyFirstModule.Cards { set (Action: call nanoflow MyFirstModule.ACT_Open()) on gSingle };")
		if got := galleryClickType(t, exec, id, "gSingle"); got != "Forms$CallNanoflowClientAction" {
			t.Errorf("mdl 0: gSingle.onClick holds %s, want the action written", got)
		}
		if !strings.Contains(out.String(), galleryClickRefused.Code) {
			t.Errorf("mdl 0: no %s warning:\n%s", galleryClickRefused.Code, out.String())
		}
	})
	// Control: Selection None is not ambiguous, under either version.
	t.Run("selection none", func(t *testing.T) {
		exec, out := pageFixtureExecutor(t)
		run(t, exec, issue854GallerySetup)
		out.Reset()
		if err := runProgErr(t, exec, "mdl 1;\nalter page MyFirstModule.Cards { set (Action: call nanoflow MyFirstModule.ACT_Open()) on gNone };"); err != nil {
			t.Fatalf("a row click on Selection None was refused: %v", err)
		}
		if strings.Contains(out.String(), galleryClickRefused.Code) {
			t.Errorf("Selection None warned:\n%s", out.String())
		}
	})
}

// check predicts it: an error under mdl 1, a warning under mdl 0, nothing for
// Selection None.
func TestIssue854_CheckAlterSetGalleryClickAmbiguity(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue854GallerySetup)
	const set = "alter page MyFirstModule.Cards { set (Action: call nanoflow MyFirstModule.ACT_Open()) on %s };"
	validate := func(src string) ([]error, []string) {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		return exec.ValidateProgramWithWarnings(prog)
	}
	errs, _ := validate("mdl 1;\n" + strings.Replace(set, "%s", "gSingle", 1))
	if !anyErrContains854(errs, "MDL-WIDGET36") {
		t.Errorf("mdl 1: check did not refuse the ambiguous click: %v", errs)
	}
	errs, warns := validate(strings.Replace(set, "%s", "gSingle", 1))
	if len(errs) != 0 || !anyContains854(warns, galleryClickRefused.Code) {
		t.Errorf("mdl 0: errors %v, warnings %v; want only the %s warning", errs, warns, galleryClickRefused.Code)
	}
	errs, warns = validate("mdl 1;\n" + strings.Replace(set, "%s", "gNone", 1))
	if len(errs) != 0 || anyContains854(warns, galleryClickRefused.Code) {
		t.Errorf("Selection None: errors %v, warnings %v; want neither", errs, warns)
	}
}

// runProgErr runs mdl as one program, so its `mdl 1;` header applies.
func runProgErr(t *testing.T, exec *Executor, mdl string) error {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", mdl, errs)
	}
	return exec.ExecuteProgram(prog)
}

func anyErrContains854(errs []error, needle string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), needle) {
			return true
		}
	}
	return false
}

func anyContains854(ss []string, needle string) bool {
	for _, s := range ss {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
