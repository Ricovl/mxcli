// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A stored text with every character the two languages spell differently: a
// CR LF (TestApp's WorkflowCommons.ASU_UserTaskView_Migrate annotation ends in
// one), a tab, backslashes before t and n, and an apostrophe.
const storedText = "It's in C:\\temp\\new\tnow.\r\n"

func mdl1Ctx() *ExecContext { return &ExecContext{LanguageVersion: langver.V1} }

// mdl0Ctx is a describe asked for in mdl 0 (`describe --mdl 0`, the REPL's
// `mdl 0;`): since the freeze (ako/mxcli#714) that has to be said, the default
// being mdl 1.
func mdl0Ctx() *ExecContext {
	v := langver.V0
	return &ExecContext{describeLang: &v}
}

// enumCaption parses `create enumeration` with the literal as a caption and
// returns the caption it reads back as.
func enumCaption(t *testing.T, header, lit string) string {
	t.Helper()
	prog, errs := visitor.Build(header + "create enumeration M.C (A " + lit + ");\n")
	if len(errs) > 0 {
		t.Fatalf("%q%s does not parse: %v", header, lit, errs[0])
	}
	return prog.Statements[len(prog.Statements)-1].(*ast.CreateEnumerationStmt).Values[0].Caption
}

// Under mdl 1 describe writes a string with `”` as the only escape and the
// line break in the literal itself; asked for mdl 0 it writes the mdl 0
// escapes (ako/mxcli#804, #840). Since the freeze mdl 1 is the default, in a
// headerless script too. Each spelling reads back as the stored text under
// the language it was written for — and, as the control, not under the other.
func TestMdlQuote_FollowsTheDescribeLanguage(t *testing.T) {
	mdl1 := mdlQuote(mdl1Ctx(), storedText)
	if want := "'It''s in C:\\temp\\new\tnow.\r\n'"; mdl1 != want {
		t.Errorf("mdl 1:\n got  %q\n want %q", mdl1, want)
	}
	mdl0 := mdlQuote(mdl0Ctx(), storedText)
	if want := `'It''s in C:\\temp\\new\tnow.\r\n'`; mdl0 != want {
		t.Errorf("mdl 0:\n got  %q\n want %q", mdl0, want)
	}
	for name, ctx := range map[string]*ExecContext{"no context": nil, "a headerless script": {}} {
		if got := mdlQuote(ctx, storedText); got != mdl1 {
			t.Errorf("%s describes as the frozen mdl 1, got %q", name, got)
		}
	}
	pinned := langver.V0
	if got := mdlQuote(&ExecContext{LanguageVersion: langver.V1, describeIn: &pinned}, storedText); got != mdl0 {
		t.Errorf("describeIn pins the language, got %q", got)
	}
	session := langver.V1
	if got := mdlQuote(&ExecContext{describeLang: &session, describeIn: &pinned}, storedText); got != mdl0 {
		t.Errorf("describeIn wins over the session's language, got %q", got)
	}

	if got := enumCaption(t, "mdl 1;\n", mdl1); got != storedText {
		t.Errorf("the mdl 1 spelling reads back under mdl 1 as %q", got)
	}
	if got := enumCaption(t, "", mdl0); got != storedText {
		t.Errorf("the mdl 0 spelling reads back under mdl 0 as %q", got)
	}
	// Control: the mdl 0 spelling under mdl 1 is another text — the reported
	// failure (a backslash and an r and an n).
	if got := enumCaption(t, "mdl 1;\n", mdl0); got == storedText {
		t.Error("the mdl 0 spelling read back unchanged under mdl 1; the comparison cannot fail")
	}
}

// The describers that wrote mdl 0 escapes whatever the language: a free
// annotation, an attached one and an activity caption (microflows), a
// pluggable widget's String property (pages), and a stored expression's
// string (create / change members).
func TestDescribersQuoteForTheDescribeLanguage(t *testing.T) {
	note := &microflows.Annotation{BaseMicroflowObject: mkObj("note"), Caption: storedText}
	oc := &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{note}}
	lit1, lit0 := mdlQuote(mdl1Ctx(), storedText), mdlQuote(mdl0Ctx(), storedText)

	for _, c := range []struct {
		name       string
		mdl1, mdl0 string
	}{
		{"free annotation",
			strings.Join(prependFreeAnnotationLines(mdl1Ctx(), oc, []string{"return;"}), "\n"),
			strings.Join(prependFreeAnnotationLines(mdl0Ctx(), oc, []string{"return;"}), "\n")},
		{"pluggable widget property",
			explicitPropValue(mdl1Ctx(), rawExplicitProp{Key: "k", Value: storedText, ValueType: "String"}),
			explicitPropValue(mdl0Ctx(), rawExplicitProp{Key: "k", Value: storedText, ValueType: "String"})},
		{"activity caption", activityCaption(mdl1Ctx()), activityCaption(mdl0Ctx())},
	} {
		if !strings.Contains(c.mdl1, lit1) {
			t.Errorf("%s under mdl 1 does not write %q:\n%s", c.name, lit1, c.mdl1)
		}
		if !strings.Contains(c.mdl0, lit0) {
			t.Errorf("%s under mdl 0 does not write %q:\n%s", c.name, lit0, c.mdl0)
		}
	}

	const expr = "'{\n  \"a\": 1\r\n}' + $S"
	if got := escapeExpressionValue(mdl1Ctx(), expr); got != expr {
		t.Errorf("an expression under mdl 1 is written as stored, got %q", got)
	}
	if got, want := escapeExpressionValue(mdl0Ctx(), expr), `'{\n  "a": 1\r\n}' + $S`; got != want {
		t.Errorf("an expression under mdl 0:\n got  %q\n want %q", got, want)
	}
}

func activityCaption(ctx *ExecContext) string {
	act := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("act"), Caption: storedText},
	}
	var lines []string
	emitObjectAnnotations(act, &lines, "", &annotationEmitter{ctx: ctx}, nil, nil, map[model.ID]microflows.MicroflowObject{})
	return strings.Join(lines, "\n")
}

// A description is parsed back under the header of the language it was
// written in, so an internal re-parse (layout, create or modify's diff)
// reads what describe wrote.
func TestDescribedSourceCarriesTheDescribeLanguage(t *testing.T) {
	if got := describedSource(mdl0Ctx(), "x"); got != "x" {
		t.Errorf("mdl 0 needs no header, got %q", got)
	}
	if got := describedSource(nil, "x"); got != "mdl 1;\nx" {
		t.Errorf("the default describe language is the frozen mdl 1, got %q", got)
	}
	if got := describedSource(mdl1Ctx(), "x"); got != "mdl 1;\nx" {
		t.Errorf("mdl 1: got %q", got)
	}
	if got := describedSource(mdl1Ctx(), "mdl 1;\nx"); got != "mdl 1;\nx" {
		t.Errorf("a description that has the header keeps one, got %q", got)
	}
}
