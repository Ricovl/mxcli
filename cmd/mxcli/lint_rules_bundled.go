// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// lint_rules_bundled.go tells a bundled lint rule mxcli may refresh from one a
// project has made its own (mendixlabs/mxcli#1615).
//
// Bundled rules were recognised by file name alone, so `mxcli init` and the
// session-start `init --sync-skills` overwrote every edit to one — including
// example_microflow.star, which ships to be customised — and wrote the stock
// file back beside a renamed copy, which then ran the same rule ID twice.

// shippedLintRulesFile lists every version of every bundled rule mxcli has
// shipped. A file whose content is on it is the stock rule, however old; any
// other content under a bundled name is an edit.
const shippedLintRulesFile = "lint_rules_shipped.txt"

//go:embed lint_rules_shipped.txt
var shippedLintRulesText string

// shippedLintRuleHashes maps a bundled file name to the hashes of the
// versions of it mxcli has shipped.
var shippedLintRuleHashes = sync.OnceValue(func() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(shippedLintRulesText))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if out[fields[1]] == nil {
			out[fields[1]] = map[string]bool{}
		}
		out[fields[1]][fields[0]] = true
	}
	return out
})

// lintRuleHash is the hash the shipped list records: sha256 of the content
// with CRLF read as LF, so a Windows checkout of a stock rule is still stock.
func lintRuleHash(content []byte) string {
	sum := sha256.Sum256(bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n")))
	return hex.EncodeToString(sum[:])
}

var ruleIDRe = regexp.MustCompile(`(?m)^RULE_ID\s*=\s*["']([^"'\n]+)["']`)

// lintRuleID is the ID a rule file reports under: its RULE_ID, or the file
// name without .star when it has none — the same fallback the linter uses.
func lintRuleID(name string, content []byte) string {
	if m := ruleIDRe.FindSubmatch(content); m != nil {
		return string(m[1])
	}
	return strings.TrimSuffix(name, ".star")
}

// bundledLintRuleWrite is what bringing one directory's bundled rules up to
// date did.
type bundledLintRuleWrite struct {
	Written []string // created, or refreshed from a version mxcli shipped
	Kept    []string // edited by the project, and left alone
	// Shadowed are bundled rules not written because another file in the
	// directory already defines their rule ID: "name (ID in other.star)".
	Shadowed []string
}

// writeBundledLintRules writes this binary's bundled rules into dir, which
// must exist. It is the one write path for `mxcli init`, `init --sync-skills`
// and `add-tool`, so the three cannot disagree about what is the project's:
//
//   - a file that is a version mxcli shipped is brought up to date;
//   - a file under a bundled name with any other content is the project's
//     edit, and is kept — delete it to take this binary's version;
//   - a missing bundled rule is written, unless another file already defines
//     its rule ID: that is the edit moved to a new name, and the stock copy
//     beside it would run the same rule a second time. To switch a bundled
//     rule off, set `enabled: false` for its ID in lint-config.yaml.
func writeBundledLintRules(dir string) (bundledLintRuleWrite, error) {
	var res bundledLintRuleWrite
	names, err := bundledLintRuleNames()
	if err != nil {
		return res, err
	}
	shipped := shippedLintRuleHashes()

	var definedBy map[string][]string // rule ID -> files in dir; read on first need
	for _, name := range names {
		want, err := lintRulesFS.ReadFile("lint-rules/" + name)
		if err != nil {
			return res, err
		}
		target := filepath.Join(dir, name)
		have, readErr := os.ReadFile(target)
		switch {
		case readErr == nil:
			h := lintRuleHash(have)
			if h == lintRuleHash(want) {
				continue
			}
			if !shipped[name][h] {
				res.Kept = append(res.Kept, name)
				continue
			}
		case os.IsNotExist(readErr):
			if definedBy == nil {
				if definedBy, err = lintRuleIDsIn(dir); err != nil {
					return res, err
				}
			}
			id := lintRuleID(name, want)
			if others := definedBy[id]; len(others) > 0 {
				res.Shadowed = append(res.Shadowed, fmt.Sprintf("%s (%s in %s)", name, id, strings.Join(others, ", ")))
				continue
			}
		default:
			return res, readErr
		}
		if err := os.WriteFile(target, want, 0o644); err != nil {
			return res, fmt.Errorf("writing %s: %w", target, err)
		}
		res.Written = append(res.Written, name)
	}
	return res, nil
}

// lintRuleIDsIn maps each rule ID defined by a .star file in dir to the files
// that define it.
func lintRuleIDsIn(dir string) (map[string][]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".star") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		id := lintRuleID(e.Name(), data)
		out[id] = append(out[id], e.Name())
	}
	return out, nil
}

// printBundledLintRuleWrite reports an explicit init's or add-tool's write.
func printBundledLintRuleWrite(w io.Writer, res bundledLintRuleWrite) {
	if len(res.Written) > 0 {
		fmt.Fprintf(w, "  Created %d lint rule files in .claude/lint-rules/\n", len(res.Written))
	}
	printKeptLintRules(w, "  ", res.Kept, res.Shadowed)
}

// printKeptLintRules names the bundled rules a write left to the project.
func printKeptLintRules(w io.Writer, indent string, kept, shadowed []string) {
	if len(kept) > 0 {
		fmt.Fprintf(w, "%sKept %d edited bundled lint rule(s) in .claude/lint-rules/ — this mxcli's version was not written; delete the file to take it: %s\n",
			indent, len(kept), abridge(kept))
	}
	if len(shadowed) > 0 {
		fmt.Fprintf(w, "%sDid not add %d bundled lint rule(s) whose rule ID another file in .claude/lint-rules/ already defines: %s\n",
			indent, len(shadowed), strings.Join(shadowed, "; "))
	}
}
