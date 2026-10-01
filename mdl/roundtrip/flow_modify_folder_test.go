// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// ako/mxcli#887 (rehearsal 2, N1): `create or modify` of a stored flow with no
// folder clause moved the flow to its module's root. "No folder clause" means
// "leave it where it is" (document_placement.go) and the rebuild path honoured
// that, but the splice path compared the declared folder with the stored one
// and moved on any difference — so an empty clause meant "the module root".
// Combined with a separate organise step (`move … to folder`) the flows moved
// back and forth on every run: 19 flows a run in mxcli-rest, 47–55 in formula1.
//
// The subjects are Studio Pro-authored PedApp flows that live in folders. The
// statement is their own description with the folder clause taken out.
//
// Named TestSpliceRerun_ so it runs in the parity suite, not the roundtrip
// suite that is near its time limit (#870).
func TestSpliceRerun_NoFolderClauseKeepsTheFlowFiled(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	subjects := []struct{ kind, name, folder string }{
		{"microflow", "Administration.ChangeMyPassword", "User Management/User"},
		{"nanoflow", "Atlas_Web_Content.DS_LoginContext", "Nanoflows"},
	}
	folderClause := regexp.MustCompile(`(?m)^folder '[^']*'\n`)
	for _, header := range []string{"mdl 1;\n", ""} {
		for _, s := range subjects {
			label := s.kind + " " + s.name + " (header " + strings.TrimSpace(header) + ")"
			target := s.kind + " " + s.name
			h.restore()
			described := h.describeUnder(header, target)
			if !strings.Contains(described, "folder '"+s.folder+"'") {
				t.Fatalf("%s: fixture: want it filed in %q:\n%s", label, s.folder, described)
			}
			stmt := folderClause.ReplaceAllString(described, "")
			if stmt == described {
				t.Fatalf("%s: no folder clause taken out", label)
			}

			// The statement as described, without its folder clause: the flow
			// stays in its folder, and a second run writes nothing.
			if err := h.exec(header + stmt); err != nil {
				t.Fatalf("%s: exec 1: %v\n%s", label, err, h.out.String())
			}
			if strings.Contains(h.out.String(), "Moved ") {
				t.Errorf("%s: exec 1 reported a move:\n%s", label, h.out.String())
			}
			if got := h.describeUnder(header, target); !strings.Contains(got, "folder '"+s.folder+"'") {
				t.Errorf("%s: a statement with no folder clause moved the flow out of %q:\n%s", label, s.folder, got)
			}
			first := h.snapshot()
			if err := h.exec(header + stmt); err != nil {
				t.Fatalf("%s: exec 2: %v\n%s", label, err, h.out.String())
			}
			if changed := first.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("%s: exec 2 wrote %d unit(s):\n  %s", label, len(changed), strings.Join(changed, "\n  "))
			}

			// The rehearsal's shape: the statement, then an organise step that
			// files it. Once settled, a re-run writes nothing.
			organised := header + stmt + "move " + target + " to folder '" + s.folder + "';\n"
			if err := h.exec(organised); err != nil {
				t.Fatalf("%s: organised run: %v\n%s", label, err, h.out.String())
			}
			// The ping-pong ends where it started, so the snapshot cannot see
			// it: each run moved the flow out and back in. The .mpr, which
			// holds the containment rows, was rewritten all the same.
			settled, mpr := h.snapshot(), h.mprBytes()
			if err := h.exec(organised); err != nil {
				t.Fatalf("%s: organised re-run: %v\n%s", label, err, h.out.String())
			}
			if changed := settled.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("%s: the organised re-run wrote %d unit(s):\n  %s", label, len(changed), strings.Join(changed, "\n  "))
			}
			if !bytes.Equal(h.mprBytes(), mpr) {
				t.Errorf("%s: the organised re-run rewrote the .mpr:\n%s", label, h.out.String())
			}

			// diff agrees with exec: nothing moves.
			if out := h.diff(header + stmt); strings.Contains(out, "module root") {
				t.Errorf("%s: diff reports a move to the module root:\n%s", label, out)
			}

			// Control: a folder clause that names another folder still moves it.
			moved := strings.Replace(described, "folder '"+s.folder+"'", "folder 'Elsewhere'", 1)
			if err := h.exec(header + moved); err != nil {
				t.Fatalf("%s: exec with another folder: %v\n%s", label, err, h.out.String())
			}
			if got := h.describeUnder(header, target); !strings.Contains(got, "folder 'Elsewhere'") {
				t.Errorf("%s: a folder clause naming another folder did not move the flow:\n%s", label, got)
			}
		}
	}
}

// mprBytes is the project file as it is on disk.
func (h *harness) mprBytes() []byte {
	h.t.Helper()
	b, err := os.ReadFile(h.mpr)
	if err != nil {
		h.t.Fatalf("read %s: %v", h.mpr, err)
	}
	return b
}
