// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// mendixlabs/mxcli#1509: QUAL004's entry-point prefixes and CUSTOM002's valid
// prefixes were constants in the rule files, and `mxcli init` rewrites those
// files, so a project with its own naming convention could not keep a change.
// Both now read a rule option from lint-config.yaml and fall back to the old
// lists when it is unset.

// flaggedFlows runs a shipped rule over the flow-kinds fixture (Big_<K> and
// ACT_<K> for each flavour, all unreferenced) and returns the flagged names
// and the rule-failure messages.
func flaggedFlows(t *testing.T, rule string, opts map[string]any) (names []string, failures []string) {
	t.Helper()
	r, err := linter.LoadStarlarkRule(filepath.Join(flowRulesDir, rule))
	if err != nil {
		t.Fatalf("LoadStarlarkRule(%s): %v", rule, err)
	}
	if opts != nil {
		r.Configure(opts)
	}
	for _, v := range r.Check(linter.NewLintContextFromDB(flowKindsFixtureDB(t))) {
		if v.RuleFailure {
			failures = append(failures, v.Message)
			continue
		}
		names = append(names, v.Location.DocumentName)
	}
	sort.Strings(names)
	return names, failures
}

func TestQUAL004_EntryPointPrefixesOption(t *testing.T) {
	const rule = "orphaned_elements.star"
	defaults := []string{"Sales.Big_MF", "Sales.Big_NF", "Sales.Big_RU"}
	bigOnly := []string{"Sales.ACT_MF", "Sales.ACT_NF", "Sales.ACT_RU"}

	for _, tc := range []struct {
		name         string
		opts         map[string]any
		want         []string
		wantFailures int
	}{
		// Control: unset, the built-in list (ACT_ among it) still applies.
		{"unset keeps the built-in list", nil, defaults, 0},
		{"yaml list replaces the list", map[string]any{"entry_point_prefixes": []any{"Big_"}}, bigOnly, 0},
		{"comma-separated string", map[string]any{"entry_point_prefixes": "Big_, ACT_"}, nil, 0},
		{"malformed value warns, keeps the built-in list", map[string]any{"entry_point_prefixes": 3}, defaults, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, failures := flaggedFlows(t, rule, tc.opts)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("flagged %v, want %v", got, tc.want)
			}
			if len(failures) != tc.wantFailures {
				t.Errorf("rule failures %v, want %d", failures, tc.wantFailures)
			}
		})
	}
}

func TestCUSTOM002_ValidPrefixesOption(t *testing.T) {
	const rule = "example_microflow.star"
	defaults := []string{"Sales.Big_MF", "Sales.Big_NF", "Sales.Big_RU"}

	got, _ := flaggedFlows(t, rule, nil)
	if strings.Join(got, ",") != strings.Join(defaults, ",") {
		t.Fatalf("control: flagged %v, want %v", got, defaults)
	}

	got, failures := flaggedFlows(t, rule, map[string]any{"valid_prefixes": []any{"ACT_", "Big_"}})
	if len(got) != 0 || len(failures) != 0 {
		t.Errorf("with valid_prefixes [ACT_, Big_]: flagged %v, failures %v, want none", got, failures)
	}

	// The message names the prefixes in force, not the built-in ones.
	r, err := linter.LoadStarlarkRule(filepath.Join(flowRulesDir, rule))
	if err != nil {
		t.Fatal(err)
	}
	r.Configure(map[string]any{"valid_prefixes": "TEST_,GET_"})
	vs := r.Check(linter.NewLintContextFromDB(flowKindsFixtureDB(t)))
	if len(vs) != 6 {
		t.Fatalf("got %d violations, want all 6 fixture flows", len(vs))
	}
	if !strings.Contains(vs[0].Message, "(TEST_, GET_)") {
		t.Errorf("message %q does not name the configured prefixes", vs[0].Message)
	}
}
