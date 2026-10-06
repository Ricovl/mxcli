// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The fixture's flows were drawn in Studio Pro, so their layout is AUTHORED:
// none of it is what the layout engine would produce. `mxcli layout flows` turns
// a flow's layout into exactly what the engine produces, which makes it DERIVED.
// Between the two, the same flow is the test and its own control.

// describeExecutor is layoutExecutor with an output buffer the test can read.
func describeExecutor(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	exec := New(&out)
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	return exec, &out
}

// describeText runs one DESCRIBE and returns what it printed.
func describeText(t *testing.T, exec *Executor, out *bytes.Buffer, stmt string) string {
	t.Helper()
	out.Reset()
	run(t, exec, stmt)
	return out.String()
}

// layoutLines returns the lines of a description that only pin geometry.
func layoutLines(mdl string) []string {
	var got []string
	for _, line := range strings.Split(mdl, "\n") {
		if layoutAnnotationLine.MatchString(line) {
			got = append(got, strings.TrimSpace(line))
		}
	}
	return got
}

// withoutGrant drops the trailing grant, which re-executing a description does
// not need and which a copy under another name could not satisfy.
func withoutGrant(mdl string) string {
	var kept []string
	for _, line := range strings.Split(mdl, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "grant ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// fullGeometry is every object's kind, position and size, and every sequence
// flow's end points, connection sides and bezier vectors — sorted, so two flows
// with the same layout compare equal whatever their IDs.
func fullGeometry(t *testing.T, exec *Executor, kind, name string) []string {
	t.Helper()
	_, g := rawFlow(t, exec, kind, name)
	idx := indexFlowGraph(g.objects)
	var out []string
	for _, id := range idx.order {
		o := idx.object[id]
		out = append(out, fmt.Sprintf("%s @%v %v", layoutKind(o), o.GetPosition(), objectSize(o)))
	}
	for _, f := range idx.flow {
		o, d := idx.object[f.OriginID], idx.object[f.DestinationID]
		out = append(out, fmt.Sprintf("flow %s@%v[%d %s] -> %s@%v[%d %s]",
			layoutKind(o), o.GetPosition(), f.OriginConnectionIndex, orZeroVector(f.OriginControlVector),
			layoutKind(d), d.GetPosition(), f.DestinationConnectionIndex, orZeroVector(f.DestinationControlVector)))
	}
	sort.Strings(out)
	return out
}

func sameGeometry(t *testing.T, what string, before, after []string) {
	t.Helper()
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("%s moved the flow:\nbefore:\n  %s\nafter:\n  %s",
			what, strings.Join(before, "\n  "), strings.Join(after, "\n  "))
	}
}

// A flow whose whole layout is what the engine derives describes with no layout
// annotation at all, and re-executing that description puts everything back
// exactly where it was.
func TestDescribe_DerivedLayoutIsOmitted(t *testing.T) {
	exec, out := describeExecutor(t)
	for _, tc := range []struct{ kind, name string }{
		{"microflow", "Administration.ChangeMyPassword"},
		{"microflow", "Administration.SaveNewAccount"},
		{"nanoflow", "FeedbackModule.ACT_Feedback_UploadImage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := exec.LayoutFlow(tc.kind, flowQN(tc.name), false)
			if err != nil || res.Refused != "" {
				t.Fatalf("layout: %v %s", err, res.Refused)
			}
			before := fullGeometry(t, exec, tc.kind, tc.name)

			mdl := describeText(t, exec, out, "describe "+tc.kind+" "+tc.name+";")
			if got := layoutLines(mdl); len(got) > 0 {
				t.Errorf("a derived layout was described (%d line(s)):\n  %s\n--- describe ---\n%s",
					len(got), strings.Join(got, "\n  "), mdl)
			}

			run(t, exec, withoutGrant(mdl))
			sameGeometry(t, "re-executing the description", before, fullGeometry(t, exec, tc.kind, tc.name))
		})
	}
}

// A Studio Pro-drawn flow keeps its layout: re-executing its canonical
// description lands every node, anchor and curve exactly where re-executing the
// FULL description (every layout annotation, as describe emitted before #748)
// does. The full description is the reference rather than the stored flow,
// because some stored geometry — Studio Pro's box sizes, the curve on the flow
// out of the start event — has no MDL at all (#721 A), and that loss is not
// this change's to judge.
//
// The control is the same description with its layout lines deleted: it lands
// elsewhere, which proves the fixture's layout is authored and that
// fullGeometry sees the difference.
func TestDescribe_AuthoredLayoutIsKept(t *testing.T) {
	exec, out := describeExecutor(t)
	for _, name := range []string{
		"Administration.ChangeMyPassword",
		"Administration.SaveNewAccount",
		"FeedbackModule.SUB_Feedback_Sanitize",
	} {
		t.Run(name, func(t *testing.T) {
			asCopy := func(mdl, suffix string) string {
				copyName := name + suffix
				script := strings.Replace(withoutGrant(mdl), "microflow "+name+" ", "microflow "+copyName+" ", 1)
				if !strings.Contains(script, copyName) {
					t.Fatalf("could not rename the described microflow:\n%s", script)
				}
				run(t, exec, script)
				return copyName
			}

			full, _, err := describeMicroflowToString(exec.newExecContext(t.Context()), flowQN(name))
			if err != nil {
				t.Fatal(err)
			}
			canonical := describeText(t, exec, out, "describe microflow "+name+";")
			if len(layoutLines(canonical)) == 0 {
				t.Fatalf("a Studio Pro-drawn flow was described with no layout at all:\n%s", canonical)
			}
			if len(layoutLines(canonical)) > len(layoutLines(full)) {
				t.Errorf("canonical description has more layout lines (%d) than the full one (%d)",
					len(layoutLines(canonical)), len(layoutLines(full)))
			}

			want := fullGeometry(t, exec, "microflow", asCopy(full, "_Full"))

			var stripped []string
			for _, line := range strings.Split(full, "\n") {
				if !layoutAnnotationLine.MatchString(line) {
					stripped = append(stripped, line)
				}
			}
			if strings.Join(fullGeometry(t, exec, "microflow", asCopy(strings.Join(stripped, "\n"), "_Stripped")), "\n") == strings.Join(want, "\n") {
				t.Fatal("control: the flow rebuilt without layout lines landed where it was, so this fixture cannot tell authored from derived")
			}

			sameGeometry(t, "the canonical description", want, fullGeometry(t, exec, "microflow", asCopy(canonical, "_Canonical")))
		})
	}
}

// Mixed: the first statement placed by hand, the rest by the engine. The engine
// places each statement after the one before it, so the statements after the
// hand-placed one are derived from it and need no annotation of their own —
// and neither does the start, which sits one step before the first statement.
// A second describe of the re-executed flow is the same text (PutGet).
//
// The script ends in an explicit `return;` because that is what DESCRIBE
// prints: the implicit end event of a flow that just stops is placed half a
// step further right than a `return;` is, so it is not derivable from the
// description and keeps its @position.
func TestDescribe_OnlyAuthoredPositionsAreKept(t *testing.T) {
	exec, out := describeExecutor(t)
	const name = "MyFirstModule.DerivedLayoutMixed"
	run(t, exec, `create microflow `+name+` ()
begin
  @position(400, 320)
  log info node 'Test' 'first';
  log info node 'Test' 'second';
  log info node 'Test' 'third';
  return;
end;`)
	before := fullGeometry(t, exec, "microflow", name)

	mdl := describeText(t, exec, out, "describe microflow "+name+";")
	if got := layoutLines(mdl); len(got) != 1 || got[0] != "@position(400, 320)" {
		t.Errorf("want exactly the one authored @position(400, 320), got %q:\n%s", got, mdl)
	}

	run(t, exec, withoutGrant(mdl))
	sameGeometry(t, "re-executing the description", before, fullGeometry(t, exec, "microflow", name))
	if again := describeText(t, exec, out, "describe microflow "+name+";"); again != mdl {
		t.Errorf("second describe differs:\nfirst:\n%s\nsecond:\n%s", mdl, again)
	}
}

// The catalog's source build does not run the derivation (#766). Its text feeds
// a search index, where layout lines are harmless, and the derivation rebuilds
// every flow several times — it made `refresh catalog full source` 2.3x slower.
// Observable as the full layout in the catalog text of a flow whose canonical
// description has none; the canonical describe of the same flow is the control.
func TestCatalogSource_SkipsDerivedLayout(t *testing.T) {
	exec, out := describeExecutor(t)
	const name = "Administration.ChangeMyPassword"
	if res, err := exec.LayoutFlow("microflow", flowQN(name), false); err != nil || res.Refused != "" {
		t.Fatalf("layout: %v %s", err, res.Refused)
	}
	if got := layoutLines(describeText(t, exec, out, "describe microflow "+name+";")); len(got) > 0 {
		t.Fatalf("control: the canonical description keeps layout, so this cannot tell the two apart:\n  %s",
			strings.Join(got, "\n  "))
	}
	src, err := captureDescribeParallel(exec.newExecContext(t.Context()), "MICROFLOW", name, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(layoutLines(src)) == 0 {
		t.Errorf("the catalog source ran the derived-layout check:\n%s", src)
	}
}

// handLaidIfChain is a flow of n if/else blocks with every node placed by hand,
// none where the layout engine would put it — the shape of mendixlabs/mxcli#1301.
func handLaidIfChain(name string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "create or modify microflow %s ($X: integer)\nbegin\n", name)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  @position(%d, %d)\n  if $X > %d then\n", 200+i*137, 100+(i%3)*53, i)
		fmt.Fprintf(&b, "    @position(%d, %d)\n    log info node 'T' 'a%d';\n  else\n", 260+i*137, 300+(i%5)*31, i)
		fmt.Fprintf(&b, "    @position(%d, %d)\n    log info node 'T' 'b%d';\n  end if;\n", 250+i*137, 500+(i%4)*29, i)
	}
	b.WriteString("  return;\nend;")
	return b.String()
}

// The derivation re-renders the description once per round, and a hand-laid
// flow takes several rounds. The stored flow's graph analysis — which merge
// closes which split, the body warnings — does not depend on which layout
// annotations a round keeps, and it grows faster than the flow, so running it
// per round made describe of a 60-block hand-laid flow 7.6x slower than v0.24
// (mendixlabs/mxcli#1301). It runs once, shared by the derivation and the
// description that is printed.
//
// The rounds are the control: a flow that settles in one round would pass
// without the memo too, so the test first requires several.
func TestDescribe_DerivedLayoutAnalysesTheFlowOnce(t *testing.T) {
	exec, out := describeExecutor(t)
	const name = "MyFirstModule.HandLaidChain"
	run(t, exec, handLaidIfChain(name, 12))

	rounds, analyses := derivedLayoutRounds.Load(), splitMergeAnalyses.Load()
	mdl := describeText(t, exec, out, "describe microflow "+name+";")
	rounds, analyses = derivedLayoutRounds.Load()-rounds, splitMergeAnalyses.Load()-analyses

	if rounds < 3 {
		t.Fatalf("control: the derivation took %d round(s), so this flow cannot show a per-round cost", rounds)
	}
	if len(layoutLines(mdl)) == 0 {
		t.Fatalf("a hand-laid flow was described with no layout:\n%s", mdl)
	}
	// One shared by every render, and one for the printed description's
	// dropped-merge warning, which walks the graph its own way.
	if analyses > 2 {
		t.Errorf("the flow's split/merge structure was analysed %d times over %d rounds; want at most 2", analyses, rounds)
	}
}
