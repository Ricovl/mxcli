// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// ako/mxcli#749: `fmt --upgrade` drops the names describe used to invent for
// elements Mendix stores no name for, and only those — a standalone row keeps
// its name, because it is built as a container that stores it, while its
// column is a layout grid column and does not.
func TestUpgrade_DropsUnstoredWidgetNames(t *testing.T) {
	src := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  row rowTop { column colMain (DesktopWidth: 12) { } }
  layoutgrid layoutGrid1 {
    row row1 {
      column col1 (DesktopWidth: AutoFill) {
        datagrid dataGrid21 (DataSource: database from M.E) {
          controlbar controlBar1 {
            actionbutton actionButton1 (Caption: 'New')
          }
          column FullName (Attribute: FullName, Caption: 'Full name') {
            textfilter textFilter1
          }
          column "UserRoles/Name" (Attribute: UserRoles/Name)
        }
      }
    }
  }
};
`
	want := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  row rowTop { column (DesktopWidth: 12) { } }
  layoutgrid layoutGrid1 {
    row {
      column (DesktopWidth: AutoFill) {
        datagrid dataGrid21 (DataSource: database from M.E) {
          controlbar {
            actionbutton actionButton1 (Caption: 'New')
          }
          column (Attribute: FullName, Caption: 'Full name') {
            textfilter textFilter1
          }
          column (Attribute: UserRoles/Name)
        }
      }
    }
  }
};
`
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if got := res.Rewritten[deprecation.UnstoredWidgetName]; got != 6 {
		t.Errorf("Rewritten = %v, want 6 %s", res.Rewritten, deprecation.UnstoredWidgetName)
	}
	// Idempotent: the upgraded script has nothing left to rewrite.
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("a second upgrade changed the script again:\n%s", again.Source)
	}
}

// ako/mxcli#528: the name on a data view's footer is dropped; a footer outside
// a data view is a container and keeps it.
func TestUpgrade_DropsDataViewFooterName(t *testing.T) {
	src := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $E: M.E )) {
  dataview dvMain (DataSource: $E) {
    footer footer1 { actionbutton btnSave (Caption: 'Save', Action: save changes) }
  }
  footer pageFooter { dynamictext t (Content: 'x') }
};
`
	want := `create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $E: M.E )) {
  dataview dvMain (DataSource: $E) {
    footer { actionbutton btnSave (Caption: 'Save', Action: save changes) }
  }
  footer pageFooter { dynamictext t (Content: 'x') }
};
`
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
}
