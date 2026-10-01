// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/conformance"
)

// guidanceFiles are the generated files an agent reads as its instructions.
// Each must tell it that scripts are mdl 1 and how to bring a headerless one
// over before editing it (decision 4 on ako/mxcli#714). Config files (JSON,
// YAML, TOML) only point at these.
var guidanceFiles = map[string]bool{
	"CLAUDE.md":                       true,
	"AGENTS.md":                       true,
	".cursorrules":                    true,
	".windsurfrules":                  true,
	".vibe/prompts/mendix-mdl.md":     true,
	".github/copilot-instructions.md": true,
}

func generatedFiles() map[string]string {
	out := map[string]string{}
	for _, tool := range SupportedTools {
		for _, f := range tool.Files {
			out[f.Path] = f.Content("Demo", "Demo.mpr")
		}
	}
	for _, f := range UniversalFiles {
		out[f.Path] = f.Content("Demo", "Demo.mpr")
	}
	return out
}

func TestInitTemplatesTeachMDL1(t *testing.T) {
	files := generatedFiles()
	for path := range guidanceFiles {
		if _, ok := files[path]; !ok {
			t.Errorf("%s is no longer generated; update guidanceFiles", path)
		}
	}
	for path, content := range files {
		// Every MDL block a template shows is valid mdl 1, and a script
		// carries the header.
		for _, u := range conformance.MarkdownUnits(path, content) {
			for _, f := range conformance.CheckMDL1(u) {
				t.Errorf("%s", f)
			}
		}
		if !guidanceFiles[path] {
			continue
		}
		for _, want := range []string{"mdl 1;", "fmt --upgrade --header", "unchanged"} {
			if !strings.Contains(content, want) {
				t.Errorf("generated %s does not say %q: an agent reading it writes headerless (mdl 0) "+
					"scripts, or mixes dialects in a file it edits", path, want)
			}
		}
		// The `/` separator is refused under mdl 1.
		if strings.Contains(content, "`/` on a line by itself") {
			t.Errorf("generated %s still teaches the `/` terminator, which mdl 1 refuses", path)
		}
	}
}
