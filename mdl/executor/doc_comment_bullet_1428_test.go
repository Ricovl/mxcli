// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1428: "Describe of a doc comment loses a markdown `*`
// bullet". The documentation "Intro line\n* bullet one\n* bullet two" was
// described with the ` * ` gutter on its first line only:
//
//	/**
//	 * Intro line
//	* bullet one
//	 */
//
// and the parser, which strips one leading `*` per line, then read the bullet
// marker as the gutter — each describe/exec cycle ate one marker.
const bulletDoc = "Intro line\n* bullet one\n* bullet two"

func bulletDocCtx(t *testing.T, doc string) (*ExecContext, *bytes.Buffer) {
	t.Helper()
	mod := mkModule("ModA")
	e := mkEntity(nextID("dm"), "EntA")
	e.Documentation = doc
	e.Attributes = []*domainmodel.Attribute{{
		BaseElement:   model.BaseElement{ID: "attr-name"},
		Name:          "Name",
		Documentation: doc,
		Type:          &domainmodel.StringAttributeType{Length: 50},
	}}
	dm := mkDomainModel(mod.ID, e)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	return newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
}

// describeParsedDocs describes ModA.EntA and returns the entity and attribute
// documentation the description parses back to.
func describeParsedDocs(t *testing.T, doc string) (text, entityDoc, attrDoc string) {
	t.Helper()
	ctx, out := bulletDocCtx(t, doc)
	if err := describeEntity(ctx, ast.QualifiedName{Module: "ModA", Name: "EntA"}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	text = out.String()
	prog, errs := visitor.Build(text)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, text)
	}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateEntityStmt); ok {
			entityDoc = s.Documentation
			if len(s.Attributes) > 0 {
				attrDoc = s.Attributes[0].Documentation
			}
			return
		}
	}
	t.Fatalf("no create entity in describe output:\n%s", text)
	return
}

// The entity's and the attribute's documentation both come back byte-equal,
// and describing the result again prints the same text: a fixed point.
func TestDescribeEntity_DocCommentKeepsBullet_Issue1428(t *testing.T) {
	for _, doc := range []string{
		bulletDoc,
		"* starts with a bullet",
		"one line",
		"First paragraph.\n\nSecond paragraph.",
	} {
		text, entityDoc, attrDoc := describeParsedDocs(t, doc)
		if entityDoc != doc {
			t.Errorf("entity documentation %q came back as %q\n%s", doc, entityDoc, text)
		}
		if attrDoc != doc {
			t.Errorf("attribute documentation %q came back as %q\n%s", doc, attrDoc, text)
		}
		again, _, _ := describeParsedDocs(t, entityDoc)
		if again != text {
			t.Errorf("second describe differs:\n--- first\n%s\n--- second\n%s", text, again)
		}
	}
}

// Every continuation line carries the gutter — the shape the reporter saw
// fail. An unguttered `* bullet one` line is exactly the defect.
func TestDescribeEntity_DocCommentGuttersEveryLine_Issue1428(t *testing.T) {
	text, _, _ := describeParsedDocs(t, bulletDoc)
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "* ") {
			t.Fatalf("doc line without the ` * ` gutter: %q\n%s", line, text)
		}
	}
	if !strings.Contains(text, " * * bullet one\n") {
		t.Errorf("bullet line not printed as ` * * bullet one`:\n%s", text)
	}
}
