// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// MaxJavaHeapSize is Studio Pro's Configuration ▸ Server ▸ Java heap ▸ Maximum
// size (in MB), beside the Extra JVM parameters of mendixlabs/mxcli#131. The
// stored configuration has no separate Default/Custom flag: 0 is Default. It was
// read and written back, but no statement could set it and describe omitted it.

func TestAlterSettingsConfiguration_MaxJavaHeapSize(t *testing.T) {
	ctx, written := jvmParamsCtx(t, &model.ServerConfiguration{Name: "Default", DatabaseType: "Hsqldb"})
	stmt := parseAlterSettings(t, "mdl 1;\nalter settings configuration 'Default' (MaxJavaHeapSize: 2048);")
	if err := alterSettings(ctx, stmt); err != nil {
		t.Fatalf("alter settings configuration refused MaxJavaHeapSize: %v", err)
	}
	if got := (*written).Configuration.Configurations[0].MaxJavaHeapSize; got != 2048 {
		t.Errorf("MaxJavaHeapSize = %d, want 2048", got)
	}
}

func TestCreateConfiguration_MaxJavaHeapSize(t *testing.T) {
	ctx, written := jvmParamsCtx(t, &model.ServerConfiguration{Name: "Default"})
	if err := createConfiguration(ctx, &ast.CreateConfigurationStmt{
		Name: "Big", Properties: map[string]any{"MaxJavaHeapSize": "4096"},
	}); err != nil {
		t.Fatalf("create configuration refused MaxJavaHeapSize: %v", err)
	}
	if cfgs := (*written).Configuration.Configurations; len(cfgs) != 2 || cfgs[1].MaxJavaHeapSize != 4096 {
		t.Fatalf("configurations = %+v, want Big with a 4096 MB heap", cfgs)
	}
}

func TestDescribeSettings_MaxJavaHeapSizeRoundTrips(t *testing.T) {
	cfg := &model.ServerConfiguration{
		Name: "Default", DatabaseType: "Hsqldb", DatabaseName: "default",
		HttpPortNumber: 8080, ServerPortNumber: 8090, MaxJavaHeapSize: 1024,
	}
	ctx, _ := jvmParamsCtx(t, cfg)
	buf := ctx.Output.(*bytes.Buffer)
	assertNoError(t, describeSettings(ctx, "Default"))
	out := buf.String()
	if !strings.Contains(out, "MaxJavaHeapSize: 1024") {
		t.Fatalf("describe does not carry MaxJavaHeapSize:\n%s", out)
	}
	stmt := findStmt[*ast.CreateConfigurationStmt](t, reparse(t, out), out)
	target := &model.ServerConfiguration{Name: "Default", DatabaseType: "Hsqldb", DatabaseName: "default",
		HttpPortNumber: 8080, ServerPortNumber: 8090}
	tctx, _ := jvmParamsCtx(t, target)
	assertNoError(t, createConfiguration(tctx, stmt))
	if !reflect.DeepEqual(*target, *cfg) {
		t.Errorf("replaying describe output did not reproduce the configuration:\nwant %+v\ngot  %+v\n%s", *cfg, *target, out)
	}
}

// 0 is Studio Pro's "Default" heap and is not printed, so a blank project's
// describe output is unchanged.
func TestDescribeSettings_DefaultHeapNotPrinted(t *testing.T) {
	ctx, _ := jvmParamsCtx(t, &model.ServerConfiguration{Name: "Default", DatabaseType: "Hsqldb"})
	buf := ctx.Output.(*bytes.Buffer)
	assertNoError(t, describeSettings(ctx, "Default"))
	if strings.Contains(buf.String(), "MaxJavaHeapSize") {
		t.Errorf("default heap printed:\n%s", buf.String())
	}
}
