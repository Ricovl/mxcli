// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli-ledger FINDINGS #170 on the MCP path: an authored submit behaviour
// and grid width are sent; unset sends what it sent before.
func TestMapPageWidget_SubmitBehaviourAndGridWidth(t *testing.T) {
	b := &Backend{}
	delay := 500
	tb := &pages.TextBox{AttributePath: "M.E.Name", SubmitBehaviour: "WhileEditing", SubmitOnInputDelay: &delay}
	tb.Name = "tb"
	m, err := b.mapPageWidget(tb)
	if err != nil || m["submitBehaviour"] != "WhileEditing" || m["submitOnInputDelay"] != 500 {
		t.Fatalf("textbox: %+v / %v", m, err)
	}
	ta := &pages.TextArea{AttributePath: "M.E.Name", SubmitBehaviour: "WhileEditing"}
	ta.Name = "ta"
	if m, err = b.mapPageWidget(ta); err != nil || m["submitBehaviour"] != "WhileEditing" {
		t.Fatalf("textarea: %+v / %v", m, err)
	}
	if _, sent := m["submitOnInputDelay"]; sent {
		t.Errorf("textarea: an unauthored delay was sent: %+v", m)
	}
	plain := &pages.TextBox{AttributePath: "M.E.Name"}
	plain.Name = "plain"
	if m, _ = b.mapPageWidget(plain); m["submitBehaviour"] != nil || m["submitOnInputDelay"] != nil {
		t.Errorf("plain textbox gained submit settings: %+v", m)
	}
	grid := &pages.LayoutGrid{Width: "FixedWidth"}
	grid.Name = "lg"
	if m, err = b.mapPageWidget(grid); err != nil || m["width"] != "FixedWidth" {
		t.Fatalf("layout grid: %+v / %v", m, err)
	}
}
