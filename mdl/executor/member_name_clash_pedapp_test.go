// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#1645: a member that reuses a name already on the entity's
// generalization chain passed check and exec, and mxbuild rejected it with
// CE0069 "Duplicate member name". Every refusal below is a model measured as
// CE0069 on mx 11.13.0; every control is one measured clean. check -p and exec
// must agree on each (assertAgree), and exec's verdict is its own — agreeExec
// runs the statements as `exec --no-check` does.

const ce0069 = "CE0069"

// The reporter's two scripts, verbatim but for the module names.
func TestMemberClash_IssueRepro(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, `mdl 1;
create module Other;
create module HR;
create persistent entity Other.Person (Name: String(50));
create persistent entity Other.Dept (Name: String(50));
create association Other.Person_Dept from Other.Person to Other.Dept;
create persistent entity HR.Emp extends Other.Person (Code: String(10));
create association HR.Person_Dept from HR.Emp to Other.Dept;`,
		"HR.Emp inherits 'Person_Dept' from Other.Person (association Other.Person_Dept)")

	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, `mdl 1;
create module Other;
create module HR;
create persistent entity Other.Person (Name: String(50));
create persistent entity HR.Emp extends Other.Person (Name: String(50), Code: String(10));`,
		"HR.Emp inherits 'Name' from Other.Person (attribute Other.Person.Name)")
}

// The controls: a non-colliding name, the same name on an unrelated entity, on
// two siblings, and in another case — none of which mxbuild reports.
func TestMemberClash_Controls(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, `mdl 1;
create module CtlA;
create module CtlB;
create persistent entity CtlA.Person (Name: String(50));
create persistent entity CtlA.Dept (Name: String(50));
create association CtlA.Person_Dept from CtlA.Person to CtlA.Dept;
create persistent entity CtlB.Emp extends CtlA.Person (Code: String(10));
create persistent entity CtlB.Contractor extends CtlA.Person (Code: String(10));
create persistent entity CtlB.Unrelated (Name: String(50), Person_Dept: String(10));
create persistent entity CtlB.Cased extends CtlA.Person (name: String(50));
create association CtlB.Emp_Dept from CtlB.Emp to CtlA.Dept;
alter entity CtlA.Person add attribute Title: String(50);`, "")
	// An association is a member of its TO entity only under owner Both.
	assertAgree(t, exec, dir, `mdl 1;
create persistent entity CtlA.Target (Link: String(10));
create association CtlA.Link from CtlA.Dept to CtlA.Target;`, "")
	// A re-run of what is stored changes nothing on the chain.
	assertAgree(t, exec, dir, `mdl 1;
create or modify persistent entity CtlB.Emp extends CtlA.Person (Code: String(10));`, "")
}

// Both members kinds collide with both: an association named like an inherited
// attribute, an attribute named like an inherited association.
func TestMemberClash_AttributeAndAssociationShareOneNameSpace(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, `mdl 1;
create module G3;
create module S3;
create persistent entity G3.Base (Code: String(10));
create persistent entity S3.Spec extends G3.Base (X: String(10));
create persistent entity S3.T (Y: String(10));
create association S3.Code from S3.Spec to S3.T;`,
		"S3.Spec inherits 'Code' from G3.Base (attribute G3.Base.Code)")

	assertAgree(t, exec, dir, `mdl 1;
create module G4;
create module S4;
create persistent entity G4.Base (A: String(10));
create persistent entity G4.T (B: String(10));
create association G4.Link from G4.Base to G4.T;
create persistent entity S4.Spec extends G4.Base (Link: String(10));`,
		"S4.Spec inherits 'Link' from G4.Base (association G4.Link)")

	// The same entity's own attribute and association.
	assertAgree(t, exec, dir, `mdl 1;
create module M10;
create persistent entity M10.A (Link: String(10));
create persistent entity M10.B (Z: String(10));
create association M10.Link from M10.A to M10.B;`,
		"M10.A already has attribute M10.A.Link")

	// Under owner Both the association is a member of the TO entity too.
	assertAgree(t, exec, dir, `mdl 1;
create module M11;
create persistent entity M11.T (Link: String(10));
create persistent entity M11.F (Z: String(10));
create association M11.Link from M11.F to M11.T type reference owner both;`,
		"M11.T already has attribute M11.T.Link")
}

// The generalization chain is walked through System entities: their
// attributes and their associations both count.
func TestMemberClash_SystemAncestors(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, `mdl 1;
create module S7;
create persistent entity S7.Acct extends System.User (Name: String(50));`,
		"S7.Acct inherits 'Name' from System.User (attribute System.User.Name)")
	assertAgree(t, exec, dir, `mdl 1;
create module S7b;
create persistent entity S7b.Acct extends System.User (UserRoles: String(10));`,
		"S7b.Acct inherits 'UserRoles' from System.User (association System.UserRoles)")
	assertAgree(t, exec, dir, `mdl 1;
create module S8;
create persistent entity S8.Doc extends System.FileDocument (Size: Long);`,
		"S8.Doc inherits 'Size' from System.FileDocument")
}

// The other direction: a generalization gaining a member a specialization
// already has — in one script, and against the stored project. Two levels.
func TestMemberClash_GeneralizationGainsSpecializationsMember(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, `mdl 1;
create module G5;
create module S5;
create persistent entity G5.Base (Name: String(50));
create persistent entity G5.Mid extends G5.Base (M: String(10));
create persistent entity S5.Spec extends G5.Mid (Code: String(10));
alter entity G5.Base add attribute Code: String(10);`,
		"G5.Base's specialization S5.Spec already has 'Code' (attribute S5.Spec.Code)")

	exec2, out2, dir2 := openPedAppCopy(t)
	if err := agreeExec(t, exec2, `mdl 1;
create module G5;
create module S5;
create persistent entity G5.Base (Name: String(50));
create persistent entity S5.Spec extends G5.Base (Code: String(10));`); err != nil {
		t.Fatalf("setup: %v\n%s", err, out2.String())
	}
	assertAgree(t, exec2, dir2, "mdl 1;\nalter entity G5.Base add attribute Code: String(10);",
		"G5.Base's specialization S5.Spec already has 'Code'")
	assertAgree(t, exec2, dir2, "mdl 1;\ncreate or modify persistent entity G5.Base (Name: String(50), Code: String(10));",
		"G5.Base's specialization S5.Spec already has 'Code'")
	// Dropping the specialization's member first makes room: the script is
	// replayed in order, not judged against the stored model alone.
	assertAgree(t, exec2, dir2, `mdl 1;
alter entity S5.Spec drop attribute Code;
alter entity G5.Base add attribute Code: String(10);`, "")
}

// A specialization created before its generalization (the generalization is
// resolved lazily): the parent's member meets the child's when the parent is
// created.
func TestMemberClash_SpecializationCreatedFirst(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, `mdl 1;
create module G6;
create persistent entity G6.Spec extends G6.Base (Name: String(50));
create persistent entity G6.Base (Name: String(50));`,
		"G6.Base's specialization G6.Spec already has 'Name'")
}

// A rename into an inherited name — of an attribute and of an association.
func TestMemberClash_Renames(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, `mdl 1;
create module G6;
create module S6;
create persistent entity G6.Base (Name: String(50));
create persistent entity S6.Spec extends G6.Base (Other: String(50));
alter entity S6.Spec rename attribute Other to Name;`,
		"S6.Spec inherits 'Name' from G6.Base")

	assertAgree(t, exec, dir, `mdl 1;
create module G13;
create module S13;
create persistent entity G13.Base (A: String(10));
create persistent entity G13.T (B: String(10));
create association G13.Lnk from G13.Base to G13.T;
create persistent entity S13.Spec extends G13.Base (C: String(10));
create association S13.Foo from S13.Spec to G13.T;
rename association S13.Foo to Lnk;`,
		"S13.Spec inherits 'Lnk' from G13.Base (association G13.Lnk)")
}

// Owner Both makes the association a member of the TO entity: setting it on
// one whose TO entity already has the name, and a new generalization that
// brings a name the entity's specialization has.
func TestMemberClash_OwnerBothAndNewGeneralization(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, `mdl 1;
create module M12;
create persistent entity M12.T (Link: String(10));
create persistent entity M12.F (Z: String(10));
create association M12.Link from M12.F to M12.T;
create persistent entity M12.Base (Code: String(10));
create persistent entity M12.Mid (A: String(10));
create persistent entity M12.Leaf extends M12.Mid (Code: String(10));`); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, "mdl 1;\nalter association M12.Link set owner both;",
		"M12.T already has attribute M12.T.Link")
	assertAgree(t, exec, dir, "mdl 1;\ncreate or modify persistent entity M12.Mid extends M12.Base (A: String(10));",
		"through the new generalization of M12.Mid, M12.Leaf would inherit 'Code' from M12.Base")
}

// ALTER ENTITIES adds to every entity of a module: one that inherits the name
// from outside the target set is refused before anything is written.
func TestMemberClash_BulkAlterEntities(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, `mdl 1;
create module Other;
create module HR;
create persistent entity Other.Person (Name: String(50));
create persistent entity HR.Alpha (A: String(10));
create persistent entity HR.Emp extends Other.Person (Code: String(10));`); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, "mdl 1;\nalter entities in HR add attribute Name: String(50);",
		"HR.Emp inherits 'Name' from Other.Person")
	// Nothing was written: HR.Alpha, which sorts first, did not get it — adding
	// it now is not refused as already existing.
	if err := agreeExec(t, exec, "mdl 1;\nalter entity HR.Alpha add attribute Name: String(50);"); err != nil {
		t.Errorf("the refused bulk statement wrote HR.Alpha.Name: %v", err)
	}
	assertAgree(t, exec, dir, "mdl 1;\nalter entities in HR add attribute Note: String(50);", "")
}
