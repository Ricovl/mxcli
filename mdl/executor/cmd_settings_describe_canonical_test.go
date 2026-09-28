// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// R3 (ako/mxcli#751): describe settings writes every section as the
// ( Key: value, … ) list `alter` takes, so its output replays with no
// deprecation warning, under no header and under `mdl 1;` alike. The control
// is the property count: output that dropped the properties would parse
// cleanly too.
func TestDescribeSettings_EmitsCanonicalPropertyLists(t *testing.T) {
	ps := &model.ProjectSettings{
		Model: &model.ModelSettings{
			AfterStartupMicroflow: "M.ASU", HashAlgorithm: "BCrypt", BcryptCost: 11,
			JavaVersion: "Java21", RoundingMode: "HalfEven",
		},
		Configuration: &model.ConfigurationSettings{
			Configurations: []*model.ServerConfiguration{
				{Name: "Default", DatabaseType: "Hsqldb", DatabaseName: "default", HttpPortNumber: 8080,
					ApplicationRootUrl: "http://localhost:8080/"},
			},
		},
		Language: &model.LanguageSettings{DefaultLanguageCode: "en_US", Languages: []model.Language{{Code: "en_US"}}},
		Workflows: &model.WorkflowsSettings{UserEntity: "System.User", DefaultTaskParallelism: 3,
			Groups: []model.WorkflowGroup{{Name: "Approvers", Description: "First line"}}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return ps, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	if err := describeSettings(ctx, ""); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, header := range []string{"", "mdl 1;\n"} {
		prog, errs := visitor.Build(header + out)
		if len(errs) > 0 {
			t.Fatalf("describe output does not parse (header %q): %v\n%s", header, errs, out)
		}
		if len(prog.Deprecations) > 0 {
			t.Errorf("describe output uses a deprecated spelling (header %q): %+v\n%s", header, prog.Deprecations, out)
		}
	}
	for _, want := range []string{
		"alter settings runtime (\n  AfterStartupMicroflow: 'M.ASU',",
		"create or modify configuration 'Default' (\n  DatabaseType: 'Hsqldb',",
		"alter settings LANGUAGE (\n  DefaultLanguageCode: 'en_US'\n);",
		"alter settings workflows (\n  UserEntity: 'System.User',\n  DefaultTaskParallelism: 3\n);",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, " = ") {
		t.Errorf("describe output still assigns with `=`:\n%s", out)
	}
}
