// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1280: the writer must store the enumeration's spelling
// whatever case the AST carries — "error" is not a ShowMessageType value and
// the runtime renders it as an Information dialog.
func TestShowMessageActionStoresCanonicalType(t *testing.T) {
	for word, want := range map[string]microflows.MessageType{
		"error":   microflows.MessageTypeError,
		"warning": microflows.MessageTypeWarning,
		"":        microflows.MessageTypeInformation,
		"Error":   microflows.MessageTypeError,
	} {
		fb := &flowBuilder{measurer: &layoutMeasurer{}}
		fb.addShowMessageAction(&ast.ShowMessageStmt{
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"},
			Type:    word,
		})
		activity := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
		if got := activity.Action.(*microflows.ShowMessageAction).Type; got != want {
			t.Errorf("type %q written as %q, want %q", word, got, want)
		}
	}
}

// A type Mendix does not have (the reporter's `ok`) cannot be stored at all —
// the runtime would show it as Information — so check refuses it rather than
// letting exec write a model mx check passes.
func TestShowMessageUnknownTypeIsRefused(t *testing.T) {
	violations := func(typ string) []string {
		var out []string
		for _, v := range ValidateMicroflow(mfWith(&ast.ShowMessageStmt{
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"},
			Type:    typ,
		})) {
			if v.RuleID == showMessageTypeRule {
				out = append(out, v.Message)
			}
		}
		return out
	}
	got := violations("ok")
	if len(got) != 1 || !strings.Contains(got[0], "ok") || !strings.Contains(got[0], "Information") {
		t.Fatalf("type ok: got %q, want one violation naming it and the valid values", got)
	}
	for _, typ := range []string{"Error", "Warning", "Information", "error", ""} {
		if got := violations(typ); len(got) != 0 {
			t.Errorf("type %q: unexpected violation %q", typ, got)
		}
	}
}

// A nanoflow's show message stores the same enumeration.
func TestShowMessageUnknownTypeIsRefusedInNanoflow(t *testing.T) {
	var got []string
	for _, v := range ValidateNanoflow(&ast.CreateNanoflowStmt{
		Name: ast.QualifiedName{Module: "Probe", Name: "N"},
		Body: []ast.MicroflowStatement{&ast.ShowMessageStmt{
			Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"},
			Type:    "ok",
		}},
	}) {
		if v.RuleID == showMessageTypeRule {
			got = append(got, v.Message)
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %q, want one %s violation", got, showMessageTypeRule)
	}
}
