// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#749 (R12): describe does not invent names Mendix does not store,
// and alter page addresses a DataGrid 2 column explicitly. Run on the Studio
// Pro-authored PedApp fixture: Administration.Account_Overview has layout
// grids, a DataGrid 2 with a control bar, and two columns over FullName — the
// duplicate the issue reports.

const accountOverview = "Administration.Account_Overview"

func describePedAppPage(t *testing.T, name string) string {
	t.Helper()
	exec, out := openPedAppFixture(t)
	return describeOn(t, exec, out, name)
}

// openPedAppWithWidgets is openPedAppFixture plus the fixture's widgets/
// folder, which a page holding a DataGrid 2 needs to be built.
func openPedAppWithWidgets(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	exec, out := openPedAppFixture(t)
	dir := filepath.Dir(exec.newExecContext(context.Background()).Backend.Path())
	if err := copyPedAppTree(filepath.Join("..", "..", "testdata", "pedapp", "widgets"), filepath.Join(dir, "widgets")); err != nil {
		t.Fatal(err)
	}
	// Reconnect so the widget registry is read from the copied folder.
	path := exec.newExecContext(context.Background()).Backend.Path()
	if err := exec.Execute(&ast.DisconnectStmt{}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Execute(&ast.ConnectStmt{Path: path}); err != nil {
		t.Fatal(err)
	}
	return exec, out
}

func describeOn(t *testing.T, exec *Executor, out *bytes.Buffer, name string) string {
	t.Helper()
	out.Reset()
	if err := afRun(t, exec, "describe page "+name+";"); err != nil {
		t.Fatalf("describe page %s: %v", name, err)
	}
	return out.String()
}

// synthetic matches a name describe used to invent: row1, col3, controlBar1,
// filter1, template1 — each written after the keyword of an element Mendix
// stores no name for.
var synthetic = regexp.MustCompile(`(?m)^\s*(row row\d+|column col\d+|controlbar controlBar\d+|filter filter\d+|template template\d+)\b`)

func TestDescribePage_NoSyntheticNames(t *testing.T) {
	got := describePedAppPage(t, accountOverview)
	if m := synthetic.FindAllString(got, -1); len(m) > 0 {
		t.Errorf("describe invents names Mendix does not store: %q\n%s", m, got)
	}
	for _, want := range []string{
		"row {",
		"column (DesktopWidth: AutoFill) {",
		"controlbar {",
		"column (Attribute: FullName, Caption: 'Full name') {",
		"column (Attribute: Name, Caption: 'Login') {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe should contain %q:\n%s", want, got)
		}
	}
	// A DataGrid 2 column's name was derived from its attribute, so the two
	// FullName columns were both `column FullName`.
	if strings.Contains(got, "column FullName") {
		t.Errorf("describe still names DataGrid 2 columns:\n%s", got)
	}
}

func TestAlterPage_ColumnAddress(t *testing.T) {
	exec, out := openPedAppFixture(t)

	if err := afRun(t, exec, `alter page `+accountOverview+` { set (Caption: 'User name') on dataGrid21 column(Name); };`); err != nil {
		t.Fatalf("set on dataGrid21 column(Name): %v", err)
	}
	if got := describeOn(t, exec, out, accountOverview); !strings.Contains(got, "column (Attribute: Name, Caption: 'User name')") {
		t.Errorf("the Name column's caption was not set:\n%s", got)
	}

	// Two columns show FullName; the address matches both and is refused, and
	// nothing changes.
	before := describeOn(t, exec, out, accountOverview)
	err := afRun(t, exec, `alter page `+accountOverview+` { drop dataGrid21 column(FullName); };`)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "dataGrid21 column(FullName)@2") {
		t.Fatalf("drop dataGrid21 column(FullName) should be refused as ambiguous, listing @2; got %v", err)
	}
	if after := describeOn(t, exec, out, accountOverview); after != before {
		t.Errorf("a refused alter changed the page")
	}

	// @2 picks the second: the custom-content column captioned ' '.
	if err := afRun(t, exec, `alter page `+accountOverview+` { drop dataGrid21 column(FullName)@2; };`); err != nil {
		t.Fatalf("drop dataGrid21 column(FullName)@2: %v", err)
	}
	got := describeOn(t, exec, out, accountOverview)
	if n := strings.Count(got, "column (Attribute: FullName"); n != 1 {
		t.Errorf("after dropping column(FullName)@2, %d FullName columns remain, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, "column (Attribute: FullName, Caption: 'Full name')") {
		t.Errorf("@2 dropped the wrong column:\n%s", got)
	}
}

// The name is optional in the grammar, and an element Mendix stores no name for
// is written without one. Executing that is the same page as executing it with
// the old invented names.
func TestCreatePage_NamelessElements(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	src := `create page MyFirstModule.Nameless (Title: 'Nameless', Layout: Atlas_Core.Atlas_Default) {
  layoutgrid lg {
    row {
      column (DesktopWidth: 6) {
        dynamictext txtLeft (Content: 'Left')
      }
      column (DesktopWidth: 6) {
        datagrid dgAccounts (DataSource: database from Administration.Account) {
          controlbar {
            actionbutton btnNew (Caption: 'New', Action: microflow Administration.NewAccount)
          }
          column (Attribute: FullName, Caption: 'Full name')
          column (Attribute: FullName, Caption: 'Again')
          column (Attribute: Name)
        }
        gallery galAccounts (DataSource: database from Administration.Account) {
          filter {
            textfilter tfName (Attributes: [Administration.Account.FullName])
          }
          template {
            dynamictext txtFullName (Content: '{1}', ContentParams: [{1} = FullName])
          }
        }
      }
    }
  }
};`
	if err := afRun(t, exec, src); err != nil {
		t.Fatalf("create page with nameless rows, columns and grid columns: %v", err)
	}
	got := describeOn(t, exec, out, "MyFirstModule.Nameless")
	if m := synthetic.FindAllString(got, -1); len(m) > 0 {
		t.Errorf("describe invents names: %q\n%s", m, got)
	}
	for _, want := range []string{
		"row {",
		"column (DesktopWidth: 6) {",
		"controlbar {",
		"actionbutton btnNew",
		"column (Attribute: FullName, Caption: 'Full name')",
		"column (Attribute: FullName, Caption: 'Again')",
		"column (Attribute: Name, Caption: 'Name')",
		"filter {",
		"template {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe should contain %q:\n%s", want, got)
		}
	}
}

// Where Mendix DOES store a name, leaving it out is refused when the widget is
// built — not silently written as an empty name.
func TestCreatePage_NamedWidgetWithoutNameIsRefused(t *testing.T) {
	for _, body := range []string{
		`dynamictext (Content: 'x')`,
		`container { dynamictext t (Content: 'x') }`,
		`row { column (DesktopWidth: 12) { dynamictext t (Content: 'x') } }`, // a row outside a layout grid is a container
		`layoutgrid { row { column (DesktopWidth: 12) { dynamictext t (Content: 'x') } } }`,
	} {
		exec, _ := openPedAppFixture(t)
		err := afRun(t, exec, `create page MyFirstModule.NoName (Title: 'x', Layout: Atlas_Core.Atlas_Default) { `+body+` };`)
		if err == nil || !strings.Contains(err.Error(), "needs a name") {
			t.Errorf("%s: want a 'needs a name' refusal, got %v", body, err)
		}
	}
}
