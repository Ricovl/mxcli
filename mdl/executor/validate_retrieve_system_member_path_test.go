// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1358 follow-up (ako/mxcli#1306): MDL-RETRIEVE02 rewrites
// `retrieve $u from $o/System.owner` to `retrieve $u from System.User where
// [id = $o/System.owner] first;`, and that rewrite passed `check -p` and exec
// and then failed the build when $o's entity does not store its owner:
//
//	[error] [CE0161] "Error(s) in XPath constraint." at Retrieve object(s) activity 'Retrieve User from database'
//
// Every row below was measured with mx check on 11.12.5.
const systemMemberPathHead = `mdl 1;
create module B1306;
create persistent entity B1306.Owned (Number: String(50), owner: AutoOwner, changedBy: AutoChangedBy);
create persistent entity B1306.Plain (Number: String(50));
create persistent entity B1306.Doc extends System.FileDocument (Title: String(50));
create persistent entity B1306.SubOwned extends B1306.Owned (Extra: String(50));
create persistent entity B1306.SubPlain extends B1306.Plain (Extra: String(50));
`

const systemMemberPathFlow = `create microflow B1306.SUB_Owner ($o: %s) returns System.User as $u
begin
  retrieve $u from System.User where %s first;
end;
`

var systemMemberPathCases = []struct {
	entity, where string
	want          []string // nil = builds clean, must pass check
}{
	// The reported shape, and its changedBy twin.
	{"B1306.Plain", "[id = $o/System.owner]", []string{
		"$o/System.owner", "B1306.Plain does not store System.owner", "CE0161",
		"alter entity B1306.Plain add attribute owner: AutoOwner"}},
	{"B1306.Plain", "[id = $o/System.changedBy]", []string{
		"B1306.Plain does not store System.changedBy", "alter entity B1306.Plain add attribute changedBy: AutoChangedBy"}},
	// A specialization is judged by its root, and the fix goes on the root.
	{"B1306.SubPlain", "[id = $o/System.owner]", []string{
		"root B1306.Plain does not store System.owner", "alter entity B1306.Plain add attribute owner: AutoOwner"}},
	// Controls, each measured clean.
	{"B1306.Owned", "[id = $o/System.owner]", nil},
	{"B1306.Owned", "[id = $o/System.changedBy]", nil},
	{"B1306.Doc", "[id = $o/System.owner]", nil},      // System.FileDocument stores it
	{"B1306.SubOwned", "[id = $o/System.owner]", nil}, // the root stores it
}

func TestRetrieveSystemMemberPath_ScriptEntity(t *testing.T) {
	for _, c := range systemMemberPathCases {
		exec, _, dir := openPedAppCopy(t)
		src := systemMemberPathHead + fmt.Sprintf(systemMemberPathFlow, c.entity, c.where)
		got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
		if c.want == nil {
			if got != "" {
				t.Errorf("$o: %s, %s — mxbuild builds this clean; check reported:\n%s", c.entity, c.where, got)
			}
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("$o: %s, %s — check -p must report %q, reported:\n%s", c.entity, c.where, w, got)
			}
		}
	}
}

// The entity already in the project, the microflow in a later script: the
// flags come from the stored domain model.
func TestRetrieveSystemMemberPath_ProjectEntity(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, systemMemberPathHead); err != nil {
		t.Fatalf("setup: %v", err)
	}
	bad := "mdl 1;\n" + fmt.Sprintf(systemMemberPathFlow, "B1306.Plain", "[id = $o/System.owner]")
	if got := strings.Join(agreeCheck(t, exec, dir, bad), "\n"); !strings.Contains(got, "B1306.Plain does not store System.owner") {
		t.Errorf("check -p must report the stored entity's missing owner, reported:\n%s", got)
	}
	good := "mdl 1;\n" + fmt.Sprintf(systemMemberPathFlow, "B1306.Owned", "[id = $o/System.owner]")
	if got := agreeCheck(t, exec, dir, good); len(got) != 0 {
		t.Errorf("control: a stored entity that stores owner must pass, reported:\n%s", strings.Join(got, "\n"))
	}
}

// A variable a create object binds is typed as well as a parameter (measured:
// CE0161), and one whose entity the text does not show is not judged.
func TestRetrieveSystemMemberPath_VariableKinds(t *testing.T) {
	cases := []struct {
		name, flow, want string
	}{
		{"created object", `create microflow B1306.SUB_New () returns System.User as $u
begin
  $p = create B1306.Plain (Number = 'x');
  retrieve $u from System.User where [id = $p/System.owner] first;
end;
`, "B1306.Plain does not store System.owner"},
		// #641's rule, on an entity the same script declares: it judged only
		// stored entities, so this shape passed check and exec (measured CE0161).
		{"own member on a script entity", `create microflow B1306.SUB_Mine ()
begin
  retrieve $l from B1306.Plain where [System.owner = '[%CurrentUser%]'];
end;
`, "does not store owner"},
		{"own member, entity stores it", `create microflow B1306.SUB_MineOk ()
begin
  retrieve $l from B1306.Owned where [System.owner = '[%CurrentUser%]'];
end;
`, ""},
		// The entity an association retrieve yields needs the project; the
		// rule stays silent rather than guess.
		{"association-retrieved object", `create association B1306.Owned_Plain from B1306.Owned to B1306.Plain;
create microflow B1306.SUB_Assoc ($o: B1306.Owned) returns System.User as $u
begin
  retrieve $x from $o/B1306.Owned_Plain;
  retrieve $u from System.User where [id = $x/System.owner] first;
end;
`, ""},
		// An entity the script alters may gain the member there.
		{"altered entity", `alter entity B1306.Plain add attribute owner: AutoOwner;
create microflow B1306.SUB_Alt ($o: B1306.Plain) returns System.User as $u
begin
  retrieve $u from System.User where [id = $o/System.owner] first;
end;
`, ""},
	}
	for _, c := range cases {
		exec, _, dir := openPedAppCopy(t)
		got := strings.Join(agreeCheck(t, exec, dir, systemMemberPathHead+c.flow), "\n")
		if c.want == "" {
			if got != "" {
				t.Errorf("%s: control must pass check, reported:\n%s", c.name, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) || !strings.Contains(got, "CE0161") {
			t.Errorf("%s: check -p must report %q (CE0161), reported:\n%s", c.name, c.want, got)
		}
	}
}

// MDL-RETRIEVE02's rewrite builds only when the entity stores the member, so
// the suggestion has to say so — it is what the follow-up on #1358 applied.
func TestRetrieveOverSystemAssociation_SuggestionNamesPrecondition(t *testing.T) {
	got := systemAssocRetrieveViolations(t, mfWith(&ast.RetrieveStmt{
		Variable: "u", StartVariable: "task", Source: ast.QualifiedName{Module: "System", Name: "owner"},
	}))
	if len(got) != 1 {
		t.Fatalf("want one violation, got %v", got)
	}
	for _, want := range []string{"[id = $task/System.owner]", "owner: AutoOwner", "CE0161"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("suggestion does not mention %q: %s", want, got[0])
		}
	}
}
