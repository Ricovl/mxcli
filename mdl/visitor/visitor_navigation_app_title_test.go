// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1756 (mendixlabs/mxcli#1664): a navigation profile's application
// title and icon (Navigation$NavigationProfile.AppTitle / AppIcon) had no MDL
// form, so `app title 'EWO'` was "extraneous input 'app'" and every generated
// app kept "Mendix" in the browser tab.
func TestCreateNavigation_AppTitleAndIcon(t *testing.T) {
	for _, c := range []struct {
		src       string
		title     *string
		icon      *string
		homePages int
	}{
		{
			src:   "create or modify navigation Responsive app title 'EWO' app icon MyModule.Images.Logo home page MyModule.Home;",
			title: strPtr("EWO"), icon: strPtr("MyModule.Images.Logo"), homePages: 1,
		},
		// The clauses are order-free, like every other navigation clause.
		{
			src:   "create or modify navigation Responsive home page MyModule.Home app icon MyModule.Images.Logo app title 'EWO';",
			title: strPtr("EWO"), icon: strPtr("MyModule.Images.Logo"), homePages: 1,
		},
		// MDL string escaping: '' is one quote.
		{src: "create or modify navigation Responsive app title 'It''s mine';", title: strPtr("It's mine")},
		// An empty title is a value, not an omission.
		{src: "create or modify navigation Responsive app title '';", title: strPtr("")},
		// `none` clears the icon; it is not an image called "none".
		{src: "create or modify navigation Responsive app icon none;", icon: strPtr("")},
		// A keyword-colliding or hyphenated segment is double-quoted, as for any name.
		{src: `create or modify navigation Responsive app icon MyModule."Images".Logo;`, icon: strPtr("MyModule.Images.Logo")},
		// Omitted: nil, so the writer keeps what is stored.
		{src: "create or modify navigation Responsive home page MyModule.Home;", homePages: 1},
	} {
		prog, errs := Build(c.src)
		if len(errs) > 0 {
			t.Errorf("%s: parse errors: %v", c.src, errs)
			continue
		}
		stmt, ok := prog.Statements[0].(*ast.CreateNavigationStmt)
		if !ok {
			t.Errorf("%s: got %T", c.src, prog.Statements[0])
			continue
		}
		if !sameStrPtr(stmt.AppTitle, c.title) {
			t.Errorf("%s: AppTitle = %s, want %s", c.src, showStrPtr(stmt.AppTitle), showStrPtr(c.title))
		}
		if !sameStrPtr(stmt.AppIcon, c.icon) {
			t.Errorf("%s: AppIcon = %s, want %s", c.src, showStrPtr(stmt.AppIcon), showStrPtr(c.icon))
		}
		if len(stmt.HomePages) != c.homePages {
			t.Errorf("%s: %d home pages, want %d", c.src, len(stmt.HomePages), c.homePages)
		}
	}
}

// The clause is read only in that order of words: the spellings the reporter
// tried first stay errors rather than parsing into something else.
func TestCreateNavigation_AppTitleRejectsOtherSpellings(t *testing.T) {
	for _, src := range []string{
		"create or modify navigation Responsive title 'EWO';",
		"create or modify navigation Responsive AppTitle: 'EWO';",
		"create or modify navigation Responsive app title MyModule.Title;",
		"create or modify navigation Responsive app icon 'MyModule.Images.Logo';",
	} {
		if _, errs := Build(src); len(errs) == 0 {
			t.Errorf("%q was accepted", src)
		}
	}
}

// APP, ICON and TITLE were lexer tokens before the clause existed (`alter app
// security`, `progressive web app`, menu-item icons, page titles), so the
// clause adds no keyword. The names still parse where they did: as module,
// entity, attribute and enumeration-value names, unquoted.
func TestCreateNavigation_AppAndIconStayIdentifiers(t *testing.T) {
	for _, src := range []string{
		"create module App;",
		"create persistent entity App.Icon (App: String(200), Icon: String(200), Title: String(200));",
		"create persistent entity MyModule.app (app: String(20), icon: String(20), title: String(20));",
		"create enumeration MyModule.Kind (App 'App', Icon 'Icon', Title 'Title');",
		"alter entity MyModule.Thing add attribute Icon: String(50);",
		"create or modify navigation Responsive home page App.Icon_Overview;",
	} {
		if _, errs := Build(src); len(errs) > 0 {
			t.Errorf("%q: %v", src, errs)
		}
	}
}

func strPtr(s string) *string { return &s }

func sameStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func showStrPtr(p *string) string {
	if p == nil {
		return "nil"
	}
	return "'" + *p + "'"
}
