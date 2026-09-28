// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"strings"
	"testing"
)

// IsMoveDocumentType reads the keyword registry, so every DocumentType MOVE
// maps to must be recognised by its own spelling and by its canonical one.
func TestIsMoveDocumentTypeKnowsEverySpelling(t *testing.T) {
	for _, d := range MoveDocumentTypeByKeyword {
		for _, s := range []string{strings.ToLower(string(d)), d.CanonicalSpelling()} {
			if !IsMoveDocumentType(s) {
				t.Errorf("IsMoveDocumentType(%q) = false", s)
			}
		}
	}
	if IsMoveDocumentType("page template") {
		t.Error("a kind MOVE cannot spell was recognised")
	}
}
