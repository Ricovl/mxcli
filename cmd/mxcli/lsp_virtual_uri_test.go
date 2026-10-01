// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// mendixlabs/mxcli#1245: the VS Code extension opens describe previews as
// `mendix-mdl:` virtual documents, and uri.URI.Filename panics on any scheme
// but file — "panic: only file URIs are supported, got mendix-mdl". The server
// exited on every didOpen until VS Code gave up restarting it. A virtual
// document is diagnosed in memory like any other; the file control proves the
// same text yields the same diagnostics, so the virtual case is not passing by
// being skipped.
func TestLSPVirtualDocumentDoesNotPanic(t *testing.T) {
	const virtual = uri.URI("mendix-mdl:/MyModule/Foo.mdl")
	s := newMDLServer(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a %s document panicked the server: %v", virtual, r)
		}
	}()

	got := s.documentDiagnostics(virtual, replaceEntity)
	want := s.documentDiagnostics(lspTestURI, replaceEntity)
	if len(diagsWithCode(got, "MDL-DEPR001")) != 1 || len(got) != len(want) {
		t.Errorf("virtual document diagnostics %+v, file control %+v", got, want)
	}

	if _, err := s.CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI(virtual)},
	}); err != nil {
		t.Errorf("CodeAction on a virtual document: %v", err)
	}

	// A virtual .test.mdl is still recognised as a test file by its name.
	if p := documentPath(uri.URI("mendix-mdl:/MyModule/Foo.test.mdl")); p != "/MyModule/Foo.test.mdl" {
		t.Errorf("documentPath = %q", p)
	}
}
