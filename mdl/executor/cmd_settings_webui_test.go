// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1549 (ako/mxcli#1447): the Web UI setting UseOptimizedClient
// (Yes / No / MigrationMode — which client the app runs on) was visible in
// `show settings` but missing from `describe settings`, and `alter settings
// webui (OptimizedClient: 'Yes')` answered "unknown settings section: webui".

// webUICtx returns an executor over a project whose Web UI part stores
// UseOptimizedClient = current, and a pointer to the settings the last write
// handed the backend. stored=false models a project whose Mendix version does not
// store the property (it was introduced in 9.10).
func webUICtx(t *testing.T, current string, stored bool) (*ExecContext, *bytes.Buffer, **model.ProjectSettings) {
	t.Helper()
	ps := &model.ProjectSettings{WebUI: &model.WebUISettings{UseOptimizedClient: current}}
	raw := map[string]any{"$Type": "Forms$WebUIProjectSettingsPart", "UrlPrefix": "p"}
	if stored {
		raw["UseOptimizedClient"] = current
	}
	ps.RawParts = []map[string]any{raw}
	var written *model.ProjectSettings
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		GetProjectSettingsFunc:    func() (*model.ProjectSettings, error) { return ps, nil },
		UpdateProjectSettingsFunc: func(p *model.ProjectSettings) error { written = p; return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	return ctx, buf, &written
}

func parseAlterSettings(t *testing.T, src string) *ast.AlterSettingsStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	return findStmt[*ast.AlterSettingsStmt](t, prog, src)
}

// The statement as reported, through the real parser.
func TestAlterSettingsWebUI_AsReported(t *testing.T) {
	ctx, _, written := webUICtx(t, "Yes", true)
	stmt := parseAlterSettings(t, "mdl 1;\nalter settings webui (OptimizedClient: 'No');")
	if err := alterSettings(ctx, stmt); err != nil {
		t.Fatalf("alter settings webui refused: %v", err)
	}
	if *written == nil {
		t.Fatal("nothing was written")
	}
	if got := (*written).WebUI.UseOptimizedClient; got != "No" {
		t.Errorf("UseOptimizedClient = %q, want No", got)
	}
}

// Matched case-insensitively and stored in Mendix's spelling, like every other
// enumeration-typed setting.
func TestAlterSettingsWebUI_CanonicalisesValue(t *testing.T) {
	ctx, _, written := webUICtx(t, "Yes", true)
	if err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section: "webui", Properties: map[string]any{"OptimizedClient": "migrationmode"},
	}); err != nil {
		t.Fatalf("alter settings webui refused: %v", err)
	}
	if got := (*written).WebUI.UseOptimizedClient; got != "MigrationMode" {
		t.Errorf("UseOptimizedClient = %q, want MigrationMode", got)
	}
}

// A value outside the enumeration is the mendixlabs/mxcli#759 shape — Studio Pro
// cannot resolve it — so it is refused, and the refusal names what is accepted.
func TestAlterSettingsWebUI_RejectsNonMember(t *testing.T) {
	ctx, _, written := webUICtx(t, "Yes", true)
	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section: "webui", Properties: map[string]any{"OptimizedClient": "React"},
	})
	if err == nil {
		t.Fatal("alter settings webui accepted OptimizedClient 'React'")
	}
	for _, want := range []string{"No", "Yes", "MigrationMode"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
	if *written != nil {
		t.Error("a refused statement still wrote the settings")
	}
}

// The overlay will not introduce a property the stored part does not carry, so a
// project that does not store it is refused rather than told "Updated".
func TestAlterSettingsWebUI_RefusesWhenNotStored(t *testing.T) {
	ctx, _, written := webUICtx(t, "Yes", false)
	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section: "webui", Properties: map[string]any{"OptimizedClient": "No"},
	})
	if err == nil || !strings.Contains(err.Error(), "does not store") {
		t.Fatalf("expected a refusal naming the missing property, got %v", err)
	}
	if *written != nil {
		t.Error("a refused statement still wrote the settings")
	}
}

func TestAlterSettingsWebUI_UnknownKeyListsValidKeys(t *testing.T) {
	ctx, _, written := webUICtx(t, "Yes", true)
	err := alterSettings(ctx, &ast.AlterSettingsStmt{
		Section: "webui", Properties: map[string]any{"UseReactClient": "Yes"},
	})
	if err == nil || !strings.Contains(err.Error(), "OptimizedClient") {
		t.Fatalf("expected the unknown key refused with the valid one named, got %v", err)
	}
	if *written != nil {
		t.Error("a refused statement still wrote the settings")
	}
}

// DESCRIBE prints the setting as a statement that re-executes: replayed onto a
// project on the other client, it moves that project to the described one.
func TestDescribeSettings_WebUIRoundTrips(t *testing.T) {
	ctx, buf, _ := webUICtx(t, "No", true)
	assertNoError(t, describeSettings(ctx, ""))
	out := buf.String()
	if !strings.Contains(out, "alter settings webui (") {
		t.Fatalf("describe settings has no Web UI statement:\n%s", out)
	}

	stmt := findStmt[*ast.AlterSettingsStmt](t, reparse(t, out), out)
	target, _, written := webUICtx(t, "Yes", true)
	assertNoError(t, alterSettings(target, stmt))
	if *written == nil || (*written).WebUI.UseOptimizedClient != "No" {
		t.Errorf("replaying describe output did not set OptimizedClient to No:\n%s", out)
	}
}

// A project whose version does not store the property describes no statement for
// it: one that names it would be refused on replay.
func TestDescribeSettings_WebUINotStoredNotPrinted(t *testing.T) {
	ctx, buf, _ := webUICtx(t, "Yes", false)
	assertNoError(t, describeSettings(ctx, ""))
	if out := buf.String(); strings.Contains(out, "webui") {
		t.Errorf("describe printed a Web UI statement the project cannot replay:\n%s", out)
	}
}
