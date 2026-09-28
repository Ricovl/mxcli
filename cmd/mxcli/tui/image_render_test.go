package tui

import (
	"bytes"
	"encoding/base64"
	"os"
	"testing"
)

// Describe writes an image as `Data: '<base64>'` (ako/mxcli#707); the preview
// decodes it into a file of its own, and still takes a script's `File:` path.
func TestExtractImagePaths_DataAndFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	img := []byte("\x89PNG\r\n\x1a\nfake")
	out := "create or modify image collection M.Icons {\n" +
		"  image Logo ( Data: '" + base64.StdEncoding.EncodeToString(img) + "' )\n" +
		"  image Home ( File: 'assets/home.png' )\n" +
		"};\n"
	paths := extractImagePaths(out)
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want 2", paths)
	}
	got, err := os.ReadFile(paths[0])
	if err != nil || !bytes.Equal(got, img) {
		t.Errorf("decoded image %q (%v), want %q", got, err, img)
	}
	if paths[1] != "assets/home.png" {
		t.Errorf("file path = %q", paths[1])
	}
}
