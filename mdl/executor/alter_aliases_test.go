// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func aliasWarnings(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, v := range ValidateProgram(prog, "") {
		if strings.HasPrefix(v.RuleID, "MDL-DEPR") {
			out = append(out, v)
		}
	}
	return out
}

// The old ALTER PAGE spellings still run, and warn with the code that names
// their rewrite (ako/mxcli#712). The canonical script is the control: it must
// produce no deprecation warning at all, or a warning on every ALTER would pass.
func TestAlterAliases_OldSpellingsWarn(t *testing.T) {
	old := aliasWarnings(t, `alter page M.P {
		set Caption = 'Save' on btnSave;
		set (Caption = 'x', ButtonStyle = Success) on btn2;
		set Title: 'T';
		drop widget txtOld;
	};`)
	var codes []string
	for _, v := range old {
		if v.Severity != linter.SeverityWarning {
			t.Errorf("%s must be a warning, got %v", v.RuleID, v.Severity)
		}
		codes = append(codes, v.RuleID)
	}
	want := []string{"MDL-DEPR101", "MDL-DEPR101", "MDL-DEPR102", "MDL-DEPR103"}
	if strings.Join(codes, ",") != strings.Join(want, ",") {
		t.Errorf("codes: got %v, want %v", codes, want)
	}
	if len(old) > 0 && !strings.Contains(old[0].Suggestion, "set (Key: value") {
		t.Errorf("suggestion should name the canonical form: %q", old[0].Suggestion)
	}

	canonical := aliasWarnings(t, `alter page M.P {
		set (Caption: 'Save') on btnSave;
		set (Caption: 'x', ButtonStyle: Success) on btn2;
		set (Title: 'T');
		drop txtOld;
		set layout = Atlas_Core.TopBar;
	};`)
	if len(canonical) != 0 {
		t.Errorf("canonical form must not warn, got %v", canonical)
	}
}

// Every grammar alternative marked `// alias: <code>` has an entry in the
// alias table, and every entry is marked somewhere in the grammar — so an alias
// cannot be added to one without the other. The deprecation registry
// (ako/mxcli#709) generalises this check.
func TestAlterAliasGrammarMarkersMatchTable(t *testing.T) {
	marker := regexp.MustCompile(`//\s*alias:\s*(MDL-DEPR\d+)`)
	files, _ := filepath.Glob("../grammar/*.g4")
	more, _ := filepath.Glob("../grammar/domains/*.g4")
	files = append(files, more...)
	if len(files) == 0 {
		t.Fatal("no grammar files found")
	}
	marked := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range marker.FindAllStringSubmatch(string(b), -1) {
			marked[m[1]] = true
		}
	}
	table := map[string]bool{}
	for _, a := range alterAliases {
		table[a.Code] = true
		if !marked[a.Code] {
			t.Errorf("%s is in the alias table but no grammar alternative is marked `// alias: %s`", a.Code, a.Code)
		}
	}
	var missing []string
	for code := range marked {
		if !table[code] {
			missing = append(missing, code)
		}
	}
	sort.Strings(missing)
	for _, code := range missing {
		t.Errorf("grammar marks an alias %s with no entry in alterAliases", code)
	}
}

// A caption or @n target is a form a page does not use. The generic grammar
// parses it (other document types need it), so check must refuse it — with no
// project — rather than leave exec to stop partway through a script. A name
// target is the control.
func TestAlterPageAddresses_RefusedAtCheck(t *testing.T) {
	errorsOf := func(src string) []linter.Violation {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		var out []linter.Violation
		for _, v := range ValidateProgram(prog, "") {
			if v.RuleID == "MDL-ALTER01" {
				out = append(out, v)
			}
		}
		return out
	}
	bad := errorsOf(`alter page M.P { drop 'Learn more'; replace hdr@2 with { container c1 } };`)
	if len(bad) != 2 {
		t.Fatalf("want 2 MDL-ALTER01 errors, got %v", bad)
	}
	for _, v := range bad {
		if v.Severity != linter.SeverityError {
			t.Errorf("want an error, got %v", v.Severity)
		}
	}
	if ok := errorsOf(`alter page M.P { drop txtOld, dg.Total; replace hdr with { container c1 } };`); len(ok) != 0 {
		t.Errorf("name targets must pass, got %v", ok)
	}
}
