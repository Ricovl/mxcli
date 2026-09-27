// SPDX-License-Identifier: Apache-2.0

// Package suggest finds the name a misspelling most likely meant, for the
// "did you mean" part of an error about an unknown name.
package suggest

import "strings"

// Closest returns the known name a misspelling most likely meant,
// or "" when nothing is close enough to be worth guessing. Case-insensitive
// prefix/substring first, then a single edit, then — for a name of five
// letters or more — the nearest within two edits, counting a swap of two
// neighbouring letters as one (`Verison`, `Passwd`).
func Closest(name string, known []string) string {
	lower := strings.ToLower(name)
	for _, k := range known {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, lower) || strings.HasPrefix(lower, lk) || strings.Contains(lk, lower) {
			return k
		}
	}
	for _, k := range known {
		if WithinOneEdit(lower, strings.ToLower(k)) {
			return k
		}
	}
	if len(lower) < 5 {
		return ""
	}
	best, bestDist := "", 3
	for _, k := range known {
		if d := editDistance(lower, strings.ToLower(k)); d < bestDist {
			best, bestDist = k, d
		}
	}
	return best
}

// editDistance is the optimal string alignment distance: insertions,
// deletions, substitutions and swaps of two neighbouring bytes.
func editDistance(a, b string) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}

// WithinOneEdit reports whether a and b differ by at most one insertion,
// deletion or substitution.
func WithinOneEdit(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b)-len(a) > 1 {
		return false
	}
	i, j, edits := 0, 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		edits++
		if edits > 1 {
			return false
		}
		if len(a) == len(b) {
			i++
		}
		j++
	}
	return true
}
