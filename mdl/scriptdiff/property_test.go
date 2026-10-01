// SPDX-License-Identifier: Apache-2.0

//go:build integration

package scriptdiff

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The property `mxcli diff` exists for (ako/mxcli#907): the documents diff
// reports changed are exactly the units exec writes. For every script, diff
// runs first and must not touch the project; exec then runs on the same
// project; the two sets of writes must be equal. Each script then runs a
// second time, which is the twice-exec rule seen from diff: whatever exec 2
// writes (usually nothing) is what diff 2 reports.
//
// Every statement runs, as `exec --continue-on-error` runs them, so a script
// with one failing statement still compares everything else it does.
//
// By default a handful of mdl-examples doctype scripts covering the document
// kinds diff used not to compare run on the PedApp fixture, each on its own
// copy. MXCLI_DIFF_ALL=1 runs every doctype script. MXCLI_DIFF_LEGS names a
// file of legs, one per line — "<project folder>\t<mpr name>\t<script>…",
// scripts relative to the folder and read where they are — each run in order
// on a copy of its project, as the acceptance rehearsal runs them.
func TestDiffFollowsExec_Examples(t *testing.T) {
	scripts := []string{
		"01-domain-model-examples.mdl",
		"02-microflow-examples.mdl",
		"03-page-examples.mdl",
		"08-security-examples.mdl",
		"09-constant-examples.mdl",
		"11-navigation-examples.mdl",
		"14-project-settings-examples.mdl",
		"18-folder-examples.mdl",
		"languages.mdl",
	}
	dir := filepath.Join("..", "..", "mdl-examples", "doctype-tests")
	if os.Getenv("MXCLI_DIFF_ALL") != "" {
		all, err := filepath.Glob(filepath.Join(dir, "*.mdl"))
		if err != nil {
			t.Fatal(err)
		}
		scripts = scripts[:0]
		for _, p := range all {
			scripts = append(scripts, filepath.Base(p))
		}
	}
	only := regexp.MustCompile(os.Getenv("MXCLI_DIFF_EXAMPLES"))
	for _, name := range scripts {
		if !only.MatchString(name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			prog, errs := parseOK(string(src))
			if errs != nil {
				t.Skipf("does not parse: %v", errs)
			}
			mpr := pedAppCopy(t)
			abs, _ := filepath.Abs(dir)
			for run := 1; run <= 2; run++ {
				rep, diffSet, execSet, _, execErr := diffThenExec(t, mpr, abs, prog, true)
				if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
					t.Errorf("run %d: diff and exec disagree on what is written:\n  %s", run, strings.Join(d, "\n  "))
				}
				if (rep.ExecErr == nil) != (execErr == nil) {
					t.Errorf("run %d: diff's exec error %v, exec's %v", run, rep.ExecErr, execErr)
				}
				t.Logf("run %d: %d unit(s), %d file(s) written", run, len(rep.Units), len(rep.Files))
				for _, f := range rep.Files {
					t.Logf("  file %s %s", f.Kind, f.Path)
				}
			}
		})
	}
}

func TestDiffFollowsExec_Legs(t *testing.T) {
	legs := os.Getenv("MXCLI_DIFF_LEGS")
	if legs == "" {
		t.Skip("MXCLI_DIFF_LEGS is not set")
	}
	f, err := os.Open(legs)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		fields := strings.Split(strings.TrimSpace(sc.Text()), "\t")
		if len(fields) < 3 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		projDir, mprName, scripts := fields[0], fields[1], strings.Fields(fields[2])
		t.Run(filepath.Base(filepath.Dir(projDir))+"/"+filepath.Base(projDir), func(t *testing.T) {
			work := filepath.Join(t.TempDir(), filepath.Base(projDir))
			if err := copyProject(projDir, work); err != nil {
				t.Fatal(err)
			}
			mpr := filepath.Join(work, mprName)
			agree, wrote := 0, 0
			for _, s := range scripts {
				// Read in place: a script may name a sibling folder of the
				// project (../model/…), which the copy does not have.
				path := filepath.Join(projDir, s)
				src, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("%s: %v", s, err)
					continue
				}
				prog, errs := parseOK(string(src))
				if errs != nil {
					t.Logf("%s: does not parse: %v", s, errs)
					continue
				}
				_, diffSet, execSet, _, _ := diffThenExec(t, mpr, filepath.Dir(path), prog, true)
				if d := symmetricDiff(diffSet, execSet); len(d) > 0 {
					t.Errorf("%s: diff and exec disagree on what is written:\n  %s", s, strings.Join(d, "\n  "))
					continue
				}
				agree++
				if len(execSet) > 0 {
					wrote++
				}
			}
			t.Logf("%d of %d script(s) agree; exec wrote in %d", agree, len(scripts), wrote)
		})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}
