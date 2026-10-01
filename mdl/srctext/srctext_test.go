// SPDX-License-Identifier: Apache-2.0

package srctext

import (
	"testing"
	"unicode/utf16"
)

func utf16Bytes(s string, bigEndian bool) []byte {
	var out []byte
	if bigEndian {
		out = []byte{0xFE, 0xFF}
	} else {
		out = []byte{0xFF, 0xFE}
	}
	for _, u := range utf16.Encode([]rune(s)) {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// TestDecode is mendixlabs/mxcli#1253: Windows PowerShell 5.1 writes a UTF-8
// BOM (Set-Content -Encoding UTF8) or UTF-16LE ('>' redirection), and the
// lexer rejected both with an invisible token-recognition error.
func TestDecode(t *testing.T) {
	const script = "create module BomTest;\r\n-- café \U0001F600\r\n"
	cases := []struct {
		name string
		in   []byte
	}{
		{"plain UTF-8 (control)", []byte(script)},
		{"UTF-8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, script...)},
		{"UTF-16LE BOM", utf16Bytes(script, false)},
		{"UTF-16BE BOM", utf16Bytes(script, true)},
	}
	for _, c := range cases {
		got, err := Decode(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if string(got) != script {
			t.Errorf("%s: got %q, want %q", c.name, got, script)
		}
	}
}

func TestDecodeRejectsTruncatedUTF16(t *testing.T) {
	in := utf16Bytes("create", false)
	if _, err := Decode(in[:len(in)-1]); err == nil {
		t.Error("an odd-length UTF-16 file decoded without error")
	}
}

// A BOM character in the middle of a file is content, not an encoding mark;
// only a leading one is stripped.
func TestDecodeKeepsInteriorBOM(t *testing.T) {
	in := []byte("-- a" + "\xef\xbb\xbf" + "b\n")
	got, err := Decode(in)
	if err != nil || string(got) != string(in) {
		t.Errorf("got %q, %v; want the input unchanged", got, err)
	}
}
