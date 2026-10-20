// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1393: `diff-local --ref HEAD~1..HEAD` printed
//
//	--- Microflow.56594ae3-d5b7-42cd-b230-ffc330b1b8b2 (current)
//	+++ Microflow.56594ae3-d5b7-42cd-b230-ffc330b1b8b2 (script)
//
// with the module dropped from the rendered MDL, and a page whose container
// gained a conditional Visible reported "Summary: 0 new, 1 modified, 0 deleted"
// with no hunk at all.
//
// Two causes for the name: an MPR v2 .mxunit does not hold its container (the
// Unit table in the .mpr does), so the "$Container" lookup never hit; and
// ObjectName was set to the file's UUID regardless. The labels were the script
// diff's, hard-coded. The empty hunk: pages rendered a fixed
// "-- (page widgets)" body, so every widget change rendered identically.
package executor

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	rr1393Module = "11111111-1111-1111-1111-111111111111"
	rr1393Folder = "22222222-2222-2222-2222-222222222222"
	rr1393Unit   = "56594ae3-d5b7-42cd-b230-ffc330b1b8b2"
)

// stubGitRevisions makes execCommand answer `git diff --name-status` with one
// modified unit and `git show <rev>:<path>` with the bytes given per revision.
func stubGitRevisions(t *testing.T, unitPath string, revs map[string]map[string]any) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{}
	for rev, doc := range revs {
		data, err := bson.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(dir, strings.NewReplacer("~", "_", "/", "_").Replace(rev)+".bson")
		if err := os.WriteFile(f, data, 0o644); err != nil {
			t.Fatal(err)
		}
		files[rev+":"+unitPath] = f
	}
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })
	execCommand = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "diff" {
			return exec.Command("printf", "M\t%s\n", unitPath)
		}
		if len(args) == 2 && args[0] == "show" {
			if f, ok := files[args[1]]; ok {
				return exec.Command("cat", f)
			}
		}
		return exec.Command("sh", "-c", "exit 128")
	}
}

// rr1393Context is a connected v2 project holding MyFirstModule, a folder in
// it, and the changed unit in that folder.
func rr1393Context() (*ExecContext, *bytes.Buffer) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		VersionFunc:     func() types.MPRVersion { return 2 },
		ContentsDirFunc: func() string { return "mprcontents" },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: rr1393Module}, Name: "MyFirstModule"}}, nil
		},
		ListFoldersFunc: func() ([]*types.FolderInfo, error) {
			return []*types.FolderInfo{{ID: rr1393Folder, ContainerID: rr1393Module, Name: "Notes"}}, nil
		},
		ListUnitsFunc: func() ([]*types.UnitInfo, error) {
			return []*types.UnitInfo{{ID: rr1393Unit, ContainerID: rr1393Folder}}, nil
		},
		// Like the real backend: each parser accepts only its own $Type.
		ParseMicroflowFromRawFunc: func(raw map[string]any, unitID, containerID model.ID) *microflows.Microflow {
			if raw["$Type"] != "Microflows$Microflow" {
				return nil
			}
			mf := &microflows.Microflow{Name: extractString(raw["Name"]), Documentation: extractString(raw["Documentation"])}
			mf.ID = unitID
			return mf
		},
		ParseNanoflowFromRawFunc: func(raw map[string]any, unitID, containerID model.ID) *microflows.Nanoflow {
			if raw["$Type"] != "Microflows$Nanoflow" {
				return nil
			}
			nf := &microflows.Nanoflow{Name: extractString(raw["Name"]), Documentation: extractString(raw["Documentation"])}
			nf.ID = unitID
			return nf
		},
	}
	out := &bytes.Buffer{}
	return &ExecContext{Context: context.Background(), Backend: mb, Output: out, Cache: &executorCache{}}, out
}

func TestDiffLocal_RefRange_HeaderNamesUnitAndRevisions(t *testing.T) {
	unitPath := "mprcontents/56/59/" + rr1393Unit + ".mxunit"
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": {"$Type": "Microflows$Microflow", "Name": "ACT_NoteFieldRun", "Documentation": "before"},
		"HEAD":   {"$Type": "Microflows$Microflow", "Name": "ACT_NoteFieldRun", "Documentation": "after"},
	})
	ctx, out := rr1393Context()

	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	for _, want := range []string{
		"--- Microflow.MyFirstModule.ACT_NoteFieldRun (HEAD~1)",
		"+++ Microflow.MyFirstModule.ACT_NoteFieldRun (HEAD)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing header %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{rr1393Unit, "(current)", "(script)"} {
		if strings.Contains(got, bad) {
			t.Errorf("output still contains %q:\n%s", bad, got)
		}
	}
	if !strings.Contains(got, "microflow MyFirstModule.ACT_NoteFieldRun") {
		t.Errorf("rendered MDL dropped the module:\n%s", got)
	}
}

func rr1393Page(box map[string]any) map[string]any {
	return map[string]any{
		"$Type": "Forms$Page",
		"Name":  "Home_Web",
		"FormCall": map[string]any{
			"$Type": "Forms$LayoutCall",
			"Form":  "Atlas_Core.Atlas_Default",
			"Arguments": []any{int32(3), map[string]any{
				"$Type":     "Forms$FormCallArgument",
				"Parameter": "Atlas_Core.Atlas_Default.Main",
				"Widgets":   []any{int32(3), box},
			}},
		},
	}
}

func TestDiffLocal_RefRange_PageWidgetChangeShowsHunk(t *testing.T) {
	unitPath := "mprcontents/56/59/" + rr1393Unit + ".mxunit"
	before := map[string]any{"$Type": "Forms$DivContainer", "Name": "box1", "Widgets": []any{int32(3)}}
	after := map[string]any{"$Type": "Forms$DivContainer", "Name": "box1", "Widgets": []any{int32(3)},
		"ConditionalVisibilitySettings": map[string]any{
			"$Type":      "Forms$ConditionalVisibilitySettings",
			"Expression": "false",
		}}
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": rr1393Page(before),
		"HEAD":   rr1393Page(after),
	})
	ctx, out := rr1393Context()

	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	if !strings.Contains(got, "Summary: 0 new, 1 modified, 0 deleted") {
		t.Fatalf("unexpected summary:\n%s", got)
	}
	if !strings.Contains(got, "@@") {
		t.Fatalf("page counted as modified but no hunk printed:\n%s", got)
	}
	var added []string
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added = append(added, line)
		}
	}
	if len(added) != 1 || !strings.Contains(added[0], "box1") || !strings.Contains(added[0], "Visible") {
		t.Errorf("want one added line for box1 gaining Visible, got %q in:\n%s", added, got)
	}
}

// Nanoflows and layouts had the same placeholder-body defect as pages: a fixed
// "-- (nanoflow body)" / "-- (layout structure)" stub, so a change inside one
// diffed to nothing. Nanoflows were worse: "Microflows$Nanoflow" contains
// "Microflow", so the dispatch sent them to the microflow parser first.
func TestDiffLocal_RefRange_NanoflowChangeShowsHunk(t *testing.T) {
	unitPath := "mprcontents/56/59/" + rr1393Unit + ".mxunit"
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": {"$Type": "Microflows$Nanoflow", "Name": "NAV_OpenNote", "Documentation": "before"},
		"HEAD":   {"$Type": "Microflows$Nanoflow", "Name": "NAV_OpenNote", "Documentation": "after"},
	})
	ctx, out := rr1393Context()

	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	for _, want := range []string{
		"--- Nanoflow.MyFirstModule.NAV_OpenNote (HEAD~1)",
		"nanoflow MyFirstModule.NAV_OpenNote",
		"- * before",
		"+ * after",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "parse failed") {
		t.Errorf("nanoflow went to a parser that rejected it:\n%s", got)
	}
}

func rr1393Layout(box map[string]any) map[string]any {
	return map[string]any{
		"$Type": "Forms$Layout",
		"Name":  "Notes_Layout",
		"Content": map[string]any{
			"$Type":      "Forms$WebLayoutContent",
			"LayoutType": "Responsive",
			"Widgets":    []any{int32(3), box},
		},
	}
}

func TestDiffLocal_RefRange_LayoutWidgetChangeShowsHunk(t *testing.T) {
	unitPath := "mprcontents/56/59/" + rr1393Unit + ".mxunit"
	before := map[string]any{"$Type": "Forms$DivContainer", "Name": "box1", "Widgets": []any{int32(3)}}
	after := map[string]any{"$Type": "Forms$DivContainer", "Name": "box1", "Widgets": []any{int32(3)},
		"ConditionalVisibilitySettings": map[string]any{
			"$Type":      "Forms$ConditionalVisibilitySettings",
			"Expression": "false",
		}}
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": rr1393Layout(before),
		"HEAD":   rr1393Layout(after),
	})
	ctx, out := rr1393Context()

	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	if !strings.Contains(got, "@@") {
		t.Fatalf("layout counted as modified but no hunk printed:\n%s", got)
	}
	if !strings.Contains(got, "create or modify layout MyFirstModule.Notes_Layout") ||
		!strings.Contains(got, "layouttype: 'Responsive'") {
		t.Errorf("layout header not rendered as DESCRIBE LAYOUT does:\n%s", got)
	}
	var added []string
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added = append(added, line)
		}
	}
	if len(added) != 1 || !strings.Contains(added[0], "box1") || !strings.Contains(added[0], "Visible") {
		t.Errorf("want one added line for box1 gaining Visible, got %q in:\n%s", added, got)
	}
}
