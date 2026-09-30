// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The re-run property of `create or modify microflow|nanoflow` (ako/mxcli#859):
// a flow statement executed a second time, on the flow its first execution
// wrote, writes nothing — under `mdl 1` as under mdl 0.
//
// #850's parity property (TestPedAppFlowSpliceParity) checks describe → exec of
// stored flows, so the statements it runs are describe's own spelling. It
// cannot see an AUTHORED spelling describe prints differently — a guard clause
// directly before the final return, a guard nested in an if without else, a
// row the builder wrapped — which re-ran as a change the splice could not make:
// rebuilt under mdl 0, refused under mdl 1. This runs authored scripts.
//
// For every mdl-examples script that creates a flow:
//
//  1. upgrade it to mdl 1 with the header (fmt --upgrade --header; a script the
//     header is refused for keeps mdl 0 and is run as it is);
//  2. execute it on a fresh PedApp copy — a script that does not execute
//     cleanly there is out of scope, and listed;
//  3. execute its flow statements again, each as `create or modify`, under the
//     same header;
//  4. require every one to succeed without the MDL-V1-REBUILD fallback, and the
//     second execution to write nothing.
//
// A flow a later statement of the script alters, drops, renames or moves is
// left out of step 3: re-running its create would undo that statement, which
// is a change. The controls (TestFlowRerunProperty_Controls) show the check
// sees a change, a refusal and a rebuild.
//
// MXCLI_RERUN_EXAMPLES=<regexp> limits the run to matching script paths, and
// MXCLI_RERUN_SHARD=i/n runs every n-th script from the i-th (as
// MXCLI_UPGRADE_SHARD does for the upgrade property).
func TestFlowRerunProperty(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	sh, err := parseShard(os.Getenv("MXCLI_RERUN_SHARD"))
	if err != nil {
		t.Fatal(err)
	}
	var clean, outOfScope, keptVersion, noFlows, allowed []string
	flows, k := 0, 0
	for _, path := range rerunExampleScripts(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel("../..", path)
		rel = filepath.ToSlash(rel)
		// Negative tests: a script that does not parse, or a .fail.mdl that
		// check must refuse — exec writes what check refuses, and describe
		// cannot always read it back.
		if strings.HasSuffix(rel, ".fail.mdl") {
			continue
		}
		if _, errs := visitor.Build(string(src)); len(errs) > 0 {
			continue
		}
		res, err := upgrade.Upgrade(string(src), upgrade.Options{AddHeader: true})
		var hb *upgrade.HeaderBlockedError
		if errors.As(err, &hb) {
			keptVersion = append(keptVersion, rel)
			res, err = upgrade.Upgrade(string(src), upgrade.Options{})
		}
		if err != nil {
			continue // the upgrade property reports these
		}
		prog, errs := visitor.Build(res.Source)
		if len(errs) > 0 {
			continue
		}
		rerun := flowRerun(prog)
		if len(rerun.Statements) == 0 {
			noFlows = append(noFlows, rel)
			continue
		}
		mine := sh.has(k)
		k++
		if !mine {
			continue
		}
		t.Run(rel, func(t *testing.T) {
			h.restore()
			if err := h.exe.ExecuteProgram(prog); err != nil {
				outOfScope = append(outOfScope, fmt.Sprintf("%s: %s", rel, firstLine(err.Error())))
				return
			}
			flows += len(rerun.Statements)
			first := h.snapshot()
			h.out.Reset()
			err := h.exe.ExecuteProgram(rerun)
			out := h.out.String()
			changed := first.diff(h.snapshot())
			var problems []string
			if err != nil {
				problems = append(problems, "the second execution failed: "+firstLine(err.Error()))
			}
			if strings.Contains(out, "MDL-V1-REBUILD") {
				problems = append(problems, "the second execution fell back to a rebuild:\n"+out)
			}
			if len(changed) != 0 {
				problems = append(problems, fmt.Sprintf("the second execution wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  ")))
			}
			if why, ok := rerunKnownFailures[rel]; ok {
				if len(problems) == 0 {
					t.Errorf("listed in rerunKnownFailures (%s) but re-runs clean: take it off the list", why)
				}
				allowed = append(allowed, rel)
				return
			}
			for _, p := range problems {
				t.Error(p)
			}
			if len(problems) == 0 {
				clean = append(clean, rel)
			}
		})
	}
	sort.Strings(outOfScope)
	t.Logf("%d scripts re-run their %d flow statements writing nothing", len(clean), flows)
	t.Logf("%d scripts are listed in rerunKnownFailures", len(allowed))
	t.Logf("%d scripts create no flow a re-run can repeat", len(noFlows))
	t.Logf("%d scripts keep mdl 0 (the header is refused for them)", len(keptVersion))
	t.Logf("%d scripts are out of scope: they do not execute cleanly on PedApp:\n  %s",
		len(outOfScope), strings.Join(outOfScope, "\n  "))
	if floor := 40 / sh.count; os.Getenv("MXCLI_RERUN_EXAMPLES") == "" && len(clean) < floor {
		t.Errorf("only %d scripts re-ran cleanly (shard %s) — the harness is not exercising the property", len(clean), sh)
	}
}

// rerunKnownFailures are scripts whose flows do not re-run clean yet, each with
// the issue that tracks it. The list may only shrink: a listed script that
// re-runs clean fails the test until it is taken off.
var rerunKnownFailures = map[string]string{
	"mdl-examples/doctype-tests/06b-soap-examples.mdl": "ako/mxcli#861: a call web service activity is re-spliced on every run",
}

// flowRerun is the second execution of a script: its flow statements, each as
// `create or modify`, under the script's language header — the last definition
// of each flow, and none a later statement alters, drops, renames or moves.
func flowRerun(prog *ast.Program) *ast.Program {
	last := map[string]int{}
	touched := map[string]bool{}
	for i, st := range prog.Statements {
		switch s := st.(type) {
		case *ast.CreateMicroflowStmt:
			last["microflow "+s.Name.String()] = i
		case *ast.CreateNanoflowStmt:
			last["nanoflow "+s.Name.String()] = i
		}
	}
	for i, st := range prog.Statements {
		switch st.(type) {
		case *ast.CreateMicroflowStmt, *ast.CreateNanoflowStmt:
			continue
		}
		kind := reflect.TypeOf(st).Elem().Name()
		if !strings.HasPrefix(kind, "Alter") && !strings.HasPrefix(kind, "Drop") &&
			!strings.HasPrefix(kind, "Rename") && !strings.HasPrefix(kind, "Move") {
			continue
		}
		// Anything that changes the model after a flow is created may change
		// what the flow refers to, so each flow created before it is left out.
		for name, at := range last {
			if at < i {
				touched[name] = true
			}
		}
	}
	out := &ast.Program{LanguageVersion: prog.LanguageVersion, LanguageHeaderLine: prog.LanguageHeaderLine}
	for i, st := range prog.Statements {
		switch s := st.(type) {
		case *ast.CreateMicroflowStmt:
			if key := "microflow " + s.Name.String(); last[key] == i && !touched[key] {
				c := *s
				c.CreateOrModify = true
				out.Statements = append(out.Statements, &c)
			}
		case *ast.CreateNanoflowStmt:
			if key := "nanoflow " + s.Name.String(); last[key] == i && !touched[key] {
				c := *s
				c.CreateOrModify = true
				out.Statements = append(out.Statements, &c)
			}
		}
	}
	return out
}

func rerunExampleScripts(t *testing.T) []string {
	t.Helper()
	var filter *regexp.Regexp
	if f := os.Getenv("MXCLI_RERUN_EXAMPLES"); f != "" {
		filter = regexp.MustCompile(f)
	}
	var out []string
	err := filepath.Walk("../../mdl-examples", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".mdl") && (filter == nil || filter.MatchString(p)) {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("no mdl-examples scripts found")
	}
	return out
}

// Controls for the property: the second execution it checks does see a
// change, a refusal and a rebuild — or a clean pass would prove nothing.
func TestFlowRerunProperty_Controls(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const script = `mdl 1;
create or modify microflow MyFirstModule.Rerun_Control ($N: Integer) returns Integer
begin
  if $N < 0 then
    return 0;
  end if;
  declare $M Integer = $N + 1;
  return $M;
end;
`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	if prog.LanguageVersion != langver.V1 {
		t.Fatalf("the control script is not under mdl 1 (%v)", prog.LanguageVersion)
	}
	rerun := func(src string) (error, string, []string) {
		h.restore()
		if err := h.exe.ExecuteProgram(prog); err != nil {
			t.Fatalf("first execution: %v", err)
		}
		first := h.snapshot()
		p, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		h.out.Reset()
		err := h.exe.ExecuteProgram(flowRerun(p))
		return err, h.out.String(), first.diff(h.snapshot())
	}

	if err, _, changed := rerun(script); err != nil || len(changed) != 0 {
		t.Fatalf("the unchanged script: err=%v, wrote %v", err, changed)
	}
	// A changed activity is a write.
	if _, _, changed := rerun(strings.Replace(script, "$N + 1", "$N + 2", 1)); len(changed) == 0 {
		t.Error("a changed activity wrote nothing: the property cannot see a change")
	}
	// A change the splice cannot make is a refusal under mdl 1 ...
	guardRemoved := strings.Replace(script, "  if $N < 0 then\n    return 0;\n  end if;\n", "", 1)
	if err, _, _ := rerun(guardRemoved); err == nil {
		t.Error("taking out a guard's return was not refused under mdl 1: the property cannot see a refusal")
	}
	// ... and a rebuild under mdl 0.
	if _, out, _ := rerun(strings.TrimPrefix(guardRemoved, "mdl 1;\n")); !strings.Contains(out, "MDL-V1-REBUILD") {
		t.Errorf("taking out a guard's return under mdl 0 did not rebuild:\n%s", out)
	}
}
