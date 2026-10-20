// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// Only what an alter states is validated: a glyph the font does not define is
// refused on an item it inserts or an icon it sets (MDL078), and an inserted
// item without an icon is warned about (MDL077) — but a `set` naming an
// existing item is not, and nothing stored is looked at. That is the TestApp
// case: one caption change in a Studio Pro menu was refused over glyph 9993
// on an item the statement never named.
func TestAlterNavigation_ValidatesOnlyWhatItStates(t *testing.T) {
	prog := parseMDL(t, `alter navigation Responsive {
  insert after 'Home' { menu item 'Bad' ( OnClick: sign out, Icon: glyph 9993 ) }
  set ( Icon: glyph 9994 ) on 'Help'
  set ( Caption: 'Start' ) on 'Home'
}`)
	var codes []string
	for _, v := range validateGlyphCodes(prog.Statements[0]) {
		codes = append(codes, v.RuleID+":"+v.Message)
	}
	joined := strings.Join(codes, "\n")
	if len(codes) != 2 || !strings.Contains(joined, "9993") || !strings.Contains(joined, "9994") {
		t.Errorf("MDL078 findings = %v, want the inserted 9993 and the set 9994", codes)
	}
	icons := validateMenuItemIcons(parseMDL(t, `alter menu M.Menu {
  insert { menu item 'NoIcon' ( OnClick: sign out ) }
  set ( Caption: 'Renamed' ) on 'Existing'
}`).Statements[0])
	if len(icons) != 1 || !strings.Contains(icons[0].Message, "NoIcon") {
		t.Errorf("MDL077 findings = %+v, want one, for the inserted item", icons)
	}
	clean := parseMDL(t, `alter navigation Responsive { set ( Caption: 'Start' ) on 'Item 4' }`)
	if v := validateGlyphCodes(clean.Statements[0]); len(v) != 0 {
		t.Errorf("a caption change was judged on icons: %v", v)
	}
}
