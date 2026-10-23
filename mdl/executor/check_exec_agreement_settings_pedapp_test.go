// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1549 (ako/mxcli#1447): `check -p` passed `alter settings
// <any section>` — and any key in a section it did know — and only exec refused
// it, so a script could pass the gate and then stop halfway through. Each case
// must now be refused by both, with the message exec gives.

func TestCheckExecAgree_AlterSettingsUnknownSection(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	// The two scripts as reported.
	assertAgree(t, exec, dir, "mdl 1;\nalter settings nosuchsection (Foo: 1);",
		"unknown settings section: nosuchsection")
	// Control: the section the report asked for now exists.
	assertAgree(t, exec, dir, "mdl 1;\nalter settings webui (OptimizedClient: 'Yes');", "")
}

func TestCheckExecAgree_AlterSettingsUnknownKey(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	for _, tc := range []struct{ src, want string }{
		{"mdl 1;\nalter settings runtime (OptimisticLocking: true);", "unknown model setting: OptimisticLocking"},
		{"mdl 1;\nalter settings configuration 'Default' (ExtraJvmParams: '-Dx=1');", "unknown configuration setting: ExtraJvmParams"},
		{"mdl 1;\nalter settings language (DefaultLanguage: 'en_US');", "unknown language setting: DefaultLanguage"},
		{"mdl 1;\nalter settings workflows (UserEntityName: 'System.User');", "unknown workflow setting: UserEntityName"},
		{"mdl 1;\nalter settings webui (UseReactClient: 'Yes');", "unknown webui setting: UseReactClient"},
		{"mdl 1;\ncreate configuration 'Agree' (Port: 8085);", "unknown configuration setting: Port"},
		// A verb the section does not define.
		{"mdl 1;\nalter settings workflows add 'de_DE';", "add/remove is only defined for the LANGUAGE section"},
		{"mdl 1;\nalter settings language add group 'Approvers';", "GROUP is only defined for the WORKFLOWS section"},
	} {
		t.Run(tc.want, func(t *testing.T) { assertAgree(t, exec, dir, tc.src, tc.want) })
	}
	// Controls: the real keys.
	assertAgree(t, exec, dir, "mdl 1;\nalter settings configuration 'Default' (ExtraJvmParameters: '-Dx=1');", "")
	assertAgree(t, exec, dir, "mdl 1;\nalter settings language (DefaultLanguageCode: 'en_US');", "")
}

// Plain `check` — no project — catches it too: neither refusal depends on the
// project, and `make check-mdl` runs without one.
func TestValidateProgram_AlterSettingsUnknownSectionWithoutProject(t *testing.T) {
	prog, errs := visitor.Build("mdl 1;\nalter settings webuii (OptimizedClient: 'Yes');")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var got []linter.Violation
	for _, v := range ValidateProgram(prog, "") {
		if v.Severity == linter.SeverityError {
			got = append(got, v)
		}
	}
	if len(got) != 1 || got[0].RuleID != "MDL-SET04" {
		t.Fatalf("want one MDL-SET04 error, got %+v", got)
	}
	if !strings.HasPrefix(got[0].Message, "unknown settings section: webuii (expected ") ||
		!strings.Contains(got[0].Message, "webui") {
		t.Errorf("message should be exec's, naming the sections that exist: %q", got[0].Message)
	}
}
