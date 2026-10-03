// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// listCountingBackend counts the whole-type microflow decodes a statement does.
type listCountingBackend struct {
	backend.FullBackend
	lists *int
}

func (b listCountingBackend) ListMicroflows() ([]*microflows.Microflow, error) {
	*b.lists++
	return b.FullBackend.ListMicroflows()
}

// DESCRIBE MICROFLOW must resolve its flow — and everything the derived-layout
// check rebuilds it against — without decoding every microflow in the project.
// A listing per describe made a sweep over a large app cost seconds per flow.
//
// The flow needs activities: an empty one never reaches the derived-layout
// rebuild, which is where the listing hid (the builder's "existing microflow"
// lookup, and IsRule's scan for a rule-shaped condition).
func TestDescribeMicroflow_RebuildDoesNotListMicroflows(t *testing.T) {
	lists := 0
	var out bytes.Buffer
	exec := New(&out)
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend {
		return listCountingBackend{FullBackend: modelsdkbackend.New(), lists: &lists}
	})
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")

	lists = 0
	for _, name := range []string{"Administration.ChangeMyPassword", "Administration.SaveNewAccount"} {
		out.Reset()
		run(t, exec, "describe microflow "+name+";")
		if !bytes.Contains(out.Bytes(), []byte("microflow "+name)) {
			t.Fatalf("describe printed no microflow:\n%s", out.String())
		}
	}
	if lists != 0 {
		t.Fatalf("describing 2 microflows listed every microflow %d time(s), want 0", lists)
	}
}
