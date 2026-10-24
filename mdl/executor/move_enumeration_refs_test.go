// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#1674: MOVE ENUMERATION to another module reported success and left
// every reference outside the domain models naming the old module — mx check:
// CE1613 at the parameter, the variable's type and value, the end event and the
// return type. execMove returned from the enumeration case before the
// cross-module reference sweep that every other document's move runs, and
// moveEnumeration itself updated only domain-model attribute types.
func TestMoveEnumerationAcrossModules_RewritesItsReferences(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	setup := `create module Shop1674;
create module Other1674;
create enumeration Shop1674.Status (Open 'Open', Closed 'Closed');
create persistent entity Shop1674.Order (Number: String(20), State: enum Shop1674.Status);
create microflow Shop1674.UseStatus ($In: enum Shop1674.Status) returns enum Shop1674.Status
begin
  declare $S enum Shop1674.Status = Shop1674.Status.Open;
  if $In = empty then
    return Shop1674.Status.Closed;
  end if;
  return $S;
end;`
	if err := afRun(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	out.Reset()
	if err := afRun(t, exec, "move enumeration Shop1674.Status to Other1674;"); err != nil {
		t.Fatalf("move: %v\n%s", err, out.String())
	}
	report := out.String()

	describe := func(stmt string) string {
		t.Helper()
		out.Reset()
		if err := afRun(t, exec, stmt); err != nil {
			t.Fatalf("%s: %v\n%s", stmt, err, out.String())
		}
		return out.String()
	}

	// Control: the domain model was always updated.
	if entity := describe("describe entity Shop1674.Order;"); !strings.Contains(entity, "Other1674.Status") {
		t.Fatalf("control: the entity attribute does not name the moved enumeration:\n%s", entity)
	}

	mf := describe("describe microflow Shop1674.UseStatus;")
	for _, want := range []string{
		"$In: enum Other1674.Status",
		"returns enum Other1674.Status",
		"declare $S enum Other1674.Status = Other1674.Status.Open;",
		"return Other1674.Status.Closed;",
	} {
		if !strings.Contains(mf, want) {
			t.Errorf("after the move, describe lacks %q:\n%s", want, mf)
		}
	}
	if strings.Contains(mf, "Shop1674.Status") {
		t.Errorf("the microflow still names Shop1674.Status after the move:\n%s\nmove printed:\n%s", mf, report)
	}
	if !strings.Contains(report, "Updated references in") {
		t.Errorf("the move should report the documents whose references it updated:\n%s", report)
	}
}
