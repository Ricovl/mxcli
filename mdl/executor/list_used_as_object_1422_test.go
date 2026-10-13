// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1422: under `mdl 1;` `retrieve … limit 1` binds a list of
// one, and `change $E (…)` over it was accepted by check and written by exec
// with the member qualified through the variable's type — Attribute =
// "List of Module.Entity.Name", which mx check reports as CE1613 "The selected
// attribute 'List of …' no longer exists".

const lao1422Entity = `create persistent entity MyFirstModule.EntL1422 ( Name: string(100) );
`

func lao1422Flow(rng string) string {
	return `create or modify microflow MyFirstModule.MF_L1422 ()
begin
  retrieve $E from MyFirstModule.EntL1422 ` + rng + `;
  change $E ( Name = 'x' );
end;
`
}

// changeItemAttributes collects the Attribute of every stored change member
// (Microflows$ChangeActionItem), and when set != nil replaces it.
func changeItemAttributes(v any, set *string, out *[]string) any {
	switch x := v.(type) {
	case bson.D:
		typ, _ := afGet(x, "$Type").(string)
		item := strings.HasSuffix(typ, "$ChangeActionItem") || strings.HasSuffix(typ, "$MemberChange")
		for i := range x {
			if item && x[i].Key == "Attribute" {
				s, _ := x[i].Value.(string)
				*out = append(*out, s)
				if set != nil {
					x[i].Value = *set
				}
				continue
			}
			x[i].Value = changeItemAttributes(x[i].Value, set, out)
		}
		return x
	case bson.A:
		for i := range x {
			x[i] = changeItemAttributes(x[i], set, out)
		}
		return x
	}
	return v
}

func storedChangeAttributes(t *testing.T, exec *Executor) []string {
	t.Helper()
	_, raw := mfUnitByName(t, exec, "MyFirstModule", "MF_L1422")
	if raw == nil {
		return nil
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	var got []string
	changeItemAttributes(d, nil, &got)
	return got
}

// The builder refuses a change over a list instead of writing the corrupt
// member; the object range (`first`) is the control.
func TestChangeOverListRetrieveRefused1422(t *testing.T) {
	exec, _ := openPedAppFixture(t)
	err := agreeExec(t, exec, "mdl 1;\n"+lao1422Entity+lao1422Flow("limit 1"))
	if err == nil || !strings.Contains(err.Error(), "CE1613") {
		t.Fatalf("exec err = %v, want a refusal naming CE1613; stored change attributes: %q", err, storedChangeAttributes(t, exec))
	}
	if id, _ := mfUnitByName(t, exec, "MyFirstModule", "MF_L1422"); id != "" {
		t.Fatalf("the refused microflow was written anyway (%s)", id)
	}

	if err := agreeExec(t, exec, "mdl 1;\n"+lao1422Flow("first")); err != nil {
		t.Fatalf("control: `first` failed: %v", err)
	}
	if got := storedChangeAttributes(t, exec); len(got) != 1 || got[0] != "MyFirstModule.EntL1422.Name" {
		t.Fatalf("control: stored change attributes = %q, want [MyFirstModule.EntL1422.Name]", got)
	}
}

// A project already written by the old bug: changing `limit 1` to `first`
// spliced the retrieve and kept the corrupt change activity, because describe
// prints it as the clean `change $E (Name = 'x')`. The stored shape now counts
// as a change the splice cannot make: mdl 0 rebuilds (healing it), mdl 1
// refuses with the reason instead of keeping it.
func TestChangeOverListStoredMemberIsNotKept1422(t *testing.T) {
	exec, out := openPedAppFixture(t)
	if err := agreeExec(t, exec, "mdl 1;\n"+lao1422Entity+lao1422Flow("first")); err != nil {
		t.Fatal(err)
	}
	id, raw := mfUnitByName(t, exec, "MyFirstModule", "MF_L1422")
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	corrupt := "List of MyFirstModule.EntL1422.Name"
	var got []string
	changeItemAttributes(d, &corrupt, &got)
	if len(got) != 1 {
		t.Fatalf("expected one change member in MF_L1422, found %q", got)
	}
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	err = agreeExec(t, exec, "mdl 1;\n"+lao1422Flow("first"))
	if err == nil || !strings.Contains(err.Error(), "List of") {
		t.Fatalf("mdl 1 re-run over a stored list-qualified member: err = %v, output %q; want a refusal naming it", err, out.String())
	}

	out.Reset()
	if err := agreeExec(t, exec, lao1422Flow("first")); err != nil {
		t.Fatalf("mdl 0 re-run: %v", err)
	}
	if got := storedChangeAttributes(t, exec); len(got) != 1 || got[0] != "MyFirstModule.EntL1422.Name" {
		t.Fatalf("after the mdl 0 re-run the stored change attributes are %q, want [MyFirstModule.EntL1422.Name] (output %q)", got, out.String())
	}
}

// check: MDL-RETRIEVE03 is the reverse of MDL-RETRIEVE01 — a list retrieve
// used as an object by `change`.
func TestRetrieveListUsedAsObject1422(t *testing.T) {
	change := &ast.ChangeObjectStmt{Variable: "E", Changes: []ast.ChangeItem{{Attribute: "Name", Value: &ast.LiteralExpr{Value: "x", Kind: ast.LiteralString}}}}
	rule := func(stmt *ast.CreateMicroflowStmt) []string {
		var out []string
		for _, v := range ValidateMicroflow(stmt) {
			if v.RuleID == retrieveListAsObjectRule {
				out = append(out, v.Message)
			}
		}
		return out
	}
	if got := rule(mfWith(retrieveLimit("E", "1", ""), change)); len(got) != 1 || !strings.Contains(got[0], "CE1613") {
		t.Fatalf("change over an mdl 1 `limit 1` retrieve: %s = %q, want one violation naming CE1613", retrieveListAsObjectRule, got)
	}
	if got := rule(mfWith(retrieveFirst("E"), change)); len(got) != 0 {
		t.Fatalf("control: change over `first` reported %q", got)
	}
	// A member path over the list is CE0117 (measured, mx 11.6.6).
	ret := &ast.ReturnStmt{Value: &ast.AttributePathExpr{Variable: "E", Path: []string{"Name"}}}
	if got := rule(mfWith(retrieveLimit("E", "1", ""), ret)); len(got) != 1 || !strings.Contains(got[0], "CE0117") {
		t.Fatalf("return $E/Name over a list retrieve: %s = %q, want one violation naming CE0117", retrieveListAsObjectRule, got)
	}
	if got := rule(mfWith(retrieveFirst("E"), ret)); len(got) != 0 {
		t.Fatalf("control: return $E/Name over `first` reported %q", got)
	}
	// `set $E = empty` over the list is a Change list Replace, which builds.
	set := &ast.MfSetStmt{Target: "E", Value: &ast.LiteralExpr{Kind: ast.LiteralEmpty}}
	if got := rule(mfWith(retrieveLimit("E", "1", ""), set)); len(got) != 0 {
		t.Fatalf("set $E = empty over a list retrieve reported %q", got)
	}
	// An association retrieve may bind an object; the rule leaves it alone.
	assoc := retrieveLimit("E", "", "")
	assoc.StartVariable = "Owner"
	if got := rule(mfWith(assoc, change)); len(got) != 0 {
		t.Fatalf("change over an association retrieve reported %q", got)
	}
}
