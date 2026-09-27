// SPDX-License-Identifier: Apache-2.0

package roundtrip

import (
	"fmt"
	"testing"
)

// TestParseShard pins the MXCLI_UPGRADE_SHARD spelling CI passes.
func TestParseShard(t *testing.T) {
	for _, tc := range []struct {
		in   string
		i, n int
		bad  bool
	}{
		{in: "", i: 1, n: 1},
		{in: "1/1", i: 1, n: 1},
		{in: "2/3", i: 2, n: 3},
		{in: " 3/3 ", i: 3, n: 3},
		{in: "0/3", bad: true},
		{in: "4/3", bad: true},
		{in: "1/0", bad: true},
		{in: "3", bad: true},
		{in: "a/b", bad: true},
		{in: "1/2/3", bad: true},
	} {
		s, err := parseShard(tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("parseShard(%q) = %+v, want an error", tc.in, s)
			}
			continue
		}
		if err != nil || s.index != tc.i || s.count != tc.n {
			t.Errorf("parseShard(%q) = %+v, %v; want %d/%d", tc.in, s, err, tc.i, tc.n)
		}
	}
}

// TestShardsPartition: CI runs the shards as separate jobs and never the
// unsharded test, so the shards together must execute every script exactly
// once. A script in no shard would silently leave per-PR CI.
func TestShardsPartition(t *testing.T) {
	for n := 1; n <= 7; n++ {
		const items = 101
		seen := make([]int, items)
		for i := 1; i <= n; i++ {
			s, err := parseShard(fmt.Sprintf("%d/%d", i, n))
			if err != nil {
				t.Fatal(err)
			}
			for k := 0; k < items; k++ {
				if s.has(k) {
					seen[k]++
				}
			}
		}
		for k, c := range seen {
			if c != 1 {
				t.Fatalf("n=%d: item %d is in %d shards, want exactly 1", n, k, c)
			}
		}
	}
}
