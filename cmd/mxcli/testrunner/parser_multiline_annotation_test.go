// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// ako/mxcli#1514 (upstream mendixlabs/mxcli#1604): an @expect written over two
// lines kept only its first. parseAnnotations read the doc comment one line at a
// time and the tag's pattern captured the rest of ITS line, so the continuation
// line — having no tag of its own — matched nothing and was dropped without a
// word. When the first line was a complete expression the test then ran a
// weaker assertion than the one written: `$r = 'a;b'` instead of
// `$r = 'a;b' and length($r) = 3`, which can pass on less.
//
// An annotation's text now runs on, as a javadoc block tag's does, to the next
// tag or the end of the comment, and the whole of it is what gets parsed.

func TestMultiLineExpectIsOneAssertion(t *testing.T) {
	cases := []struct {
		name, doc, raw string
	}{
		{
			name: "or on the next line",
			doc: `/**
 * @test multiline3
 * @expect $r = 'zzz'
 *   or $r = 'qqq'
 */`,
			raw: `$r = 'zzz' or $r = 'qqq'`,
		},
		{
			name: "and on the next line",
			doc: `/**
 * @test multiline
 * @expect $r = 'a;b'
 *   and length($r) = 3
 */`,
			raw: `$r = 'a;b' and length($r) = 3`,
		},
		{
			// The first line alone does not parse, which used to be the one
			// shape that was reported — as "expected a value at column 16".
			name: "open parenthesis on the first line",
			doc: `/**
 * @test multiline2
 * @expect ($r = 'a;b' and
 *   length($r) = 3)
 */`,
			raw: `($r = 'a;b' and length($r) = 3)`,
		},
		{
			name: "three lines, with a blank line inside",
			doc: `/**
 * @test three
 * @expect $r = 'a'
 *
 *   or $r = 'b'
 *   or $r = 'c'
 */`,
			raw: `$r = 'a' or $r = 'b' or $r = 'c'`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := parseAnnotations(c.doc)
			if len(a.AssertionErrors) != 0 {
				t.Fatalf("AssertionErrors = %v, want none", a.AssertionErrors)
			}
			if len(a.Expects) != 1 {
				t.Fatalf("Expects = %+v, want exactly one assertion", a.Expects)
			}
			if got := a.Expects[0].Raw; got != c.raw {
				t.Errorf("Raw = %q, want %q: the continuation line was not part of the assertion", got, c.raw)
			}
			// The condition evaluated is the joined one, not the first line's.
			want, err := ParseExpect(c.raw)
			if err != nil {
				t.Fatalf("ParseExpect(%q): %v", c.raw, err)
			}
			if a.Expects[0].Condition != want.Condition {
				t.Errorf("Condition = %q, want %q", a.Expects[0].Condition, want.Condition)
			}
		})
	}
}

// The continuation ends at the next tag: two @expect lines are still two
// assertions, and a tag after a multi-line one is still read as itself.
func TestMultiLineExpectEndsAtTheNextTag(t *testing.T) {
	a := parseAnnotations(`/**
 * @test boundaries
 * @expect $a = 1
 *   and $b = 2
 * @expect $c = 3
 * @cleanup none
 */`)
	if len(a.AssertionErrors) != 0 {
		t.Fatalf("AssertionErrors = %v", a.AssertionErrors)
	}
	var raws []string
	for _, e := range a.Expects {
		raws = append(raws, e.Raw)
	}
	if strings.Join(raws, " | ") != "$a = 1 and $b = 2 | $c = 3" {
		t.Errorf("Expects = %q, want [$a = 1 and $b = 2, $c = 3]", raws)
	}
	if a.Cleanup != CleanupNone {
		t.Errorf("Cleanup = %q: the tag after the continuation was swallowed", a.Cleanup)
	}
}

// Fail closed: text after an @expect that does not continue the expression is
// not quietly ignored either — ignoring it is how the truncation went unseen.
// It makes the assertion uncompilable, which is an ERROR, and the message says
// the following lines were read as part of the annotation and where prose goes.
func TestTextAfterExpectIsNotSilentlyDropped(t *testing.T) {
	a := parseAnnotations(`/**
 * @test trailing prose
 * @expect $r = 'a;b'
 * Checks that the separator survives.
 */`)
	if len(a.Expects) != 0 {
		t.Errorf("Expects = %+v, want none: a line that is not part of the expression was dropped", a.Expects)
	}
	if len(a.AssertionErrors) != 1 {
		t.Fatalf("AssertionErrors = %v, want one", a.AssertionErrors)
	}
	msg := a.AssertionErrors[0]
	for _, want := range []string{"Checks that the separator survives", "2 lines", "before the first tag"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// @verify has the same shape — an OQL query is the annotation most likely to be
// wrapped — and was truncated the same way.
func TestMultiLineVerifyIsOneQuery(t *testing.T) {
	a := parseAnnotations(`/**
 * @test verify wrapped
 * @cleanup none
 * @verify select count(*) as n
 *   from Probe.Item
 *   where Status = 'Open' = 2
 */`)
	if len(a.AssertionErrors) != 0 {
		t.Fatalf("AssertionErrors = %v", a.AssertionErrors)
	}
	if len(a.Verify) != 1 {
		t.Fatalf("Verify = %+v, want one", a.Verify)
	}
	v := a.Verify[0]
	if v.Query != "select count(*) as n from Probe.Item where Status = 'Open'" || v.Expected != "2" {
		t.Errorf("Verify = {Query: %q, Expected: %q}, want the whole query compared with 2", v.Query, v.Expected)
	}
}

// Controls: prose BEFORE the tags is the description and stays ignored, and so
// do the lines after a tag that takes no expression (@test's name is one line).
func TestDescriptionLinesAreNotAnnotationText(t *testing.T) {
	a := parseAnnotations(`/**
 * Orders over the limit need approval.
 * @test approval needed
 * The limit is configured per customer.
 * @expect $r = true
 */`)
	if a.Test != "approval needed" {
		t.Errorf("Test = %q", a.Test)
	}
	if len(a.AssertionErrors) != 0 {
		t.Errorf("AssertionErrors = %v, want none", a.AssertionErrors)
	}
	if len(a.Expects) != 1 || a.Expects[0].Raw != "$r = true" {
		t.Errorf("Expects = %+v, want [$r = true]", a.Expects)
	}
}

// End to end through the file parser, as `mxcli test --list` reads it: the test
// case carries the joined assertion, so --list, check and the generated
// microflow all see the expression that was written.
func TestParseMDLTestsKeepsAMultiLineExpect(t *testing.T) {
	tests, err := parseMDLTests(`mdl 1;
/**
 * @test multiline
 * @expect $r = 'a;b'
 *   and length($r) = 3
 */
$r = call microflow Probe.SUB_Make();
/
`, "multi.test.mdl")
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 1 || len(tests[0].Expects) != 1 {
		t.Fatalf("tests = %+v", tests)
	}
	if got := tests[0].Expects[0].Raw; got != "$r = 'a;b' and length($r) = 3" {
		t.Errorf("Raw = %q: --list and the run would assert less than the file says", got)
	}
}

// A tag on a line of its own, with its text starting on the next, is the same
// annotation. The old pattern required text on the tag's line, so a bare
// @expect matched nothing and the assertion below it was dropped; one that
// stays bare is an error, like any @expect that cannot be compiled.
func TestBareExpectTagTakesTheNextLines(t *testing.T) {
	a := parseAnnotations(`/**
 * @test bare tag
 * @expect
 *   $r = 'a;b'
 */`)
	if len(a.AssertionErrors) != 0 || len(a.Expects) != 1 || a.Expects[0].Raw != "$r = 'a;b'" {
		t.Errorf("Expects = %+v, AssertionErrors = %v, want [$r = 'a;b']", a.Expects, a.AssertionErrors)
	}

	a = parseAnnotations(`/**
 * @test bare tag, nothing after it
 * @expect
 */`)
	if len(a.Expects) != 0 || len(a.AssertionErrors) != 1 {
		t.Errorf("Expects = %+v, AssertionErrors = %v, want one error", a.Expects, a.AssertionErrors)
	}
}
