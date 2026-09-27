// SPDX-License-Identifier: Apache-2.0

// Package suggest finds the name a misspelling most likely meant, for the
// "did you mean" part of an error about an unknown name.
package suggest

import "strings"

// Closest returns the known name a misspelling most likely meant,
// or "" when nothing is close enough to be worth guessing. Case-insensitive
// prefix/substring first, then a single edit.
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
	return ""
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
