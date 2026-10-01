// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func encodeUTF16LE(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

// mendixlabs/mxcli#1253: a script written by Windows PowerShell 5.1 — a UTF-8
// BOM from `Set-Content -Encoding UTF8`, UTF-16LE from `>` — failed check and
// exec with "token recognition error at: 'U+FEFF'". Every script reader the
// CLI has must hand the parser the decoded text. The plain-UTF-8 file is the
// control: it must parse under the same readers, or a pass proves nothing.
func TestScriptReadersDecodeBOMAndUTF16(t *testing.T) {
	const script = "mdl 1;\r\ncreate module BomTest;\r\n"
	dir := t.TempDir()
	files := map[string][]byte{
		"plain.mdl": []byte(script),
		"bom.mdl":   append([]byte{0xEF, 0xBB, 0xBF}, script...),
		"utf16.mdl": encodeUTF16LE(script),
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}

		// check / exec / fmt / diff
		src, err := readMDLSource(p)
		if err != nil {
			t.Fatalf("%s: readMDLSource: %v", name, err)
		}
		if _, errs := visitor.Build(string(src)); len(errs) > 0 {
			t.Errorf("%s: readMDLSource text does not parse: %v", name, errs)
		}
		if !strings.HasPrefix(string(src), "mdl 1;") {
			t.Errorf("%s: the header is not the first thing in the text: %q", name, src)
		}

		// the multi-file check's set-level pass
		set := parseScriptSet([]string{p})
		if len(set) != 1 || set[0].Prog == nil {
			t.Errorf("%s: parseScriptSet did not parse the file", name)
		}
	}
}

// The test runner reads .test.mdl files on its own path.
func TestTestRunnerDecodesBOM(t *testing.T) {
	const body = "/**\n * @test bom\n */\nbegin\n  return true;\nend;\n"
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.test.mdl")
	bom := filepath.Join(dir, "bom.test.mdl")
	_ = os.WriteFile(plain, []byte(body), 0o644)
	_ = os.WriteFile(bom, append([]byte{0xEF, 0xBB, 0xBF}, body...), 0o644)
	want, err := testrunner.ParseTestFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := testrunner.ParseTestFile(bom)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tests) != len(want.Tests) || len(want.Tests) == 0 {
		t.Fatalf("bom file: %d tests, plain control: %d", len(got.Tests), len(want.Tests))
	}
	if got.Tests[0].MDL != want.Tests[0].MDL {
		t.Errorf("bom file test body %q, plain %q", got.Tests[0].MDL, want.Tests[0].MDL)
	}
}
