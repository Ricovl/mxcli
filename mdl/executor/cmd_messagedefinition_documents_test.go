// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// Mendix 11.15 stores each message definition as a MessageDefinition2 document
// and has no collections (ako/mxcli#987). The two forms are each refused on the
// version that does not have them, and the document form is written, described
// and resolved by its two-part name.

func withProjectVersion(ctx *ExecContext, major, minor int) {
	ctx.Backend.(*mock.MockBackend).ProjectVersionFunc = func() *types.ProjectVersion {
		return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor,
			ProductVersion: fmt.Sprintf("%d.%d.0", major, minor)}
	}
}

func mdDocCreate() *ast.CreateMessageDefinitionStmt {
	return &ast.CreateMessageDefinitionStmt{
		Name: ast.QualifiedName{Module: "Sales", Name: "OrderMessage"},
		Definition: &ast.MessageDefinitionDef{
			Entity:      ast.QualifiedName{Module: "Sales", Name: "Order"},
			ExposedName: "Orders",
			Members: []*ast.MessageMemberDef{
				attrMember("OrderId"),
				assocMember("Order_Customer", "Customer", attrMember("Name")),
			},
		},
	}
}

func TestCreateMessageDefinitionDocument_WritesOn1115(t *testing.T) {
	ctx, _ := mdFixture(t)
	withProjectVersion(ctx, 11, 15)
	var written *model.MessageDefinitionDocument
	ctx.Backend.(*mock.MockBackend).CreateMessageDefinitionDocumentFunc =
		func(d *model.MessageDefinitionDocument) error { written = d; return nil }

	if err := execCreateMessageDefinition(ctx, mdDocCreate()); err != nil {
		t.Fatalf("create: %v", err)
	}
	if written == nil {
		t.Fatal("no document written")
	}
	if written.Name != "OrderMessage" || written.Root == nil || written.Root.Entity != "Sales.Order" {
		t.Fatalf("document = %+v", written)
	}
	if written.Root.ExposedName != "Orders" || written.Root.MaxOccurs != -1 {
		t.Errorf("root exposed=%q maxOccurs=%d, want Orders/-1", written.Root.ExposedName, written.Root.MaxOccurs)
	}
	if len(written.Root.Children) != 2 || written.Root.Children[1].MaxOccurs != 1 {
		t.Errorf("members not built as the collection entry's are: %+v", written.Root.Children)
	}
}

func TestCreateMessageDefinitionDocument_RefusedBelow1115(t *testing.T) {
	ctx, _ := mdFixture(t)
	withProjectVersion(ctx, 11, 14)
	ctx.Backend.(*mock.MockBackend).CreateMessageDefinitionDocumentFunc =
		func(*model.MessageDefinitionDocument) error {
			t.Error("a document was written below 11.15")
			return nil
		}

	err := execCreateMessageDefinition(ctx, mdDocCreate())
	if err == nil {
		t.Fatal("create message definition was accepted on 11.14")
	}
	for _, want := range []string{"11.15", "create message definition collection"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}
}

func TestCreateMessageDefinitionCollection_RefusedOn1115(t *testing.T) {
	ctx, _ := mdFixture(t)
	withProjectVersion(ctx, 11, 15)
	ctx.Backend.(*mock.MockBackend).CreateMessageDefinitionCollectionFunc =
		func(*model.MessageDefinitionCollection) error {
			t.Error("a collection was written on 11.15")
			return nil
		}

	err := execCreateMessageDefinitionCollection(ctx, mdCreate(&ast.MessageDefinitionDef{
		Name: "M", Entity: ast.QualifiedName{Module: "Sales", Name: "Order"},
		Members: []*ast.MessageMemberDef{attrMember("OrderId")},
	}))
	if err == nil {
		t.Fatal("create message definition collection was accepted on 11.15")
	}
	for _, want := range []string{"removed message definition collections", "create message definition Module.Name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}
}

// storedDoc builds the document the fixture's module would hold after a create.
func storedDoc(ctx *ExecContext, t *testing.T) *model.MessageDefinitionDocument {
	t.Helper()
	var written *model.MessageDefinitionDocument
	mb := ctx.Backend.(*mock.MockBackend)
	mb.CreateMessageDefinitionDocumentFunc = func(d *model.MessageDefinitionDocument) error { written = d; return nil }
	if err := execCreateMessageDefinition(ctx, mdDocCreate()); err != nil {
		t.Fatalf("create: %v", err)
	}
	mods, _ := mb.ListModules()
	written.ContainerID = mods[0].ID
	written.ID = nextID("md2")
	mb.ListMessageDefinitionDocumentsFunc = func() ([]*model.MessageDefinitionDocument, error) {
		return []*model.MessageDefinitionDocument{written}, nil
	}
	return written
}

// DESCRIBE prints the per-document form, and that output parses back into the
// same statement — the describe -> exec contract.
func TestDescribeMessageDefinitionDocument_ParsesBack(t *testing.T) {
	ctx, _ := mdFixture(t)
	withProjectVersion(ctx, 11, 15)
	storedDoc(ctx, t)
	out := &bytes.Buffer{}
	ctx.Output = out

	if err := execDescribeMessageDefinition(ctx, ast.QualifiedName{Module: "Sales", Name: "OrderMessage"}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "create or modify message definition Sales.OrderMessage") ||
		!strings.Contains(text, "for Sales.Order as 'Orders'") {
		t.Fatalf("describe output:\n%s", text)
	}
	prog, errs := visitor.Build("mdl 1;\n" + text)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs[0], text)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("parsed %d statements", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateMessageDefinitionStmt)
	if !ok {
		t.Fatalf("parsed %T", prog.Statements[0])
	}
	if !stmt.CreateOrModify || stmt.Definition.Entity.String() != "Sales.Order" ||
		stmt.Definition.ExposedName != "Orders" || len(stmt.Definition.Members) != 2 {
		t.Errorf("parsed back as %+v / %+v", stmt, stmt.Definition)
	}
}

// A mapping's source on 11.15 is the document's two-part name, and the
// pre-11.15 three-part one resolves to the document `mx convert` made of it —
// stored as the two-part name.
func TestFindMessageDefinition_DocumentReferences(t *testing.T) {
	ctx, _ := mdFixture(t)
	withProjectVersion(ctx, 11, 15)
	d := storedDoc(ctx, t)

	def, ref, err := findMessageDefinition(ctx, "Sales.OrderMessage")
	if err != nil || def == nil || ref != "Sales.OrderMessage" {
		t.Fatalf("two-part: def=%v ref=%q err=%v", def, ref, err)
	}

	// The converter's folder keeps the collection's name.
	folder := nextID("folder")
	withContainer(ctx.Cache.hierarchy, folder, d.ContainerID)
	ctx.Cache.hierarchy.folderNames[folder] = "MD_Order"
	d.ContainerID = folder
	def, ref, err = findMessageDefinition(ctx, "Sales.MD_Order.OrderMessage")
	if err != nil || def == nil || ref != "Sales.OrderMessage" {
		t.Fatalf("three-part on 11.15: def=%v ref=%q err=%v", def, ref, err)
	}
	if _, _, err := findMessageDefinition(ctx, "Sales.Other.OrderMessage"); err == nil {
		t.Error("a three-part reference through the wrong folder resolved")
	}
	if _, _, err := findMessageDefinition(ctx, "Sales.Nope"); err == nil ||
		!strings.Contains(err.Error(), "Sales.OrderMessage") {
		t.Errorf("unknown reference should list the documents: %v", err)
	}
}
