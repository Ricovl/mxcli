// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// allowlistHeader heads the allowlist file; FormatAllowlist writes it.
const allowlistHeader = `# Canonical-form conformance allowlist (ako/mxcli#756). IT MAY ONLY SHRINK.
#
# One line per source and class: the number of findings the gate tolerates
# there today. <count> TAB <class> TAB <source>. Class is a deprecated
# spelling's registry code (MDL-DEPRnnn) or "syntax" for a block that parses in
# no context. Source is a repository path, or syntax:<topic> for an
# ` + "`mxcli syntax`" + ` entry.
#
# The gate (make check-conformance) fails when a count grows or a source/class
# has no line here, and when a count falls below its line, so that fixing a doc
# lowers the ceiling for good. After fixing docs, lower the list with
#
#     make conformance-shrink
#
# which rewrites this file with the measured counts. It only ever lowers a
# count or deletes a line; never add or raise one by hand — fix the doc, or
# fence a counterexample as ` + "```text" + `.
`

// ParseAllowlist reads an allowlist file.
func ParseAllowlist(r io.Reader) (Tally, error) {
	t := Tally{}
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("allowlist line %d: want <count> TAB <class> TAB <source>, got %q", n, line)
		}
		count, err := strconv.Atoi(f[0])
		if err != nil || count <= 0 {
			return nil, fmt.Errorf("allowlist line %d: count %q is not a positive number", n, f[0])
		}
		k := Key{Source: f[2], Class: f[1]}
		if _, dup := t[k]; dup {
			return nil, fmt.Errorf("allowlist line %d: %s %s is listed twice", n, k.Class, k.Source)
		}
		t[k] = count
	}
	return t, sc.Err()
}

// FormatAllowlist writes an allowlist file, sorted by source and class.
func FormatAllowlist(t Tally) string {
	keys := make([]Key, 0, len(t))
	for k, n := range t {
		if n > 0 {
			keys = append(keys, k)
		}
	}
	SortKeys(keys)
	var b strings.Builder
	b.WriteString(allowlistHeader)
	b.WriteString("\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%d\t%s\t%s\n", t[k], k.Class, k.Source)
	}
	return b.String()
}
