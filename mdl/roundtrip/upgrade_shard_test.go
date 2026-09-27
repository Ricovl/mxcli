// SPDX-License-Identifier: Apache-2.0

package roundtrip

import (
	"fmt"
	"strconv"
	"strings"
)

// shard selects every count-th item starting at index-1: MXCLI_UPGRADE_SHARD
// ("i/n", 1-based) splits the upgrade property test's executions across CI
// jobs (ako/mxcli#757). Round-robin rather than contiguous ranges, because
// the corpus is sorted by path and neighbouring scripts cost alike.
type shard struct{ index, count int }

// parseShard reads "i/n"; empty is the whole set, 1/1.
func parseShard(s string) (shard, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return shard{1, 1}, nil
	}
	a, b, ok := strings.Cut(s, "/")
	i, errI := strconv.Atoi(a)
	n, errN := strconv.Atoi(b)
	if !ok || errI != nil || errN != nil || n < 1 || i < 1 || i > n {
		return shard{}, fmt.Errorf("MXCLI_UPGRADE_SHARD=%q: want i/n with 1 <= i <= n", s)
	}
	return shard{i, n}, nil
}

// has reports whether the k-th item (0-based) belongs to this shard.
func (s shard) has(k int) bool { return k%s.count == s.index-1 }

func (s shard) String() string { return fmt.Sprintf("%d/%d", s.index, s.count) }
