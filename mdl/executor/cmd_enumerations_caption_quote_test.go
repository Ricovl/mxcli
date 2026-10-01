// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#394: DESCRIBE ENUMERATION wrote a caption with an apostrophe
// as `'It's a test'`, which does not re-parse — the round trip of any caption a
// person would type ("Won't fix") was broken. The output must re-parse, under
// both describe languages, to the same caption; a folder path with an
// apostrophe is the same emit.
func TestDescribeEnumeration_CaptionApostropheRoundTrips(t *testing.T) {
	const caption = "It's a test"
	for _, lang := range []langver.Version{langver.V0, langver.V1} {
		mod := mkModule("MyFirstModule")
		folderID := nextID("folder")
		enum := &model.Enumeration{
			BaseElement: model.BaseElement{ID: nextID("enum")},
			ContainerID: folderID,
			Name:        "TestEnum",
			Values: []model.EnumerationValue{
				{BaseElement: model.BaseElement{ID: nextID("ev")}, Name: "Value1",
					Caption: &model.Text{Translations: map[string]string{"en_US": caption}}},
				{BaseElement: model.BaseElement{ID: nextID("ev")}, Name: "Value2",
					Caption: &model.Text{Translations: map[string]string{"en_US": "plain"}}},
			},
		}
		h := mkHierarchy(mod)
		withContainer(h, folderID, mod.ID)
		h.folderNames[folderID] = "Bob's"
		mb := &mock.MockBackend{
			IsConnectedFunc:      func() bool { return true },
			ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
		}
		ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
		ctx.describeLang = &lang
		assertNoError(t, describeEnumeration(ctx, ast.QualifiedName{Module: "MyFirstModule", Name: "TestEnum"}))
		out := buf.String()

		src := out
		if lang >= langver.V1 {
			src = "mdl 1;\n" + out
		}
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("%v: describe output does not re-parse: %v\n%s", lang, errs, out)
		}
		var stmt *ast.CreateEnumerationStmt
		for _, s := range prog.Statements {
			if c, ok := s.(*ast.CreateEnumerationStmt); ok {
				stmt = c
			}
		}
		if stmt == nil || len(stmt.Values) != 2 {
			t.Fatalf("%v: re-parsed to %#v\n%s", lang, prog.Statements, out)
		}
		if stmt.Values[0].Caption != caption {
			t.Errorf("%v: caption re-parses as %q, want %q\n%s", lang, stmt.Values[0].Caption, caption, out)
		}
		if !strings.Contains(out, "Bob''s") {
			t.Errorf("%v: folder path apostrophe not doubled:\n%s", lang, out)
		}
	}
}
