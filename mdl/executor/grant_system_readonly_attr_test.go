// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// storedMemberRights reads the rights each attribute entry of an entity's
// access rules carries, keyed by the stored attribute reference.
func storedMemberRights(t *testing.T, exec *Executor, module, entity string) map[string]string {
	t.Helper()
	ctx := exec.newExecContext(t.Context())
	mod, err := ctx.Backend.GetModuleByName(module)
	if err != nil || mod == nil {
		t.Fatalf("module %s: %v", module, err)
	}
	dm, err := ctx.Backend.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("domain model: %v", err)
	}
	ent := dm.FindEntityByName(entity)
	if ent == nil {
		t.Fatalf("entity %s.%s not found", module, entity)
	}
	out := map[string]string{}
	for _, r := range ent.AccessRules {
		for _, ma := range r.MemberAccesses {
			if ma.AttributeName != "" {
				out[ma.AttributeName] = string(ma.AccessRights)
			}
		}
	}
	return out
}

// `grant read *, write *` on a System.FileDocument / System.Image
// specialization wrote HasContents (and PublicThumbnailPath) ReadWrite, and
// mxbuild 11.14.0 (security level Production) then reported CE6592 "Attribute
// 'HasContents' cannot have write rights, because it is a system attribute".
// The other inherited members — Name, DeleteAfterDownload, Contents, Size,
// EnableCaching — are the control: measured clean with write, they must keep
// ReadWrite, so the fix cannot have been a blanket downgrade of System members.
func TestGrantWriteAll_SystemReadOnlyAttributesDowngraded(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const src = `mdl 1;
create module ModG;
create module role ModG.User;
create persistent entity ModG.Doc extends System.FileDocument (Title: String(100));
create persistent entity ModG.Pic extends System.Image (Caption: String(100));`
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := agreeExec(t, exec, "mdl 1;\ngrant read *, write * on entity ModG.Doc to ModG.User;\ngrant read *, write * on entity ModG.Pic to ModG.User;"); err != nil {
		t.Fatalf("grant: %v\n%s", err, out.String())
	}
	// The grant writes the rule itself, and the reconcile that follows also
	// downgrades (so `update security` repairs a rule written before this was
	// known). The grant's own report proves the first half: it is printed
	// from the rule the grant wrote.
	for _, line := range strings.Split(out.String(), "\n") {
		if _, writes, ok := strings.Cut(line, "write ("); ok && (strings.Contains(writes, "HasContents") || strings.Contains(writes, "PublicThumbnailPath")) {
			t.Errorf("grant reports write on a system read-only attribute: %s", line)
		}
	}

	doc := storedMemberRights(t, exec, "ModG", "Doc")
	pic := storedMemberRights(t, exec, "ModG", "Pic")
	for _, c := range []struct {
		rights map[string]string
		ref    string
		want   string
	}{
		{doc, "System.FileDocument.HasContents", "ReadOnly"},
		{pic, "System.FileDocument.HasContents", "ReadOnly"},
		{pic, "System.Image.PublicThumbnailPath", "ReadOnly"},
		// Controls.
		{doc, "System.FileDocument.Name", "ReadWrite"},
		{doc, "System.FileDocument.Contents", "ReadWrite"},
		{doc, "System.FileDocument.Size", "ReadWrite"},
		{doc, "System.FileDocument.DeleteAfterDownload", "ReadWrite"},
		{pic, "System.Image.EnableCaching", "ReadWrite"},
		{doc, "ModG.Doc.Title", "ReadWrite"},
	} {
		if got := c.rights[c.ref]; got != c.want {
			t.Errorf("%s: stored %q, want %q (all: %v)", c.ref, got, c.want, c.rights)
		}
	}
}

func TestSystemAttributeWriteForbidden(t *testing.T) {
	for ref, want := range map[string]bool{
		"System.FileDocument.HasContents":  true,
		"System.Image.PublicThumbnailPath": true,
		"System.FileDocument.Name":         false,
		"System.FileDocument.Contents":     false,
		"System.Image.EnableCaching":       false,
		"ModG.Doc.HasContents":             false,
		"System.NoSuch.HasContents":        false,
	} {
		if got := meta.SystemAttributeWriteForbidden(ref); got != want {
			t.Errorf("SystemAttributeWriteForbidden(%s) = %v, want %v", ref, got, want)
		}
	}
	if !types.WriteRightsForbidden(false, false, true) || types.WriteRightsForbidden(false, false, false) {
		t.Error("WriteRightsForbidden must honour the system read-only cause, and only it")
	}
}

// `revoke write (Name)` on a FileDocument specialization — an INHERITED member
// — answered "No access rules found matching …" and changed nothing while the
// rule held ReadWrite: the member was qualified with the specialization
// (ModG.Doc.Name) and the stored entry is qualified with the declaring entity
// (System.FileDocument.Name). The entity's own Title is the control; it was
// always found.
func TestRevokeWriteMember_InheritedMemberIsFound(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const src = `mdl 1;
create module ModG;
create module role ModG.User;
create persistent entity ModG.Doc extends System.FileDocument (Title: String(100));
grant read *, write * on entity ModG.Doc to ModG.User;`
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if got := storedMemberRights(t, exec, "ModG", "Doc")["System.FileDocument.Name"]; got != "ReadWrite" {
		t.Fatalf("setup: inherited Name = %q, want ReadWrite", got)
	}

	out.Reset()
	if err := agreeExec(t, exec, "mdl 1;\nrevoke write (\"Name\", \"Title\") on entity ModG.Doc from ModG.User;"); err != nil {
		t.Fatalf("revoke: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "No access rules found") {
		t.Errorf("revoke reported nothing to revoke while the rule held ReadWrite:\n%s", out.String())
	}
	rights := storedMemberRights(t, exec, "ModG", "Doc")
	if got := rights["System.FileDocument.Name"]; got != "ReadOnly" {
		t.Errorf("inherited Name after revoke write = %q, want ReadOnly", got)
	}
	if got := rights["ModG.Doc.Title"]; got != "ReadOnly" {
		t.Errorf("control: own Title after revoke write = %q, want ReadOnly", got)
	}
}
