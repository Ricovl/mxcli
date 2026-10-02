// SPDX-License-Identifier: Apache-2.0

// Upgrading a .test.mdl / .test.md file: `mxcli fmt --upgrade` on a test file
// (ako/mxcli#837).
//
// A test file is not top-level MDL, so mdl/upgrade cannot parse it as written —
// the `/** @test … */` doc comments and the `/` separators are the test
// format's, and each block is a microflow body. It upgrades the rendering
// CheckSource makes instead, which is the file check already reads: every
// body on its own lines and columns, wrapped in a microflow whose fragments sit
// on the lines the doc comments and separators occupied.
//
// Because a body line is rendered verbatim, an upgraded body line is taken
// back into the file in its place, and every other line — doc comments,
// `--` comments, separators, markdown prose — is the author's, byte for byte.
// A rewrite that touched a wrapper fragment, or changed the number of lines,
// could not be mapped back, and is refused rather than approximated. The
// language header is the one addition: it is written where the test format
// reads it (ako/mxcli#847).
package testrunner

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
)

// UpgradeSource upgrades a test file's blocks with mdl/upgrade and returns the
// file with only those rewrites applied.
//
// opts.AddHeader is honoured as for any script (ako/mxcli#847): the rendering
// is upgraded with the header-gated rewrites that keep each body's meaning
// under the new version, and the header is then written where the test format
// reads it — the top of a .test.mdl file, and the first line of each
// ```mdl-test block of a markdown one, which is a script of its own. A file
// that already has a header keeps it, as a script does.
func UpgradeSource(content, path string, opts upgrade.Options) (upgrade.Result, error) {
	checked, err := CheckSource(content, path)
	if err != nil {
		return upgrade.Result{Source: content}, err
	}
	res, err := upgrade.Upgrade(checked.MDL, opts)
	if err != nil {
		res.Source = content
		return res, err
	}

	upgradedText := res.Source
	header := ""
	if res.HeaderAdded {
		// Upgrade writes the header as a line of its own above the script;
		// it is put back where the test format reads it, below.
		nl := strings.IndexByte(upgradedText, '\n')
		if nl < 0 || !langver.IsHeaderLine(upgradedText[:nl]) {
			res.Source = content
			return res, fmt.Errorf("the upgrade did not put the language header on a line of its own; upgrade this file by hand")
		}
		header, upgradedText = upgradedText[:nl], upgradedText[nl+1:]
	}

	orig := strings.Split(content, "\n")
	rendered := strings.Split(checked.MDL, "\n")
	upgraded := strings.Split(upgradedText, "\n")
	out, err := mapUpgradeBack(orig, rendered, upgraded)
	if err != nil {
		res.Source = content
		return res, err
	}
	if header != "" {
		out = insertTestHeader(out, header, strings.EqualFold(filepath.Ext(path), ".md"))
	}
	res.Source = strings.Join(out, "\n")
	return res, nil
}

// mapUpgradeBack takes the upgraded rendering back onto the file.
//
// A body line is rendered verbatim (rendered[i] == orig[i]); every other line
// — a doc comment, a separator, a comment, markdown prose — is blank or a
// wrapper fragment. The two renderings are aligned line by line (a diff, not
// an index walk): a header-gated rewrite may add lines, as the one for `\n`
// in a string does by writing the line break itself. Every changed region
// must consist of body lines; a rewrite that reached a wrapper fragment, or
// any line that is not the author's statement, cannot be mapped back and is
// refused rather than approximated.
func mapUpgradeBack(orig, rendered, upgraded []string) ([]string, error) {
	isBody := func(i int) bool { return i >= 0 && i < len(orig) && rendered[i] == orig[i] }
	m := difflib.NewMatcherWithJunk(rendered, upgraded, false, nil)
	out := make([]string, 0, len(orig))
	for _, op := range m.GetOpCodes() {
		if op.Tag == 'e' {
			// Unchanged: the author's line, whatever the rendering made of
			// it. The rendering's spare last line has no counterpart.
			for i := op.I1; i < op.I2 && i < len(orig); i++ {
				out = append(out, orig[i])
			}
			continue
		}
		for i := op.I1; i < op.I2; i++ {
			if i >= len(orig) {
				return nil, fmt.Errorf("an upgrade rewrite reached the end of the last test block, " +
					"which cannot be mapped back onto the file; upgrade that block by hand")
			}
			if !isBody(i) {
				return nil, fmt.Errorf("line %d: an upgrade rewrite reached outside a test block's "+
					"statements, which cannot be mapped back onto the file; upgrade that block by hand", i+1)
			}
		}
		if op.I1 == op.I2 && !isBody(op.I1-1) && !isBody(op.I1) {
			return nil, fmt.Errorf("line %d: an upgrade rewrite inserted lines outside a test block's "+
				"statements, which cannot be mapped back onto the file; upgrade that block by hand", op.I1+1)
		}
		out = append(out, upgraded[op.J1:op.J2]...)
	}
	return out, nil
}

// insertTestHeader writes the language header where the test parser reads it:
// the first line of a .test.mdl file, or the first line inside each
// ```mdl-test block of a markdown file (found as parseMarkdownTests finds
// them).
func insertTestHeader(lines []string, header string, markdown bool) []string {
	// A CRLF file gets a CRLF header line: lines were split on "\n" alone.
	if len(lines) > 0 && strings.HasSuffix(lines[0], "\r") {
		header += "\r"
	}
	if !markdown {
		return append([]string{header}, lines...)
	}
	out := make([]string, 0, len(lines)+4)
	inBlock := false
	for _, line := range lines {
		out = append(out, line)
		trimmed := strings.TrimSpace(line)
		switch {
		case !inBlock && strings.HasPrefix(trimmed, "```mdl-test"):
			inBlock = true
			out = append(out, header)
		case inBlock && trimmed == "```":
			inBlock = false
		}
	}
	return out
}
