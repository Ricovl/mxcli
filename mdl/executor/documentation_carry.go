// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// carriedDocumentation returns the documentation a rewrite should store.
//
// A statement that carried a `/** … */` comment — even an empty one — states its
// intent and wins. A statement that carried none said nothing about
// documentation, so the stored value is preserved.
//
// Before this, every rewrite path wrote the statement's value unconditionally,
// so a statement with no doc comment overwrote stored prose with "" and reported
// success (mendixlabs/mxcli#1018). It is an empty overwrite, not a drop, which is
// why nothing downstream could see it: the resulting document is valid, and
// `mx check` has no code for "this used to say something".
//
// Preserve-always was not an option. `SET DOCUMENTATION` / `SET COMMENT` exist
// only in the domain-model grammar — there is no ALTER for a microflow, a queue
// or a workflow — so an explicitly empty comment is the only clearing spelling
// most doctypes have.
//
// A stated comment that is exactly the stored text's doc-comment normal form
// (see docCommentNormalForm) says the same thing in the only spelling MDL has,
// so the stored bytes are kept: overwriting them rewrote the document when
// describe output was run unchanged (#731).
func carriedDocumentation(set bool, stated, stored string) string {
	if set {
		if stated != stored && stated == docCommentNormalForm(stored) {
			return stored
		}
		return stated
	}
	return stored
}

// docCommentNormalForm is what a `/** … */` comment can carry of text: the
// visitor trims every line and drops the blank lines at the edges
// (extractDocComment), so trailing whitespace, CRLF line ends and leading or
// trailing blank lines have no spelling. An interior blank line does — it is a
// paragraph break (mendixlabs/mxcli#1300) — so it is kept here too; dropping it
// would make describe output of two-paragraph prose compare unequal to itself.
func docCommentNormalForm(s string) string {
	var kept []string
	for line := range strings.SplitSeq(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		kept = append(kept, strings.TrimSpace(line))
	}
	for len(kept) > 0 && kept[0] == "" {
		kept = kept[1:]
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, "\n")
}
