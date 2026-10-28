// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#854: an enumeration property of a pluggable widget was written
// exactly as the script spelt it. `onClickTrigger: Double` stored "Double", and
// the Gallery compares its trigger with `"single" === clickTrigger` /
// `"double" === clickTrigger` — so neither matched and the row action never ran
// on a click, while `mx check` reported 0 errors (measured on 11.13.0, also for
// "triple" and for a data grid's `pagination: 'Buttons'`). Nothing checked the
// value against the keys the widget declares.

const issue854EnumSetup = `create persistent entity MyFirstModule.Card (Title: String(100));
create nanoflow MyFirstModule.ACT_Open () begin end;`

func issue854EnumPage(trigger, pagination string) string {
	return `create or modify page MyFirstModule.Cards (Title: 'Cards', Layout: Atlas_Core.Atlas_Default) {
  gallery gCards (DataSource: database from MyFirstModule.Card, Selection: Single, onClickTrigger: ` + trigger + `, onClick: call nanoflow MyFirstModule.ACT_Open()) {
    template { dynamictext t1 (Content: 'x') }
  }
  datagrid dgCards (DataSource: database from MyFirstModule.Card, pagination: ` + pagination + `) {
    column (Attribute: Title, Caption: 'Title')
  }
};`
}

func storedPrimitive854(t *testing.T, exec *Executor, id, widget, key string) string {
	t.Helper()
	return bsonnav.DGetString(widgetProp(t, namedDoc(t, storedUnitDoc(t, exec, id), widget), key), "PrimitiveValue")
}

// A key written in another case is stored as the key, under either version —
// a generated definition's (the data grid) as well as a hand-written one's.
func TestIssue854_EnumValueIsStoredAsItsKey(t *testing.T) {
	for _, header := range []string{"", "mdl 1;\n"} {
		exec, _ := pageFixtureExecutor(t)
		run(t, exec, issue854EnumSetup)
		if err := runProgErr(t, exec, header+issue854EnumPage("Double", "'Buttons'")); err != nil {
			t.Fatalf("%q: %v", header, err)
		}
		id := unitIDByName(t, exec, "page", "Cards")
		if got := storedPrimitive854(t, exec, id, "gCards", "onClickTrigger"); got != "double" {
			t.Errorf("%q: onClickTrigger stored %q, want the key \"double\"", header, got)
		}
		if got := storedPrimitive854(t, exec, id, "dgCards", "pagination"); got != "buttons" {
			t.Errorf("%q: pagination stored %q, want the key \"buttons\"", header, got)
		}
	}
}

// A value the widget declares no key for is refused under mdl 1 (MDL-WIDGET08)
// with nothing written, and written as given with the MDL-V1-WIDGETENUM warning
// under mdl 0.
func TestIssue854_UndeclaredEnumValue(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854EnumSetup)
	err := runProgErr(t, exec, "mdl 1;\n"+issue854EnumPage("triple", "'buttons'"))
	if err == nil || !strings.Contains(err.Error(), "MDL-WIDGET08") || !strings.Contains(err.Error(), "single, double") {
		t.Fatalf("mdl 1: got %v, want the MDL-WIDGET08 refusal naming the keys", err)
	}
	if pages, _ := exec.backend.ListPages(); len(pages) > 0 {
		for _, p := range pages {
			if p.Name == "Cards" {
				t.Error("mdl 1: the refused page was written")
			}
		}
	}

	out.Reset()
	run(t, exec, issue854EnumPage("triple", "'buttons'"))
	id := unitIDByName(t, exec, "page", "Cards")
	if got := storedPrimitive854(t, exec, id, "gCards", "onClickTrigger"); got != "triple" {
		t.Errorf("mdl 0: onClickTrigger stored %q, want it written as given", got)
	}
	if !strings.Contains(out.String(), widgetEnumRefused.Code) {
		t.Errorf("mdl 0: no %s warning:\n%s", widgetEnumRefused.Code, out.String())
	}
}

// check -p predicts it from the installed package: an error under mdl 1, the
// gated warning under mdl 0, nothing for a key in another case.
func TestIssue854_CheckUndeclaredEnumValue(t *testing.T) {
	project := projectFixture(t)
	violations := func(src string) []linter.Violation {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		return ValidateWidgetProperties(prog, project)
	}
	find := func(vs []linter.Violation, rule string) *linter.Violation {
		for i := range vs {
			if vs[i].RuleID == rule && strings.Contains(vs[i].Message, "onClickTrigger") {
				return &vs[i]
			}
		}
		return nil
	}
	if v := find(violations("mdl 1;\n"+issue854EnumPage("triple", "'buttons'")), "MDL-WIDGET08"); v == nil || v.Severity != linter.SeverityError {
		t.Errorf("mdl 1: want an MDL-WIDGET08 error for onClickTrigger triple, got %+v", v)
	}
	if v := find(violations(issue854EnumPage("triple", "'buttons'")), widgetEnumRefused.Code); v == nil || v.Severity != linter.SeverityWarning {
		t.Errorf("mdl 0: want the %s warning, got %+v", widgetEnumRefused.Code, v)
	}
	for _, vs := range [][]linter.Violation{violations("mdl 1;\n" + issue854EnumPage("Double", "'Buttons'"))} {
		for _, v := range vs {
			if v.RuleID == "MDL-WIDGET08" || v.RuleID == widgetEnumRefused.Code {
				t.Errorf("a declared key in another case was reported: %s %s", v.RuleID, v.Message)
			}
		}
	}
}

// ALTER … SET writes the key too, and refuses an undeclared value the same way.
func TestIssue854_AlterSetEnumValue(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue854EnumSetup)
	run(t, exec, issue854EnumPage("double", "'buttons'"))
	id := unitIDByName(t, exec, "page", "Cards")

	run(t, exec, "alter page MyFirstModule.Cards { set (pagination: 'VirtualScrolling') on dgCards };")
	if got := storedPrimitive854(t, exec, id, "dgCards", "pagination"); got != "virtualScrolling" {
		t.Errorf("set pagination stored %q, want the key \"virtualScrolling\"", got)
	}

	err := runProgErr(t, exec, "mdl 1;\nalter page MyFirstModule.Cards { set (pagination: 'pages') on dgCards };")
	if err == nil || !strings.Contains(err.Error(), "MDL-WIDGET08") {
		t.Fatalf("mdl 1: got %v, want the MDL-WIDGET08 refusal", err)
	}
	if got := storedPrimitive854(t, exec, id, "dgCards", "pagination"); got != "virtualScrolling" {
		t.Errorf("the refused set still wrote %q", got)
	}

	out.Reset()
	run(t, exec, "alter page MyFirstModule.Cards { set (pagination: 'pages') on dgCards };")
	if got := storedPrimitive854(t, exec, id, "dgCards", "pagination"); got != "pages" {
		t.Errorf("mdl 0: stored %q, want it written as given", got)
	}
	if !strings.Contains(out.String(), widgetEnumRefused.Code) {
		t.Errorf("mdl 0: no %s warning:\n%s", widgetEnumRefused.Code, out.String())
	}
}
