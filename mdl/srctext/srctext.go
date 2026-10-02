// SPDX-License-Identifier: Apache-2.0

// Package srctext turns the bytes of an MDL script file into the UTF-8 text the
// lexer reads.
//
// The lexer reads UTF-8 without a byte-order mark. Windows PowerShell 5.1 —
// the default shell on Windows 10/11 — writes neither by default:
// `Set-Content -Encoding UTF8` writes a UTF-8 BOM and `'…' > file.mdl` writes
// UTF-16LE. Both failed with a token-recognition error at an invisible
// character (mendixlabs/mxcli#1253). A BOM carries no content, so stripping it
// and decoding UTF-16 changes no meaning. Every reader of a script file —
// check, exec, fmt, diff, the test runner, `execute script` — goes through
// Decode, so they cannot disagree about what a file says.
package srctext

import (
	"bytes"
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// Decode returns b as UTF-8 text without a leading byte-order mark. A UTF-8 BOM
// is stripped; a UTF-16 (LE or BE) BOM selects UTF-16 decoding. Input without
// a BOM is returned unchanged.
func Decode(b []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(b, bomUTF8):
		return b[len(bomUTF8):], nil
	case bytes.HasPrefix(b, bomUTF16LE):
		return decodeUTF16(b[2:], false)
	case bytes.HasPrefix(b, bomUTF16BE):
		return decodeUTF16(b[2:], true)
	}
	return b, nil
}

// DecodeString is Decode for a reader that wants a string.
func DecodeString(b []byte) (string, error) {
	out, err := Decode(b)
	return string(out), err
}

func decodeUTF16(b []byte, bigEndian bool) ([]byte, error) {
	if len(b)%2 != 0 {
		return nil, fmt.Errorf("file starts with a UTF-16 byte-order mark but has an odd number of bytes; save it as UTF-8")
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		if bigEndian {
			units[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			units[i] = uint16(b[2*i+1])<<8 | uint16(b[2*i])
		}
	}
	runes := utf16.Decode(units)
	out := make([]byte, 0, len(runes))
	for _, r := range runes {
		out = utf8.AppendRune(out, r)
	}
	return out, nil
}
