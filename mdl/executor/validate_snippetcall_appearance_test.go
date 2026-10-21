// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1511. A snippet call renders no element of its own: the
// snippet's widgets are inlined into the parent. Measured on Mendix 11.15.0
// (`run --local`, `playwright check`): a snippetcall carrying Class 'probe-x',
// Style 'border: 3px solid red', DynamicClasses 'probe-dyn' and the design
// properties 'Align self' / 'Hide on' put NONE of them in the DOM — the
// snippet's container was a direct child of the enclosing container — while a
// container on the same page with the same four settings carried all of them.
// check, exec and mx check were all clean, so nothing told the author.

func snippetCallRuleCount(t *testing.T, src string) int {
	t.Helper()
	n := 0
	for _, v := range ValidateWidgetProperties(parseMDL(t, src), "") {
		if v.RuleID == "MDL-WIDGET48" {
			n++
		}
	}
	return n
}

func TestCheck1511_SnippetCallAppearanceWarns(t *testing.T) {
	cases := map[string]string{
		"class":           `class: 'probe-x'`,
		"style":           `style: 'border: 3px solid red'`,
		"dynamic classes": `dynamicclasses: '''probe-dyn'''`,
		"design property": `designproperties: ('Align self': 'Right')`,
		"several at once": `class: 'a', style: 'color: red'`,
	}
	for name, props := range cases {
		t.Run(name, func(t *testing.T) {
			src := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  container c1 { snippetcall sc1 (snippet: M.S, ` + props + `) }
};`
			if got := snippetCallRuleCount(t, src); got != 1 {
				t.Errorf("MDL-WIDGET48 reported %d times, want 1", got)
			}
		})
	}
}

// The message has to say why and what to do instead, or it is one more
// warning to ignore.
func TestCheck1511_SnippetCallAppearanceMessage(t *testing.T) {
	src := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  snippetcall sc1 (snippet: M.S, class: 'x', style: 'color: red')
};`
	for _, v := range ValidateWidgetProperties(parseMDL(t, src), "") {
		if v.RuleID != "MDL-WIDGET48" {
			continue
		}
		for _, want := range []string{"sc1", "Class", "Style", "renders no element"} {
			if !strings.Contains(v.Message, want) {
				t.Errorf("message %q lacks %q", v.Message, want)
			}
		}
		if !strings.Contains(v.Suggestion, "container") {
			t.Errorf("suggestion %q does not point at a container", v.Suggestion)
		}
		return
	}
	t.Fatal("no MDL-WIDGET48 reported")
}

// The controls: a bare snippet call, and the same settings on a container,
// must stay silent. A rule that fires on the fix it recommends is worse than
// no rule.
func TestCheck1511_SnippetCallAppearanceControls(t *testing.T) {
	for name, src := range map[string]string{
		"bare snippet call": `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  snippetcall sc1 (snippet: M.S)
};`,
		"styled container around the call": `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  container c1 (class: 'probe-x', style: 'border: 1px solid', designproperties: ('Align self': 'Right')) {
    snippetcall sc1 (snippet: M.S)
  }
};`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := snippetCallRuleCount(t, src); got != 0 {
				t.Errorf("MDL-WIDGET48 reported %d times, want 0", got)
			}
		})
	}
}

// ALTER PAGE INSERT / REPLACE carry the widget, so they are checked without a
// project, like CREATE.
func TestCheck1511_SnippetCallAppearanceInAlterInsert(t *testing.T) {
	src := `alter page M.P { insert after c1 { snippetcall sc2 (snippet: M.S, class: 'x') } };`
	if got := snippetCallRuleCount(t, src); got != 1 {
		t.Errorf("MDL-WIDGET48 reported %d times, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// ALTER PAGE … SET and ALTER STYLING name a STORED widget: what it is lives
// only in the document, so these are judged in the reference pass.
// ---------------------------------------------------------------------------

func storedSnippetCallPage() bson.D {
	appearance := func() bson.D {
		return bson.D{
			{Key: "$Type", Value: "Forms$Appearance"},
			{Key: "Class", Value: ""},
			{Key: "DesignProperties", Value: bson.A{int32(3)}},
			{Key: "DynamicClasses", Value: ""},
			{Key: "Style", Value: ""},
		}
	}
	call := bson.D{
		{Key: "$Type", Value: "Forms$SnippetCallWidget"},
		{Key: "Name", Value: "snippetCall1"},
		{Key: "Appearance", Value: appearance()},
		{Key: "FormCall", Value: bson.D{{Key: "$Type", Value: "Forms$SnippetCall"}}},
	}
	div := bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "c1"},
		{Key: "Appearance", Value: appearance()},
		{Key: "Widgets", Value: bson.A{int32(2), call}},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), div}}},
			}},
		}},
	}
}

func snippetCallPageCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	pg := mkPage(mod.ID, "P")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedSnippetCallPage(), unitID, &countingDeps{}), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	ctx.ThemeRegistry = atlasListViewTheme()
	return ctx
}

func storedSnippetCallWarnings(t *testing.T, src string) []string {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	validateStoredSnippetCallAppearance(snippetCallPageCtx(t), prog, sc)
	var out []string
	for _, w := range sc.warnings {
		if strings.Contains(w, "MDL-WIDGET48") {
			out = append(out, w)
		}
	}
	return out
}

// The reported statement.
func TestCheck1511_AlterSetClassOnStoredSnippetCall(t *testing.T) {
	got := storedSnippetCallWarnings(t, `alter page M.P { set (Class: 'x') on snippetCall1 };`)
	if len(got) != 1 {
		t.Fatalf("want one MDL-WIDGET48 warning, got %v", got)
	}
	if !strings.Contains(got[0], "snippetCall1") || !strings.Contains(got[0], "Class") {
		t.Errorf("warning %q does not name the widget and the property", got[0])
	}
}

func TestCheck1511_AlterSetStyleAndDesignPropertyOnStoredSnippetCall(t *testing.T) {
	got := storedSnippetCallWarnings(t,
		`alter page M.P { set (Style: 'color: red', DynamicClasses: '''d''', 'Align self': 'Right') on snippetCall1 };`)
	if len(got) != 1 {
		t.Fatalf("want one MDL-WIDGET48 warning, got %v", got)
	}
	for _, want := range []string{"Style", "DynamicClasses", "Align self"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning %q does not name %s", got[0], want)
		}
	}
}

func TestCheck1511_AlterStylingOnStoredSnippetCall(t *testing.T) {
	got := storedSnippetCallWarnings(t, `alter styling on page M.P widget snippetCall1 set Class = 'x';`)
	if len(got) != 1 {
		t.Fatalf("want one MDL-WIDGET48 warning, got %v", got)
	}
}

// Controls: the same SET on the container around the call, and a SET of a
// property that is not appearance, are silent.
func TestCheck1511_StoredControlsAreSilent(t *testing.T) {
	for _, src := range []string{
		`alter page M.P { set (Class: 'x', Style: 'color: red') on c1 };`,
		`alter styling on page M.P widget c1 set Class = 'x';`,
		`alter page M.P { set (Class: '') on snippetCall1 };`,
	} {
		if got := storedSnippetCallWarnings(t, src); len(got) != 0 {
			t.Errorf("%s: want no MDL-WIDGET48, got %v", src, got)
		}
	}
}
