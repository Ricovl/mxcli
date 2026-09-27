// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// The generic ALTER resolves every target on every backend (ako/mxcli#712):
// over MCP a widget resolves by name, and what this backend cannot address yet
// is refused explicitly instead of surfacing as some operation's own failure.
func TestPageMutator_ResolveAlterTarget(t *testing.T) {
	m := newTestMutator()

	if got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"t1"}}); err != nil || got.Kind != "widget" || got.Name != "t1" {
		t.Errorf("widget by name: %+v %v", got, err)
	}
	cases := []struct {
		target backend.AlterTarget
		want   string
	}{
		{backend.AlterTarget{Path: []string{"nope"}}, `widget "nope" not found`},
		{backend.AlterTarget{Path: []string{"dg", "Total"}}, "not yet supported by the MCP backend"},
		{backend.AlterTarget{Caption: "Save"}, "by name"},
		{backend.AlterTarget{Path: []string{"t1"}, Ordinal: 2}, "@2"},
	}
	for _, c := range cases {
		_, err := m.ResolveAlterTarget(c.target)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.target, c.want, err)
		}
	}
}
