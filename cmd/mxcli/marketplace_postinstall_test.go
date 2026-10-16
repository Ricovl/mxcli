// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	mp "github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
)

// A headless install leaves CE0463 (and, for modules, CE6087) until Mendix's own
// repair tools run. install used to only print "Next: mxcli fix widgets"; it
// now runs them itself (mendixlabs/mxcli#1527). These tests stub the mx seam:
// no mx binary, no marketplace network.

// stubPostInstall replaces the mx resolution and tool runner, recording every
// tool invocation.
func stubPostInstall(t *testing.T, resolveErr error, toolErr map[string]error) *[]string {
	t.Helper()
	var calls []string
	origResolve, origRun := postInstallResolveMx, postInstallRunTool
	postInstallResolveMx = func(mendixVer string) (string, error) {
		if resolveErr != nil {
			return "", resolveErr
		}
		return "/fake/mx", nil
	}
	postInstallRunTool = func(mxPath, mprPath, subcommand string, w, stderr io.Writer) (*docker.HarvestResult, error) {
		calls = append(calls, subcommand)
		if err := toolErr[subcommand]; err != nil {
			return nil, err
		}
		return &docker.HarvestResult{Harvested: true, UnitsWritten: 3, StillV2: true, StorageFiles: 10, StorageFilesBefore: 10}, nil
	}
	t.Cleanup(func() { postInstallResolveMx, postInstallRunTool = origResolve, origRun })
	return &calls
}

func TestPostInstallRepairs_ModuleRunsBothTools(t *testing.T) {
	calls := stubPostInstall(t, nil, nil)
	var out bytes.Buffer
	runPostInstallRepairs(&out, &out, "/p/app.mpr", "11.15.0", moduleRepairs)

	if got := strings.Join(*calls, ","); got != "update-widgets,rename-design-properties" {
		t.Fatalf("tools run = %q, want update-widgets then rename-design-properties\noutput:\n%s", got, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "widget definitions") || !strings.Contains(s, "3 unit(s) changed") {
		t.Errorf("expected a per-step result line, got:\n%s", s)
	}
	if strings.Contains(s, "mxcli fix widgets -p") {
		t.Errorf("the manual hint must not be printed when the repair ran:\n%s", s)
	}
}

func TestPostInstallRepairs_WidgetRunsOnlyUpdateWidgets(t *testing.T) {
	calls := stubPostInstall(t, nil, nil)
	var out bytes.Buffer
	runPostInstallRepairs(&out, &out, "/p/app.mpr", "11.15.0", widgetRepairs)
	if got := strings.Join(*calls, ","); got != "update-widgets" {
		t.Fatalf("tools run = %q, want only update-widgets", got)
	}
}

// No mx for the project's version: a warning plus the manual commands, never a
// failure — the install itself succeeded.
func TestPostInstallRepairs_NoMxKeepsTheHint(t *testing.T) {
	calls := stubPostInstall(t, errors.New("not cached"), nil)
	var out bytes.Buffer
	runPostInstallRepairs(&out, &out, "/p/app.mpr", "11.15.0", moduleRepairs)
	if len(*calls) != 0 {
		t.Fatalf("no tool may run without mx, ran %v", *calls)
	}
	s := out.String()
	for _, want := range []string{"Warning", "mxcli setup mxbuild", "mxcli fix widgets -p", "mxcli fix design-properties -p"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}

// Unknown Mendix version (the project could not be read): same fallback.
func TestPostInstallRepairs_UnknownVersionKeepsTheHint(t *testing.T) {
	calls := stubPostInstall(t, nil, nil)
	var out bytes.Buffer
	runPostInstallRepairs(&out, &out, "/p/app.mpr", "", widgetRepairs)
	if len(*calls) != 0 {
		t.Fatalf("no tool may run for an unknown version, ran %v", *calls)
	}
	if !strings.Contains(out.String(), "mxcli fix widgets -p") {
		t.Errorf("expected the manual hint, got:\n%s", out.String())
	}
}

// One failing step warns, names its manual command, and the next still runs.
func TestPostInstallRepairs_StepFailureWarnsAndContinues(t *testing.T) {
	calls := stubPostInstall(t, nil, map[string]error{"update-widgets": errors.New("boom")})
	var out bytes.Buffer
	runPostInstallRepairs(&out, &out, "/p/app.mpr", "11.15.0", moduleRepairs)
	if got := strings.Join(*calls, ","); got != "update-widgets,rename-design-properties" {
		t.Fatalf("tools run = %q, want both despite the first failing", got)
	}
	s := out.String()
	if !strings.Contains(s, "boom") || !strings.Contains(s, "mxcli fix widgets -p") {
		t.Errorf("expected the failure and its manual command, got:\n%s", s)
	}
	if strings.Contains(s, "mxcli fix design-properties -p") {
		t.Errorf("the step that succeeded must not print its manual hint:\n%s", s)
	}
}

// A module installed by the transplant writer runs both repairs instead of
// printing them as a "Next" step.
func TestReportTransplantInstall_RunsRepairs(t *testing.T) {
	calls := stubPostInstall(t, nil, nil)
	var out bytes.Buffer
	reportTransplantInstall(&out, "/p/app.mpr", "11.15.0", "Connector", "version 1.0.0", &mp.UpdateResult{UnitsCopied: 7})
	if got := strings.Join(*calls, ","); got != "update-widgets,rename-design-properties" {
		t.Fatalf("tools run = %q, want both repairs\noutput:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), `Installed module "Connector"`) {
		t.Errorf("install report missing:\n%s", out.String())
	}
}

// The --file widget path runs the resync after placing the file.
func TestInstallFile_WidgetRunsUpdateWidgets(t *testing.T) {
	calls := stubPostInstall(t, nil, nil)
	origVer := postInstallMendixVersion
	postInstallMendixVersion = func(string) string { return "11.15.0" }
	t.Cleanup(func() { postInstallMendixVersion = origVer })

	mpr := placeholderProject(t)
	src := buildLocalMPK(t, "Badge.mpk", map[string]string{"package.xml": widgetPackageXML})
	out, err := runInstallFile(t, "--file", src, "-p", mpr)
	if err != nil {
		t.Fatalf("run: %v\noutput: %s", err, out)
	}
	if got := strings.Join(*calls, ","); got != "update-widgets" {
		t.Errorf("tools run = %q, want update-widgets\noutput:\n%s", got, out)
	}
}
