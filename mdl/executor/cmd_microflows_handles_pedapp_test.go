// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// pedAppEnv names a pristine, Studio Pro-authored PedApp.mpr. The target
// resolver has to be proven on a flow Studio Pro drew: its object order,
// merges and captions are what an mxcli-authored flow cannot reproduce.
// Until the fixture is committed (ako/mxcli#703) the test runs only when this
// points at a local copy; the project is copied before it is opened.
const pedAppEnv = "MXCLI_PEDAPP_MPR"

func openPedApp(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	src := os.Getenv(pedAppEnv)
	if src == "" {
		t.Skipf("set %s to a pristine PedApp.mpr to run the Studio Pro-authored resolver test (fixture pending ako/mxcli#703)", pedAppEnv)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, filepath.Base(src))
	if err := copyPedAppFile(src, dst); err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
	if contents := filepath.Join(filepath.Dir(src), "mprcontents"); dirExists(contents) {
		if err := copyPedAppTree(contents, filepath.Join(dir, "mprcontents")); err != nil {
			t.Fatalf("copy mprcontents: %v", err)
		}
	}

	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: dst}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	return exec, out
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func copyPedAppFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

func copyPedAppTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyPedAppFile(p, target)
	})
}

// valFeedbackTargets returns the resolver candidates of the stored
// FeedbackModule.VAL_Feedback, in describe order.
func valFeedbackTargets(t *testing.T, exec *Executor) []mfmutator.Candidate {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	microflowNames := getMicroflowNames(ctx, h)
	var mf *microflows.Microflow
	for _, m := range all {
		microflowNames[m.ID] = h.GetQualifiedName(m.ContainerID, m.Name)
		if m.Name == "VAL_Feedback" && h.GetModuleName(h.FindModuleID(m.ContainerID)) == "FeedbackModule" {
			mf = m
		}
	}
	if mf == nil {
		t.Fatal("FeedbackModule.VAL_Feedback not found: is this PedApp?")
	}
	cands, _, _, _ := microflowTargets(ctx, mf, getEntityNames(ctx, h), microflowNames)
	return cands
}

func pos(c mfmutator.Candidate) string {
	p := c.Object.GetPosition()
	return fmt.Sprintf("(%d, %d)", p.X, p.Y)
}

// Each addressing form, on the flow Studio Pro drew. Positions identify the
// activities: they are Studio Pro's, and describe prints them as @position.
func TestMicroflowTargets_PedAppValFeedback(t *testing.T) {
	exec, _ := openPedApp(t)
	cands := valFeedbackTargets(t, exec)

	resolves := []struct {
		target, at string
		kind       any
	}{
		{"$IsValidEmail", "(980, 200)", &microflows.ActionActivity{}},
		{"$ValidFeedback", "(-390, 200)", &microflows.ActionActivity{}},
		{"'Email is Valid?'", "(1155, 200)", &microflows.ExclusiveSplit{}},
		{"'Subject not empty?'", "(-215, 200)", &microflows.ExclusiveSplit{}},
		{"if not($IsValidEmail) then", "(1155, 200)", &microflows.ExclusiveSplit{}}, // as describe prints it
		{"if $IsValidEmail then", "(1155, 200)", &microflows.ExclusiveSplit{}},      // as it is stored
		{"$IsValidEmail = call java action FeedbackModule.ValidateEmail *", "(980, 200)", &microflows.ActionActivity{}},
		{"validation feedback $Feedback/SubmitterEmail * 'Email is required'", "(770, 325)", &microflows.ActionActivity{}},
		{"validation feedback $Feedback/SubmitterEmail * @2", "(770, 325)", &microflows.ActionActivity{}},
		{"set $ValidFeedback = false @3", "(1305, 460)", &microflows.ActionActivity{}},
		{"return $ValidFeedback", "(1640, 200)", &microflows.EndEvent{}},
	}
	for _, tc := range resolves {
		c, err := mfmutator.ResolveText(cands, tc.target)
		if err != nil {
			t.Errorf("%s: %v", tc.target, err)
			continue
		}
		if pos(c) != tc.at || fmt.Sprintf("%T", c.Object) != fmt.Sprintf("%T", tc.kind) {
			t.Errorf("%s: resolved to %T at %s, want %T at %s", tc.target, c.Object, pos(c), tc.kind, tc.at)
		}
	}

	// `set $ValidFeedback = false` occurs three times. The resolver refuses,
	// and lists the three in describe order with the ordinal for each.
	_, err := mfmutator.ResolveText(cands, "set $ValidFeedback = false")
	var amb *mfmutator.AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("set $ValidFeedback = false: want an ambiguity error, got %v", err)
	}
	var at []string
	for _, m := range amb.Matches {
		at = append(at, pos(m))
	}
	if got, want := strings.Join(at, " "), "(135, 460) (420, 460) (1305, 460)"; got != want {
		t.Errorf("ambiguous matches at %s, want %s", got, want)
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(err.Error(), fmt.Sprintf("set $ValidFeedback = false @%d", i)) {
			t.Errorf("error does not offer @%d:\n%v", i, err)
		}
	}

	// A $variable that no activity outputs is not found, even though `set`
	// assigns one of that name; the forms do not fall back on each other.
	if _, err := mfmutator.ResolveText(cands, "$Feedback"); err == nil {
		t.Error("$Feedback is a parameter, not an activity output: want an error")
	}
}

var positionLine = regexp.MustCompile(`^\s*@position\((-?\d+), (-?\d+)\)`)

// Every handle `describe … with handles` prints resolves to the activity it is
// printed above, and removing the handles leaves the plain description.
func TestDescribeWithHandles_PedAppValFeedback(t *testing.T) {
	exec, out := openPedApp(t)
	cands := valFeedbackTargets(t, exec)

	describe := func(src string) string {
		out.Reset()
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse %q: %v", src, errs[0])
		}
		for _, s := range prog.Statements {
			if err := exec.Execute(s); err != nil {
				t.Fatalf("%s: %v", src, err)
			}
		}
		return out.String()
	}
	withHandles := describe("describe microflow FeedbackModule.VAL_Feedback with handles;")
	plain := describe("describe microflow FeedbackModule.VAL_Feedback;")

	lines := strings.Split(withHandles, "\n")
	handles := 0
	var kept []string
	for i, line := range lines {
		h, ok := strings.CutPrefix(strings.TrimSpace(line), "-- handle: ")
		if !ok {
			kept = append(kept, line)
			continue
		}
		handles++
		c, err := mfmutator.ResolveText(cands, h)
		if err != nil {
			t.Errorf("handle %q: %v", h, err)
			continue
		}
		m := positionLine.FindStringSubmatch(lines[i+1])
		if m == nil {
			t.Errorf("handle %q is not followed by the activity's @position:\n%s", h, lines[i+1])
			continue
		}
		if want := "(" + m[1] + ", " + m[2] + ")"; pos(c) != want {
			t.Errorf("handle %q printed above the activity at %s resolves to the one at %s", h, want, pos(c))
		}
	}
	// 16 activities, the void-less end event included; merges and the start
	// event are not addressable.
	if handles != len(cands) || handles != 16 {
		t.Errorf("printed %d handles for %d addressable activities, want 16", handles, len(cands))
	}
	if strings.Join(kept, "\n") != plain {
		t.Errorf("with handles minus the handle lines differs from plain describe")
	}
}
