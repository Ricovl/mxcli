// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"path/filepath"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1509: a YAML list under a rule's options reached the rule as
// None, because goValueToStarlark knew only scalars. A rule reading a list
// option therefore fell back to its default with no sign that the configured
// value had been dropped.

const optionEchoRule = `
RULE_ID = "OPT001"
RULE_NAME = "Echo"
DESCRIPTION = "reports the options it was given"
CATEGORY = "Quality"
SEVERITY = "info"

def check():
    return [
        violation(message = "get_option=" + repr(get_option("xs", "unset"))),
        violation(message = "get_list_option=" + repr(get_list_option("xs", ["DEF_"]))),
    ]
`

func runEcho(t *testing.T, opts map[string]any) (getOpt, getList string, warnings []Violation) {
	t.Helper()
	r := loadRuleSource(t, "echo.star", optionEchoRule)
	if opts != nil {
		r.Configure(opts)
	}
	for _, v := range r.Check(&LintContext{}) {
		switch {
		case v.RuleFailure:
			warnings = append(warnings, v)
		case strings.HasPrefix(v.Message, "get_option="):
			getOpt = strings.TrimPrefix(v.Message, "get_option=")
		case strings.HasPrefix(v.Message, "get_list_option="):
			getList = strings.TrimPrefix(v.Message, "get_list_option=")
		}
	}
	return
}

func TestGetOption_YAMLListBecomesStarlarkList(t *testing.T) {
	getOpt, getList, warnings := runEcho(t, map[string]any{"xs": []any{"ACT_", "TEST_"}})
	if getOpt != `["ACT_", "TEST_"]` {
		t.Errorf("get_option = %s, want the list (a YAML list used to arrive as None)", getOpt)
	}
	if getList != `["ACT_", "TEST_"]` {
		t.Errorf("get_list_option = %s, want the list", getList)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", warnings)
	}
}

func TestGetOption_NestedValuesConvertRecursively(t *testing.T) {
	v, err := goValueToStarlark(map[string]any{"a": []any{1, "b", true, []any{2.5}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != `{"a": [1, "b", True, [2.5]]}` {
		t.Errorf("got %s", got)
	}
}

func TestGetListOption_CommaSeparatedString(t *testing.T) {
	_, getList, warnings := runEcho(t, map[string]any{"xs": " ACT_, TEST_ ,,GET_"})
	if getList != `["ACT_", "TEST_", "GET_"]` {
		t.Errorf("get_list_option = %s, want the trimmed, non-empty parts", getList)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", warnings)
	}
}

func TestGetListOption_UnsetUsesDefault(t *testing.T) {
	getOpt, getList, warnings := runEcho(t, nil)
	if getOpt != `"unset"` || getList != `["DEF_"]` {
		t.Errorf("get_option = %s, get_list_option = %s, want the defaults", getOpt, getList)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", warnings)
	}
}

// A value of the wrong shape is reported, and the rule still runs on its
// default rather than on None.
func TestGetListOption_MalformedValueWarnsAndUsesDefault(t *testing.T) {
	for name, val := range map[string]any{
		"number":          5,
		"list of numbers": []any{"ACT_", 7},
		"map":             map[string]any{"a": "b"},
	} {
		t.Run(name, func(t *testing.T) {
			_, getList, warnings := runEcho(t, map[string]any{"xs": val})
			if getList != `["DEF_"]` {
				t.Errorf("get_list_option = %s, want the default", getList)
			}
			if len(warnings) != 1 {
				t.Fatalf("got %d warnings, want 1: %+v", len(warnings), warnings)
			}
			w := warnings[0]
			if w.Severity != SeverityWarning || w.RuleID != "OPT001" {
				t.Errorf("warning = %+v, want a warning for OPT001", w)
			}
			for _, want := range []string{"rules.OPT001.options.xs", "list of strings or a comma-separated string", "default"} {
				if !strings.Contains(w.Message, want) {
					t.Errorf("warning %q does not mention %q", w.Message, want)
				}
			}
		})
	}
}

// A warning belongs to the run that read the option: a second run with a
// fixed config must not repeat it.
func TestGetListOption_WarningDoesNotLeakIntoNextRun(t *testing.T) {
	r := loadRuleSource(t, "echo.star", optionEchoRule)
	r.Configure(map[string]any{"xs": 5})
	_ = r.Check(&LintContext{})
	r.Configure(map[string]any{"xs": "A_"})
	for _, v := range r.Check(&LintContext{}) {
		if v.RuleFailure {
			t.Errorf("stale warning: %+v", v)
		}
	}
}

// The options map is what yaml.v3 makes of lint-config.yaml; this pins that a
// flow-style and a block-style list both arrive in the shape get_list_option
// reads.
func TestGetListOption_FromLintConfigYAML(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lint-config.yaml", `rules:
  OPT001:
    options:
      xs: [ACT_, TEST_]
  OPT002:
    options:
      xs:
        - GET_
`)
	cfg, err := LoadConfig(filepath.Join(dir, "lint-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, got, warnings := runEcho(t, cfg.Rules["OPT001"].Options)
	if got != `["ACT_", "TEST_"]` || len(warnings) != 0 {
		t.Errorf("flow list: get_list_option = %s, warnings %+v", got, warnings)
	}
	_, got, warnings = runEcho(t, cfg.Rules["OPT002"].Options)
	if got != `["GET_"]` || len(warnings) != 0 {
		t.Errorf("block list: get_list_option = %s, warnings %+v", got, warnings)
	}
}
