// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1602 (ako/mxcli#1512): after
//
//	ALTER SETTINGS CONSTANT @MyFirstModule.ApiKey VALUE 'secret123' IN CONFIGURATION 'Default';
//
// or `ALTER SETTINGS RUNTIME (BcryptCost: 11);`, `diff-local` printed
//
//	Summary: 0 new, 1 modified, 0 deleted
//
// and with -f struct `ProjectSettings: <unit>` / `(no changes)`. bsonToMDL had
// no case for the settings unit, so both sides fell into the generic
// "-- <Type>: <name>" header, which is the same text before and after.
package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

const set1602Unit = "4e70063d-53e1-477a-ab92-6b763330c293"

// set1602Settings is a project-settings unit in its stored shape: a versioned
// "Settings" array of parts, the constant's value under
// SharedOrPrivateValue.Value.
func set1602Settings(bcrypt int64, apiKey string) map[string]any {
	return map[string]any{
		"$Type": "Settings$ProjectSettings",
		"Settings": []any{int32(3),
			map[string]any{"$Type": "Settings$ModelSettings", "BcryptCost": bcrypt, "HashAlgorithm": "BCrypt"},
			map[string]any{"$Type": "Settings$ConfigurationSettings", "Configurations": []any{int32(3), map[string]any{
				"$Type":        "Settings$ServerConfiguration",
				"Name":         "Default",
				"DatabaseType": "HSQLDB",
				"ConstantValues": []any{int32(3), map[string]any{
					"$Type":                "Settings$ConstantValue",
					"ConstantId":           "MyFirstModule.ApiKey",
					"SharedOrPrivateValue": map[string]any{"$Type": "Settings$SharedValue", "Value": apiKey},
				}},
			}}},
		},
	}
}

// set1602ParseSettings stands in for the backend's decode of that shape, and
// like it refuses any other document.
func set1602ParseSettings(raw map[string]any) *model.ProjectSettings {
	if raw["$Type"] != "Settings$ProjectSettings" {
		return nil
	}
	ps := &model.ProjectSettings{}
	for _, p := range extractBsonArray(raw["Settings"]) {
		part := p.(map[string]any)
		ps.RawParts = append(ps.RawParts, part)
		switch part["$Type"] {
		case "Settings$ModelSettings":
			ps.Model = &model.ModelSettings{BcryptCost: extractInt(part["BcryptCost"]), HashAlgorithm: extractString(part["HashAlgorithm"])}
		case "Settings$ConfigurationSettings":
			ps.Configuration = &model.ConfigurationSettings{}
			for _, c := range extractBsonArray(part["Configurations"]) {
				cm := c.(map[string]any)
				cfg := &model.ServerConfiguration{Name: extractString(cm["Name"]), DatabaseType: extractString(cm["DatabaseType"])}
				for _, v := range extractBsonArray(cm["ConstantValues"]) {
					vm := v.(map[string]any)
					val := vm["SharedOrPrivateValue"].(map[string]any)
					cfg.ConstantValues = append(cfg.ConstantValues, &model.ConstantValue{
						ConstantId: extractString(vm["ConstantId"]),
						Value:      extractString(val["Value"]),
					})
				}
				ps.Configuration.Configurations = append(ps.Configuration.Configurations, cfg)
			}
		}
	}
	return ps
}

func set1602Context() (*ExecContext, *bytes.Buffer) {
	mb := &mock.MockBackend{
		IsConnectedFunc:                 func() bool { return true },
		VersionFunc:                     func() types.MPRVersion { return 2 },
		ContentsDirFunc:                 func() string { return "mprcontents" },
		ParseProjectSettingsFromRawFunc: set1602ParseSettings,
	}
	out := &bytes.Buffer{}
	return &ExecContext{Context: context.Background(), Backend: mb, Output: out, Cache: &executorCache{}}, out
}

func TestDiffLocal_SettingsConstantValueChangeShowsMaskedHunk(t *testing.T) {
	unitPath := "mprcontents/4e/70/" + set1602Unit + ".mxunit"
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": set1602Settings(10, "x"),
		"HEAD":   set1602Settings(10, "secret123"),
	})
	ctx, out := set1602Context()

	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "-alter settings constant 'MyFirstModule.ApiKey' value <masked") ||
		!strings.Contains(got, "+alter settings constant 'MyFirstModule.ApiKey' value <masked") {
		t.Errorf("diff-local shows no hunk for a configuration constant's value (#1602):\n%s", got)
	}
	for _, secret := range []string{"secret123", "'x'"} {
		if strings.Contains(got, secret) {
			t.Errorf("diff-local printed the constant's value %s; configuration constants often hold secrets:\n%s", secret, got)
		}
	}
}

func TestDiffLocal_SettingsRuntimeChangeShowsHunk(t *testing.T) {
	unitPath := "mprcontents/4e/70/" + set1602Unit + ".mxunit"
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": set1602Settings(10, "x"),
		"HEAD":   set1602Settings(11, "x"),
	})
	ctx, out := set1602Context()

	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{Format: DiffFormatStructural}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "(no changes)") {
		t.Errorf("a BcryptCost change reported (no changes) (#1602):\n%s", got)
	}

	ctx, out = set1602Context()
	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "-  BcryptCost: 10") || !strings.Contains(got, "+  BcryptCost: 11") {
		t.Errorf("diff-local shows no hunk for ALTER SETTINGS RUNTIME (BcryptCost: 11) (#1602):\n%s", got)
	}
}

// An unchanged value must mask to the same text on both sides, or every
// settings diff would show a hunk for every constant.
func TestDiffLocal_SettingsMaskIsStableForAnUnchangedValue(t *testing.T) {
	if a, b := maskedSettingValue("secret123"), maskedSettingValue("secret123"); a != b {
		t.Errorf("the same value masked twice differs: %s vs %s", a, b)
	}
	if a, b := maskedSettingValue("x"), maskedSettingValue("y"); a == b {
		t.Errorf("two different values masked the same (%s), so a changed value diffs to nothing", a)
	}
}
