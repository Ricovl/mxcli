package docker

import (
	"os"
	"path/filepath"
	"testing"
)

// Mendix 11.15's mxbuild ships tools/node/<platform>/node but no
// rollup-runner.mjs (it bundles with rspack only). The bundled node must still
// be found when none is on PATH; requiring the rollup runner made
// `mxcli playwright check` fail with "node not found" on a machine whose newest
// cached mxbuild is 11.15.
func TestResolveNodeForScript_RspackOnlyMxBuild(t *testing.T) {
	t.Setenv("PATH", "")
	modeler := t.TempDir()
	toolsNode := filepath.Join(modeler, "tools", "node")
	node := filepath.Join(toolsNode, "linux-x64", "node")
	if err := os.MkdirAll(filepath.Dir(node), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(node, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsNode, "rspack-runner.mjs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := resolveNodeForScript(filepath.Join(modeler, "mxbuild"))
	if got == "" {
		t.Fatal("no node found for an mxbuild without rollup-runner.mjs (Mendix 11.15 layout)")
	}
}
