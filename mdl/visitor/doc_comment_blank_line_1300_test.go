// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1300: "A blank line inside a /** */ documentation block is
// dropped for entities, attributes and microflows" — the stored documentation
// collapsed to "First paragraph.\nSecond paragraph.", so a Studio Pro "\n\n"
// came back as "\n" on the next write. Only the blank lines at the edges of the
// block are framing; an interior one is part of the text.

const twoParagraphs = "First paragraph.\n\nSecond paragraph."

const docBlock = `/**
 * First paragraph.
 *
 * Second paragraph.
 */`

func TestDocCommentKeepsInteriorBlankLine_Entity(t *testing.T) {
	stmts := buildOK(t, docBlock+`
create persistent entity M.E (
  `+docBlock+`
  Name: String(100)
);`)
	e, ok := stmts[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("expected CreateEntityStmt, got %T", stmts[0])
	}
	if e.Documentation != twoParagraphs {
		t.Errorf("entity documentation = %q, want %q", e.Documentation, twoParagraphs)
	}
	if len(e.Attributes) != 1 || e.Attributes[0].Documentation != twoParagraphs {
		t.Errorf("attribute documentation = %q, want %q", e.Attributes[0].Documentation, twoParagraphs)
	}
}

func TestDocCommentKeepsInteriorBlankLine_Microflow(t *testing.T) {
	stmts := buildOK(t, docBlock+`
create microflow M.MF ()
begin
end;`)
	mf, ok := stmts[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("expected CreateMicroflowStmt, got %T", stmts[0])
	}
	if mf.Documentation != twoParagraphs {
		t.Errorf("microflow documentation = %q, want %q", mf.Documentation, twoParagraphs)
	}
}

// The framing lines (`/**` and ` */` on their own lines) and the leading `*`
// are still stripped, and a single-line block is unchanged.
func TestExtractDocCommentFraming(t *testing.T) {
	cases := map[string]string{
		"/** One line. */":                            "One line.",
		"/**\n * A\n * B\n */":                        "A\nB",
		"/**\n *\n * A\n *\n */":                      "A",
		"/**\n * A\n *\n *\n * B\n */":                "A\n\n\nB",
		"/**\n * " + "A" + "\n * \n * B\n */":         "A\n\nB", // describe's " * " form
		"/**\n   First paragraph.\n\n   Second.\n */": "First paragraph.\n\nSecond.",
	}
	for in, want := range cases {
		if got := extractDocComment(in); got != want {
			t.Errorf("extractDocComment(%q) = %q, want %q", in, got, want)
		}
	}
}
