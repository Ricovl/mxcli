// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1562 (mendixlabs/mxcli#1630): diff-local counted a domain model as
// modified after a `grant` on one of its entities and printed no difference.
// The domain model renderer printed each entity's attributes and the
// associations, but not the entity access rules, so a script that only changed
// access had nothing to review. The rules now render with DESCRIBE ENTITY's own
// grant writer, read from a parse of each revision of the unit.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

const diffAccessSetup = `create module M;
create module role M.User;
create persistent entity M.Item (Name: String(100));
grant read * on entity M.Item to M.User;`

func TestDiffLocal_EntityGrantChangeRenders(t *testing.T) {
	got := diffLocalAfter(t, diffAccessSetup, "grant read *, write * on entity M.Item to M.User;", DiffOptions{})

	for _, want := range []string{
		"\n-grant read * on entity M.Item to M.User;\n",
		"\n+grant read *, write * on entity M.Item to M.User;\n",
		"Summary: 0 new, 1 modified, 0 deleted",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Modified: DomainModel") {
		t.Errorf("the grant fell through to the not-rendered note:\n%s", got)
	}
}

func TestDiffLocal_EntityGrantChangeInStructFormat(t *testing.T) {
	got := diffLocalAfter(t, diffAccessSetup, "grant read *, write * on entity M.Item to M.User;",
		DiffOptions{Format: DiffFormatStructural})
	if !strings.Contains(got, "DomainModel: M\n  ~ Line") {
		t.Errorf("struct format shows no changed line:\n%s", got)
	}
	if strings.Contains(got, "(no changes)") || strings.Contains(got, "Modified: DomainModel") {
		t.Errorf("struct format did not render the grant:\n%s", got)
	}
}

// A grant on an entity with no rule yet is a + line, and a revoke of the last
// rule a - line.
func TestDiffLocal_EntityGrantAddedAndRevoked(t *testing.T) {
	setup := "create module M;\ncreate module role M.User;\ncreate persistent entity M.Item (Name: String(100));"
	got := diffLocalAfter(t, setup, "grant read * on entity M.Item to M.User;", DiffOptions{})
	if !strings.Contains(got, "\n+grant read * on entity M.Item to M.User;\n") {
		t.Errorf("a new rule is not a + line:\n%s", got)
	}
	got = diffLocalAfter(t, diffAccessSetup, "revoke all on entity M.Item from M.User;", DiffOptions{})
	if !strings.Contains(got, "\n-grant read * on entity M.Item to M.User;\n") {
		t.Errorf("a revoked rule is not a - line:\n%s", got)
	}
}

// The grant lines are DESCRIBE ENTITY's, not a second renderer's: every grant
// describe prints for the entity appears verbatim on diff-local's new side.
func TestDiffLocal_EntityGrantLinesAreDescribeEntitys(t *testing.T) {
	const change = "grant create, delete, read (Name), write (Name) on entity M.Item to M.User where [Name != empty];"
	got := diffLocalAfter(t, diffAccessSetup, change, DiffOptions{})

	var out bytes.Buffer
	x := New(&out)
	x.SetQuiet(true)
	x.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { x.Close() })
	run(t, x, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	run(t, x, diffAccessSetup)
	run(t, x, change)
	out.Reset()
	run(t, x, "describe entity M.Item;")

	n := 0
	for line := range strings.SplitSeq(out.String(), "\n") {
		if strings.HasPrefix(line, "grant ") {
			n++
			if !strings.Contains(got, "\n+"+line+"\n") && !strings.Contains(got, "\n "+line+"\n") {
				t.Errorf("describe entity prints %q, diff-local does not:\n%s", line, got)
			}
		}
	}
	if n == 0 {
		t.Fatalf("describe entity printed no grant:\n%s", out.String())
	}
}
