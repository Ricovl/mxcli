// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#131 (ako/mxcli#1225): Studio Pro's Configuration ▸ Server tab
// has an "Extra JVM parameters" field — the way to give the runtime JVM a proxy —
// and the settings reader already read it, but ALTER SETTINGS CONFIGURATION
// answered "unknown configuration setting: ExtraJvmParameters".

const proxyJvmParameters = "-Dhttps.proxyHost=127.0.0.1 -Dhttps.proxyPort=29758"

// jvmParamsCtx returns an executor over one Default configuration, and a pointer
// to the settings the last write handed the backend.
func jvmParamsCtx(t *testing.T, cfg *model.ServerConfiguration) (*ExecContext, **model.ProjectSettings) {
	t.Helper()
	ps := &model.ProjectSettings{Configuration: &model.ConfigurationSettings{
		Configurations: []*model.ServerConfiguration{cfg},
	}}
	var written *model.ProjectSettings
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		GetProjectSettingsFunc:    func() (*model.ProjectSettings, error) { return ps, nil },
		UpdateProjectSettingsFunc: func(p *model.ProjectSettings) error { written = p; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx, &written
}

// The statement exactly as reported, through the real parser: the old
// `Key = value` spelling, which still runs.
func TestAlterSettingsConfiguration_ExtraJvmParameters_AsReported(t *testing.T) {
	cfg := &model.ServerConfiguration{Name: "Default", DatabaseType: "Hsqldb", HttpPortNumber: 8080}
	ctx, written := jvmParamsCtx(t, cfg)

	prog, errs := visitor.Build("ALTER SETTINGS CONFIGURATION 'Default' ExtraJvmParameters = '" + proxyJvmParameters + "'")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	stmt := findStmt[*ast.AlterSettingsStmt](t, prog, "")
	if err := alterSettings(ctx, stmt); err != nil {
		t.Fatalf("alter settings configuration refused ExtraJvmParameters: %v", err)
	}
	if *written == nil {
		t.Fatal("nothing was written")
	}
	if got := (*written).Configuration.Configurations[0].ExtraJvmParameters; got != proxyJvmParameters {
		t.Errorf("ExtraJvmParameters = %q, want %q", got, proxyJvmParameters)
	}
}

func TestCreateConfiguration_ExtraJvmParameters(t *testing.T) {
	ctx, written := jvmParamsCtx(t, &model.ServerConfiguration{Name: "Default"})
	err := createConfiguration(ctx, &ast.CreateConfigurationStmt{
		Name:       "Proxied",
		Properties: map[string]any{"ExtraJvmParameters": proxyJvmParameters},
	})
	if err != nil {
		t.Fatalf("create configuration refused ExtraJvmParameters: %v", err)
	}
	cfgs := (*written).Configuration.Configurations
	if len(cfgs) != 2 || cfgs[1].Name != "Proxied" || cfgs[1].ExtraJvmParameters != proxyJvmParameters {
		t.Fatalf("configurations = %+v, want a Proxied configuration carrying the JVM parameters", cfgs)
	}
}

// DESCRIBE has to carry the parameters, or a described project replays without
// them. The value holds a quote, so the round trip also proves the escape.
func TestDescribeSettings_ExtraJvmParametersRoundTrips(t *testing.T) {
	const params = "-Dhttps.proxyHost=127.0.0.1 -Dapp.greeting='hi'"
	cfg := &model.ServerConfiguration{
		Name: "Default", DatabaseType: "Hsqldb", DatabaseName: "default",
		HttpPortNumber: 8080, ServerPortNumber: 8090, ExtraJvmParameters: params,
	}
	ctx, _ := jvmParamsCtx(t, cfg)
	buf := ctx.Output.(*bytes.Buffer)
	assertNoError(t, describeSettings(ctx, "Default"))
	out := buf.String()
	if !strings.Contains(out, "ExtraJvmParameters:") {
		t.Fatalf("describe does not carry ExtraJvmParameters:\n%s", out)
	}

	// Replay onto a project whose Default has no parameters: they come back.
	stmt := findStmt[*ast.CreateConfigurationStmt](t, reparse(t, out), out)
	target := &model.ServerConfiguration{Name: "Default", DatabaseType: "Hsqldb", DatabaseName: "default",
		HttpPortNumber: 8080, ServerPortNumber: 8090}
	tctx, _ := jvmParamsCtx(t, target)
	assertNoError(t, createConfiguration(tctx, stmt))
	if !reflect.DeepEqual(*target, *cfg) {
		t.Errorf("replaying describe output did not reproduce the configuration:\nwant %+v\ngot  %+v\n%s", *cfg, *target, out)
	}
}

// Empty parameters are the default and are not printed, like an empty root URL —
// a blank project's describe output stays as it was.
func TestDescribeSettings_EmptyExtraJvmParametersNotPrinted(t *testing.T) {
	ctx, _ := jvmParamsCtx(t, &model.ServerConfiguration{Name: "Default", DatabaseType: "Hsqldb"})
	buf := ctx.Output.(*bytes.Buffer)
	assertNoError(t, describeSettings(ctx, "Default"))
	if out := buf.String(); strings.Contains(out, "ExtraJvmParameters") {
		t.Errorf("empty ExtraJvmParameters printed:\n%s", out)
	}
}
