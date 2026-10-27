// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Security$ModuleSecurity and Projects$ModuleSettings contain "Module", so the
// containment dispatch sent them to the module renderer: a new module's
// security unit printed as `+create module ;`, naming the wrong thing and no
// module at all.
func TestDiffLocal_ModuleSecurityIsNotRenderedAsAModule(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	proj := t.TempDir()
	writeEnumUnit(t, proj, "Base")
	gitIn(t, proj, "init", "-q")
	gitIn(t, proj, "add", "-A")
	gitIn(t, proj, "commit", "-q", "-m", "base")
	unit := filepath.Join(proj, "mprcontents", "7c", "7c", "7c7c7c7c-1111-2222-3333-444444444444.mxunit")
	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := bson.Marshal(map[string]any{"$Type": "Security$ModuleSecurity",
		"ModuleRoles": []any{int32(3), map[string]any{"$Type": "Security$ModuleRole", "Name": "User"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, data, 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, proj, "add", "-A")
	gitIn(t, proj, "commit", "-q", "-m", "module security")

	ctx, out := gitDiffContext(filepath.Join(proj, "mprcontents"))
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "create module") || !strings.Contains(got, "ModuleSecurity") {
		t.Errorf("a new module security unit is rendered as a module:\n%s", got)
	}
}
