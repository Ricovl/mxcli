// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ako/mxcli#1790: a project with a source index rebuilt all of it whenever the
// catalog went stale — every save — describing every document again. The
// build now carries a document's row over when nothing its text depends on
// changed. Reuse is only safe if it never changes the answer, so every step
// below compares the incremental build with a forced full one, row for row.

// sourceBuild builds a source catalog, incremental unless force, and returns
// its rows (ElementId|QualifiedName|ObjectType -> text) and the build output.
func sourceBuild(t *testing.T, exec *Executor, force bool) (map[string]string, string) {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	var out bytes.Buffer
	ctx.Output = &out
	ctx.catalogForce = force
	if err := buildCatalog(ctx, true, true, false, 0); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	defer func() { ctx.Catalog.Close(); ctx.Catalog = nil }()
	res, err := ctx.Catalog.Query(`SELECT ElementId, QualifiedName, ObjectType, SourceText FROM source`)
	if err != nil {
		t.Fatalf("query source: %v", err)
	}
	rows := map[string]string{}
	for _, r := range res.Rows {
		key := fmt.Sprintf("%v|%v|%v", r[0], r[1], r[2])
		if _, dup := rows[key]; dup {
			t.Errorf("source row %s written twice", key)
		}
		rows[key] = fmt.Sprint(r[3])
	}
	return rows, out.String()
}

var reusedRe = regexp.MustCompile(`source: (\d+) reused, (\d+) described`)

// reusedCount is how many rows the build carried over, from its progress line.
func reusedCount(out string) int {
	m := reusedRe.FindStringSubmatch(out)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func diffSourceRows(t *testing.T, step string, got, want map[string]string) {
	t.Helper()
	var keys []string
	for k := range want {
		keys = append(keys, k)
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	bad := 0
	for _, k := range keys {
		g, gok := got[k]
		w, wok := want[k]
		switch {
		case !gok:
			t.Errorf("%s: the incremental build has no row for %s", step, k)
		case !wok:
			t.Errorf("%s: the incremental build kept a row the full build does not have: %s", step, k)
		case g != w:
			t.Errorf("%s: %s differs from the full build:\n--- incremental\n%s\n--- full\n%s", step, k, g, w)
		default:
			continue
		}
		if bad++; bad == 5 {
			t.Fatalf("%s: stopping after %d differences", step, bad)
		}
	}
}

func TestSourceCatalog_IncrementalBuildEqualsFullBuild(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, redeclareChain(4, "create")); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	// A page whose describe prints what another document says: the context
	// entity of a microflow data source is the flow's return type.
	if err := agreeExec(t, exec, `mdl 1;
create persistent entity MyFirstModule.ProbeA (Name: String(50));
create persistent entity MyFirstModule.ProbeB (Name: String(50));
create microflow MyFirstModule.GetProbe () returns MyFirstModule.ProbeA as $A
begin
  $A = create MyFirstModule.ProbeA (Name = 'a');
  return $A;
end;
create page MyFirstModule.ProbePage (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  dataview dataView1 (DataSource: microflow MyFirstModule.GetProbe) {
    textbox txtName (Attribute: Name)
  }
};
`); err != nil {
		t.Fatalf("setup page: %v\n%s", err, out.String())
	}
	base, _ := sourceBuild(t, exec, true)
	if len(base) < 50 {
		t.Fatalf("control: the fixture's source index has %d rows", len(base))
	}

	// Each step edits the project the way an agent does between two
	// rebuilds, and names what the incremental build must not get wrong.
	steps := []struct {
		name, mdl string
	}{
		// Unchanged project: everything is reused, and that must change nothing.
		{"no change", ""},
		// One flow's own bytes change.
		{"edit a flow", strings.Replace(redeclareChain(2, "create or modify"), "length($Title) + $Prev", "length($Title) * 2 + $Prev", 1)},
		// A callee's signature changes: its callers' describes resolve it.
		{"callee returns another type", `mdl 1;
create or modify microflow MyFirstModule.MF_Chain001 ($Title: String) returns Boolean as $B
begin
  declare $B Boolean = length($Title) > 3;
  return $B;
end;
`},
		// The page's own unit is untouched, and its text still changes: the
		// data source flow now returns another entity.
		{"data source flow returns another entity", `mdl 1;
create or modify microflow MyFirstModule.GetProbe () returns MyFirstModule.ProbeB as $B
begin
  $B = create MyFirstModule.ProbeB (Name = 'b');
  return $B;
end;
`},
		// A new document, and a document that calls it.
		{"add a flow", `mdl 1;
create microflow MyFirstModule.MF_Added ($Title: String) returns Integer as $N
begin
  $N = call microflow MyFirstModule.MF_Chain004(Title = $Title);
  return $N;
end;
`},
		// A rename rewrites the callers too; the old name's row must go.
		{"rename a flow", "rename microflow MyFirstModule.MF_Chain004 to MF_Chain004b;"},
		// A domain model change re-describes its entities and what uses them.
		{"alter an entity", "alter entity FeedbackModule.Feedback add attribute IncrementalProbe: String;"},
		// A new folder is a unit no describe covers: everything is described.
		{"move a flow to a new folder", "move microflow MyFirstModule.MF_Chain001 to folder 'Probes';"},
		// A move into an existing folder changes no unit's bytes, only its
		// container, and the describe prints the folder.
		{"move a flow into that folder", "move microflow MyFirstModule.MF_Chain002 to folder 'Probes';"},
		// A deleted document's row must not survive.
		{"drop a flow", "drop microflow MyFirstModule.MF_Added;"},
	}
	reusedAny := false
	for _, s := range steps {
		if s.mdl != "" {
			if err := agreeExec(t, exec, s.mdl); err != nil {
				t.Fatalf("%s: %v\n%s", s.name, err, out.String())
			}
		}
		got, buildOut := sourceBuild(t, exec, false)
		n := reusedCount(buildOut)
		if n > 0 {
			reusedAny = true
		}
		if s.name == "no change" && n != len(base) {
			t.Errorf("no change: reused %d of %d rows, want all\n%s", n, len(base), buildOut)
		}
		want, _ := sourceBuild(t, exec, true)
		diffSourceRows(t, s.name, got, want)
		t.Logf("%s: reused %d of %d rows", s.name, n, len(want))
	}
	if !reusedAny {
		t.Fatalf("control: no build reused a row, so the comparison proves nothing about reuse")
	}
}
