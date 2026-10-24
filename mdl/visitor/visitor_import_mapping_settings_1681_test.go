// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1681: "`Microflows$ImportMappingCall.ParameterVariableName`
// (`NewStormglass`, `NewZeeweerActueelWeer`) and `Commit: YesWithoutEvents` …
// MDL has no syntax for the parameter or the commit mode."
//
// The activity's dialog settings are one ( Key: value, … ) list after the main
// operand (ADR-0013), like send email's: `import from mapping M.IMM($Json)
// (Parameter: $Obj, Commit: No) all;`. New syntax, so an unknown key, a repeated
// key or a value of the wrong shape is an error under every language version.
package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func importStmtOf(t *testing.T, body string) (*ast.ImportFromMappingStmt, []error) {
	t.Helper()
	prog, errs := Build("create microflow M.F ($Json: String, $Obj: M.E) begin\n  " + body + "\nend;")
	if prog == nil || len(prog.Statements) == 0 {
		return nil, errs
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok || len(mf.Body) == 0 {
		return nil, errs
	}
	s, _ := mf.Body[0].(*ast.ImportFromMappingStmt)
	return s, errs
}

func TestImportFromMapping_Settings(t *testing.T) {
	for _, tc := range []struct {
		body              string
		param, commit     string
		all, first, bound bool
	}{
		{"$R = import from mapping M.IMM($Json) (Parameter: $Obj, Commit: No) all;", "Obj", "No", true, false, true},
		{"import from mapping M.IMM($Json) (Parameter: $Obj);", "Obj", "", false, false, false},
		{"import from mapping M.IMM($Json) (commit: yeswithoutevents) first;", "", "YesWithoutEvents", false, true, false},
		{"$R = import from mapping M.IMM($Json) (Commit: Yes,) all;", "", "Yes", true, false, true},
		// Control: no list leaves both unset, so the writer keeps its defaults.
		{"$R = import from mapping M.IMM($Json) all;", "", "", true, false, true},
	} {
		s, errs := importStmtOf(t, tc.body)
		if len(errs) > 0 || s == nil {
			t.Fatalf("%s: parse errors %v", tc.body, errs)
		}
		if s.Parameter != tc.param || s.Commit != tc.commit || s.All != tc.all || s.First != tc.first ||
			(s.OutputVariable != "") != tc.bound || s.SourceVariable != "Json" {
			t.Errorf("%s: got Parameter=%q Commit=%q All=%v First=%v Output=%q Source=%q",
				tc.body, s.Parameter, s.Commit, s.All, s.First, s.OutputVariable, s.SourceVariable)
		}
	}
}

func TestImportFromMapping_SettingsRejectsBadInput(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"import from mapping M.IMM($Json) (Parametr: $Obj);", "did you mean 'Parameter'"},
		{"import from mapping M.IMM($Json) (Commit: No, Commit: Yes);", "sets Commit twice"},
		{"import from mapping M.IMM($Json) (Commit: Maybe);", "Yes, YesWithoutEvents or No"},
		{"import from mapping M.IMM($Json) (Parameter: Obj);", "a variable"},
	} {
		_, errs := importStmtOf(t, tc.body)
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		if !strings.Contains(strings.Join(msgs, "\n"), tc.want) {
			t.Errorf("%s: errors %q, want one containing %q", tc.body, msgs, tc.want)
		}
	}
}
