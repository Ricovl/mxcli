// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// ako/mxcli#749: a name written on an element Mendix stores no name for is a
// deprecated spelling (MDL-DEPR005). It is reported only where the parent shows
// the name cannot be stored, and it is dropped from the AST, so the old and the
// canonical spelling build the same page.

const withOldNames = `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  layoutgrid lg {
    row row1 {
      column col1 (DesktopWidth: 6) {
        datagrid dg (DataSource: database from M.E) {
          controlbar controlBar1 { actionbutton btn (Caption: 'New') }
          column Name (Attribute: Name)
          column "Title"(Attribute: Title)
        }
      }
      column col2 (DesktopWidth: 6) {
        gallery gal (DataSource: database from M.E) {
          filter filter1 { textfilter tf }
          template template1 { dynamictext t (Content: 'x') }
        }
      }
    }
  }
};`

const withoutNames = `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  layoutgrid lg {
    row {
      column (DesktopWidth: 6) {
        datagrid dg (DataSource: database from M.E) {
          controlbar { actionbutton btn (Caption: 'New') }
          column (Attribute: Name)
          column(Attribute: Title)
        }
      }
      column (DesktopWidth: 6) {
        gallery gal (DataSource: database from M.E) {
          filter { textfilter tf }
          template { dynamictext t (Content: 'x') }
        }
      }
    }
  }
};`

func TestUnstoredWidgetNameIsDeprecated(t *testing.T) {
	old := mustBuild(t, withOldNames)
	var subjects []string
	for _, d := range old.Deprecations {
		if d.Code != deprecation.UnstoredWidgetName {
			t.Errorf("unexpected deprecation %s", d.Code)
			continue
		}
		subjects = append(subjects, d.Subject)
		if d.Fix == nil {
			t.Errorf("%s at line %d has no rewrite: %s", d.Code, d.Line, d.NoFix)
		}
	}
	want := []string{"row", "column", "controlbar", "column", "column", "column", "filter", "template"}
	if !sameMultiset(subjects, want) {
		t.Errorf("recorded %v, want (in any order) %v", subjects, want)
	}

	canon := mustBuild(t, withoutNames)
	if got := deprecationCodes(canon); len(got) != 0 {
		t.Errorf("the canonical spelling recorded %v", got)
	}
	if !reflect.DeepEqual(old.Statements, canon.Statements) {
		t.Errorf("the two spellings build different pages:\n old:   %#v\n canon: %#v", old.Statements, canon.Statements)
	}
}

// The name is kept, and nothing is reported, where the model does store it: a
// row or column outside a layout grid is built as a container with that name.
// A standalone row's columns are layout grid columns, though, and store none.
func TestStoredWidgetNamesAreKept(t *testing.T) {
	prog := mustBuild(t, `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  row rowTop { column colMain (DesktopWidth: 12) { dynamictext t (Content: 'x') } }
  container ctn { column colStandalone (DesktopWidth: 12) { } }
};`)
	if got := deprecationCodes(prog); !reflect.DeepEqual(got, []string{deprecation.UnstoredWidgetName}) {
		t.Errorf("recorded %v, want only the standalone row's column", got)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	row := page.Widgets[0]
	if row.Name != "rowTop" || row.Children[0].Name != "" {
		t.Errorf("standalone row %q (want rowTop), its column %q (want none)", row.Name, row.Children[0].Name)
	}
	if got := page.Widgets[1].Children[0].Name; got != "colStandalone" {
		t.Errorf("a column in a container lost its name: %q", got)
	}
}

func sameMultiset(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	count := map[string]int{}
	for _, s := range a {
		count[s]++
	}
	for _, s := range b {
		count[s]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}

// ako/mxcli#528: a data view's footer is a region — its widgets are stored in
// the data view's FooterWidgets and the footer has no Name — so `footer
// footerButtons { … }` inside a data view named nothing ALTER could find. It is
// the same deprecated spelling; a footer anywhere else is a container that
// keeps its name.
func TestDataViewFooterNameIsDeprecated(t *testing.T) {
	old := mustBuild(t, `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $E: M.E )) {
  dataview dvMain (DataSource: $E) {
    footer footerButtons { actionbutton btnSave (Caption: 'Save', Action: save changes) }
  }
  footer pageFooter { dynamictext t (Content: 'x') }
};`)
	canon := mustBuild(t, `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $E: M.E )) {
  dataview dvMain (DataSource: $E) {
    footer { actionbutton btnSave (Caption: 'Save', Action: save changes) }
  }
  footer pageFooter { dynamictext t (Content: 'x') }
};`)
	if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{deprecation.UnstoredWidgetName}) {
		t.Errorf("recorded %v, want one %s for the data view's footer", got, deprecation.UnstoredWidgetName)
	}
	if got := deprecationCodes(canon); len(got) != 0 {
		t.Errorf("the canonical spelling recorded %v", got)
	}
	if !reflect.DeepEqual(old.Statements, canon.Statements) {
		t.Errorf("the two spellings build different pages")
	}
	page := canon.Statements[0].(*ast.CreatePageStmtV3)
	if got := page.Widgets[1].Name; got != "pageFooter" {
		t.Errorf("a footer outside a data view lost its stored name: %q", got)
	}
}
