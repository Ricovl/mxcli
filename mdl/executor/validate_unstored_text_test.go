// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#706: an index name is accepted and discarded — DomainModels$Index
// has no name, so `describe` prints `index (…)` and `drop index Name` cannot
// find it. The name is still the required part of `create index X on E (…)`,
// so it is a warning, not a refusal: the index itself is written correctly.
func indexNameWarnings(t *testing.T, script string) []linter.Violation {
	t.Helper()
	return ruleViolations(t, script, indexNameRule)
}

func ruleViolations(t *testing.T, script, rule string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == rule {
			out = append(out, v)
		}
	}
	return out
}

func TestIndexNameIsWarnedAboutInEverySpelling(t *testing.T) {
	for _, tc := range []struct{ name, script, idx string }{
		{"entity body", `create persistent entity M.Cell ("Row": integer) index "IdxRow" on ("Row");`, "IdxRow"},
		{"alter add index", `alter entity M.Cell add index IdxRow ("Row");`, "IdxRow"},
		{"create index", `create index IdxRow on M.Cell ("Row");`, "IdxRow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vs := indexNameWarnings(t, tc.script)
			if len(vs) != 1 {
				t.Fatalf("got %d %s warnings, want 1: %+v", len(vs), indexNameRule, vs)
			}
			v := vs[0]
			if v.Severity != linter.SeverityWarning {
				t.Errorf("severity %v, want warning — the index is written correctly, only the name is lost", v.Severity)
			}
			if !strings.Contains(v.Message, tc.idx) || !strings.Contains(v.Suggestion, "index (") {
				t.Errorf("message should name %q and suggest the anonymous form:\n%s\n%s", tc.idx, v.Message, v.Suggestion)
			}
		})
	}
}

// The control: the anonymous form — what describe emits — is silent, as is a
// drop by column list, which also carries no name.
func TestAnonymousIndexIsNotWarnedAbout(t *testing.T) {
	if vs := indexNameWarnings(t, `create persistent entity M.Cell ("Row": integer) index ("Row");
alter entity M.Cell add index ("Row");
alter entity M.Cell drop index ("Row");`); len(vs) != 0 {
		t.Errorf("anonymous indexes warned: %+v", vs)
	}
}

// ako/mxcli#706: a `/** … */` on an enumeration value is read and written
// nowhere — Enumerations$EnumerationValue has no documentation property. The
// enumeration itself is correct, so this warns rather than refuses.
func TestEnumerationValueDocCommentIsWarnedAbout(t *testing.T) {
	vs := ruleViolations(t, `create enumeration M.Status (
  /** Waiting for approval */
  Pending 'Pending',
  Done 'Done'
);`, enumValueDocRule)
	if len(vs) != 1 {
		t.Fatalf("got %d %s warnings, want 1: %+v", len(vs), enumValueDocRule, vs)
	}
	if vs[0].Severity != linter.SeverityWarning {
		t.Errorf("severity %v, want warning", vs[0].Severity)
	}
	if !strings.Contains(vs[0].Message, "Pending") || !strings.Contains(vs[0].Suggestion, "--") {
		t.Errorf("message should name the value and suggest a -- comment:\n%s\n%s", vs[0].Message, vs[0].Suggestion)
	}
}

// The control: the enumeration's own doc comment IS stored, and must not warn.
func TestEnumerationDocCommentIsNotWarnedAbout(t *testing.T) {
	if vs := ruleViolations(t, `/** Order lifecycle */
create enumeration M.Status (
  -- waiting for approval
  Pending 'Pending'
);`, enumValueDocRule); len(vs) != 0 {
		t.Errorf("warned about stored documentation: %+v", vs)
	}
}
