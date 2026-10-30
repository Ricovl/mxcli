// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

func headerLines(hs []types.DocumentHeader) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = fmt.Sprintf("%+v", h)
	}
	sort.Strings(out)
	return out
}

// The page and snippet headers are what decoding every page and snippet gives.
func TestPageAndSnippetHeaders_MatchTheListings(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	pgs, err := b.ListPages()
	if err != nil {
		t.Fatal(err)
	}
	var want []types.DocumentHeader
	for _, p := range pgs {
		want = append(want, types.DocumentHeader{ID: p.ID, ContainerID: p.ContainerID, Name: p.Name, Excluded: p.Excluded})
	}
	got, err := b.ListPageHeaders()
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 || fmt.Sprint(headerLines(got)) != fmt.Sprint(headerLines(want)) {
		t.Errorf("page headers (%d) differ from the listing (%d)", len(got), len(want))
	}

	sns, err := b.ListSnippets()
	if err != nil {
		t.Fatal(err)
	}
	want = nil
	for _, s := range sns {
		want = append(want, types.DocumentHeader{ID: s.ID, ContainerID: s.ContainerID, Name: s.Name, Excluded: s.Excluded})
	}
	if got, err = b.ListSnippetHeaders(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(headerLines(got)) != fmt.Sprint(headerLines(want)) {
		t.Errorf("snippet headers (%d) differ from the listing (%d)", len(got), len(want))
	}
	t.Logf("%d pages, %d snippets", len(pgs), len(sns))
}
