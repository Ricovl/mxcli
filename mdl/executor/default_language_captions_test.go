// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#944: making de_DE the default left Administration.Account_Overview's
// en_US-only tabPage2 caption empty in de_DE, and mxbuild refused it with CE4899
// while check, lint and exec said nothing. Measured on 11.14: tab page captions
// are the one caption kind the build requires.

// tabPageUnit is a stored page whose one tab page caption has text only in the
// given languages.
func tabPageUnit(t *testing.T, pageName, tabName string, langs ...string) []byte {
	t.Helper()
	items := bson.A{int32(3)}
	for _, l := range langs {
		items = append(items, bson.D{
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: l},
			{Key: "Text", Value: "Local Users"},
		})
	}
	raw, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: "page-" + pageName},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: pageName},
		{Key: "Widgets", Value: bson.A{bson.D{
			{Key: "$ID", Value: "tab-" + tabName},
			{Key: "$Type", Value: "Forms$TabPage"},
			{Key: "Name", Value: tabName},
			{Key: "Caption", Value: bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: items}}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// languageProject is a mock project with en_US and de_DE enabled, default
// en_US, holding Administration.Account_Overview with an en_US-only tabPage2.
func languageProject(t *testing.T) (*ExecContext, *mockLanguageState) {
	t.Helper()
	mod := mkModule("Administration")
	st := &mockLanguageState{ps: &model.ProjectSettings{Language: &model.LanguageSettings{
		DefaultLanguageCode: "en_US",
		Languages:           []model.Language{{Code: "en_US"}, {Code: "de_DE"}},
	}}}
	raw := tabPageUnit(t, "Account_Overview", "tabPage2", "en_US")
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return st.ps, nil },
		UpdateProjectSettingsFunc: func(ps *model.ProjectSettings) error {
			st.ps = ps
			return nil
		},
		ListUnitsFunc: func() ([]*types.UnitInfo, error) {
			return []*types.UnitInfo{{ID: "u1", ContainerID: mod.ID, Type: "Forms$Page"}}, nil
		},
		GetRawUnitBytesFunc: func(id model.ID) ([]byte, error) { return raw, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	st.out = buf
	return ctx, st
}

type mockLanguageState struct {
	ps  *model.ProjectSettings
	out interface{ String() string }
}

func switchDefault(lang string) *ast.AlterSettingsStmt {
	return &ast.AlterSettingsStmt{Section: "LANGUAGE", Properties: map[string]any{"DefaultLanguageCode": lang}}
}

// (c) The change of default is when the captions break, so it says so.
func TestAlterDefaultLanguage_ReportsRequiredCaptionsWithoutIt(t *testing.T) {
	ctx, st := languageProject(t)
	assertNoError(t, alterSettings(ctx, switchDefault("de_DE")))
	out := st.out.String()
	assertContainsStr(t, out, "1 required caption(s) have no de_DE text")
	assertContainsStr(t, out, `Administration.Account_Overview: tab page caption tabPage2 ("Local Users")`)
	assertContainsStr(t, out, "alter page Administration.Account_Overview { set (Caption: '…') on tabPage2; };")
}

// Control: switching to a language the caption has says nothing.
func TestAlterDefaultLanguage_SilentWhenCaptionsHaveIt(t *testing.T) {
	ctx, st := languageProject(t)
	st.ps.Language.DefaultLanguageCode = "de_DE"
	assertNoError(t, alterSettings(ctx, switchDefault("en_US")))
	if strings.Contains(st.out.String(), "required caption") {
		t.Errorf("every tab page has en_US; want no note, got:\n%s", st.out.String())
	}
}

// (b) The authoring language was resolved once per session, so a page created
// after the switch in the same script was still written in the old default —
// measured: its tab page then failed the de_DE build with CE4899 too.
func TestAlterDefaultLanguage_NewTextsUseTheNewDefault(t *testing.T) {
	ctx, _ := languageProject(t)
	if got := authoringLanguage(ctx); got != "en_US" {
		t.Fatalf("before: authoringLanguage = %q", got)
	}
	assertNoError(t, alterSettings(ctx, switchDefault("de_DE")))
	if got := authoringLanguage(ctx); got != "de_DE" {
		t.Errorf("after the switch: authoringLanguage = %q, want de_DE", got)
	}
}

func tabPageWidget(name string) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "TABCONTAINER", Name: "tc", Children: []*ast.WidgetV3{
		{Type: "TABPAGE", Name: name, Properties: map[string]any{"Caption": "Tab"}},
	}}
}

// (b) check -p foresees CE4899 for a script that changes the default: the
// stored page, and a page the script creates before the change. A page created
// after it is written in the new default and is fine.
func TestCheckDefaultLanguageCaptions(t *testing.T) {
	ctx, _ := languageProject(t)
	before := &ast.CreatePageStmtV3{Name: ast.QualifiedName{Module: "M", Name: "Before"}, Widgets: []*ast.WidgetV3{tabPageWidget("tpBefore")}}
	after := &ast.CreatePageStmtV3{Name: ast.QualifiedName{Module: "M", Name: "After"}, Widgets: []*ast.WidgetV3{tabPageWidget("tpAfter")}}
	prog := &ast.Program{Statements: []ast.Statement{before, switchDefault("de_DE"), after}}

	vs := CheckDefaultLanguageCaptions(ctx, prog)
	var msgs []string
	for _, v := range vs {
		if v.RuleID != defaultCaptionRule {
			t.Errorf("rule = %s", v.RuleID)
		}
		msgs = append(msgs, v.Message)
	}
	all := strings.Join(msgs, "\n")
	if len(vs) != 2 || !strings.Contains(all, "Administration.Account_Overview: tab page caption tabPage2") ||
		!strings.Contains(all, "M.Before: tab page caption tpBefore") || strings.Contains(all, "tpAfter") {
		t.Errorf("want the stored tabPage2 and tpBefore, not tpAfter; got:\n%s", all)
	}

	// Control: the same script without the switch reports nothing.
	prog = &ast.Program{Statements: []ast.Statement{before, after}}
	if vs := CheckDefaultLanguageCaptions(ctx, prog); len(vs) != 0 {
		t.Errorf("no default change; want nothing, got %+v", vs)
	}

	// A stored page the script rewrites after the switch is written in the new
	// default and is not reported.
	rewrite := &ast.CreatePageStmtV3{Name: ast.QualifiedName{Module: "Administration", Name: "Account_Overview"},
		Widgets: []*ast.WidgetV3{tabPageWidget("tabPage2")}}
	prog = &ast.Program{Statements: []ast.Statement{switchDefault("de_DE"), rewrite}}
	if vs := CheckDefaultLanguageCaptions(ctx, prog); len(vs) != 0 {
		t.Errorf("rewritten after the switch; want nothing, got %+v", vs)
	}
}
