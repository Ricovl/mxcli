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
// back into the file line for line, and every other line — doc comments,
// `--` comments, separators, markdown prose — is the author's, byte for byte.
// A rewrite that touched a wrapper fragment, or changed the number of lines,
// could not be mapped back, and is refused rather than approximated.
package testrunner

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/upgrade"
)

// UpgradeSource upgrades a test file's blocks with mdl/upgrade and returns the
// file with only those rewrites applied.
//
// A test file takes no language header: check and the runner read its blocks
// as mdl 0, and nothing in either reads a `mdl 1;` line (the runner even
// drops the first test behind one). So opts.AddHeader is not honoured here —
// adding the header, and the header-gated rewrites that go with it, would
// change what the tests mean — and headerSkipped reports that it was asked for.
// Honouring a header in test files is ako/mxcli#847.
func UpgradeSource(content, path string, opts upgrade.Options) (res upgrade.Result, headerSkipped bool, err error) {
	headerSkipped = opts.AddHeader
	opts.AddHeader = false

	checked, err := CheckSource(content, path)
	if err != nil {
		return upgrade.Result{Source: content}, headerSkipped, err
	}
	res, err = upgrade.Upgrade(checked.MDL, opts)
	if err != nil {
		res.Source = content
		return res, headerSkipped, err
	}

	orig := strings.Split(content, "\n")
	rendered := strings.Split(checked.MDL, "\n")
	upgraded := strings.Split(res.Source, "\n")
	if len(upgraded) != len(rendered) {
		res.Source = content
		return res, headerSkipped, fmt.Errorf("an upgrade rewrite changed the number of lines in a test block "+
			"(%d to %d), which cannot be mapped back onto the file; upgrade that block by hand",
			len(rendered), len(upgraded))
	}
	out := make([]string, len(orig))
	for i := range orig {
		switch {
		case rendered[i] == orig[i]:
			// A body line (or a line both leave blank): take the upgrade's.
			out[i] = upgraded[i]
		case upgraded[i] == rendered[i]:
			// A doc comment, separator or comment line: the author's.
			out[i] = orig[i]
		default:
			res.Source = content
			return res, headerSkipped, fmt.Errorf("line %d: an upgrade rewrite reached outside a test block's "+
				"statements, which cannot be mapped back onto the file; upgrade that block by hand", i+1)
		}
	}
	// The rendering's spare last line holds only a wrapper fragment.
	for i := len(orig); i < len(rendered); i++ {
		if upgraded[i] != rendered[i] {
			res.Source = content
			return res, headerSkipped, fmt.Errorf("an upgrade rewrite reached the end of the last test block, " +
				"which cannot be mapped back onto the file; upgrade that block by hand")
		}
	}
	res.Source = strings.Join(out, "\n")
	return res, headerSkipped, nil
}
