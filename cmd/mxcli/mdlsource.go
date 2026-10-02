// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/mendixlabs/mxcli/mdl/srctext"
)

// stdinPath is the conventional spelling for "read from standard input".
const stdinPath = "-"

// readMDLSource reads an MDL script from a file, or from standard input when the
// path is "-".
//
// A heredoc is the natural way to drive MDL from an agent or a shell script, and
// `-` is how every other Unix tool spells it — without this the dash was taken
// literally and the command failed with "open -: no such file or directory",
// forcing a temp file. (mxcli-todo findings #5)
//
// The bytes are decoded by srctext.Decode: a leading UTF-8 byte-order mark is
// dropped and a UTF-16 file is decoded, so a script saved by Windows
// PowerShell 5.1 or Notepad reads the same as one saved without a BOM
// (mendixlabs/mxcli#1253).
func readMDLSource(path string) ([]byte, error) {
	var content []byte
	var err error
	if path == stdinPath {
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading MDL from stdin: %w", err)
		}
	} else if content, err = os.ReadFile(path); err != nil {
		return nil, err
	}
	decoded, err := srctext.Decode(content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mdlSourceLabel(path), err)
	}
	return decoded, nil
}

// mdlSourceLabel names the source in messages: a real path, or "<stdin>".
func mdlSourceLabel(path string) string {
	if path == stdinPath {
		return "<stdin>"
	}
	return path
}
