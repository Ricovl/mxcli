// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// xmlOrderMapping is an import mapping over an XML schema as a Studio Pro
// import leaves it: a root element name and an XmlPath on every element, with
// a microflow resolving the nested object — the shape of mendixlabs/mxcli#1623.
// MDL can state none of the XML half, and exec has no XSD to derive it from.
func xmlOrderMapping(containerID model.ID) *model.ImportMapping {
	value := func(attr, name, path string) *model.ImportMappingElement {
		return &model.ImportMappingElement{
			BaseElement: model.BaseElement{ID: nextID("ve"), TypeName: "ImportMappings$ValueMappingElement"},
			Kind:        "Value", Attribute: attr, DataType: "String",
			ExposedName: name, XmlPath: path, MinOccurs: 1, MaxOccurs: 1,
		}
	}
	return &model.ImportMapping{
		BaseElement:     model.BaseElement{ID: nextID("im")},
		ContainerID:     containerID,
		Name:            "IMM_Order",
		XmlSchema:       "XM.XS_Order",
		RootElementName: "Order",
		Elements: []*model.ImportMappingElement{{
			BaseElement: model.BaseElement{ID: nextID("oe"), TypeName: "ImportMappings$ObjectMappingElement"},
			Kind:        "Object", Entity: "XM.Order", ObjectHandling: "Create", ObjectHandlingBackup: "Create",
			ExposedName: "Order", XmlPath: "Order", MinOccurs: 1, MaxOccurs: 1,
			Children: []*model.ImportMappingElement{
				value("XM.Order.Number", "Number", "Order|Number"),
				{
					BaseElement: model.BaseElement{ID: nextID("oe"), TypeName: "ImportMappings$ObjectMappingElement"},
					Kind:        "Object", Entity: "XM.Customer", Association: "XM.Customer_Order",
					ObjectHandling: "Custom", ObjectHandlingBackup: "Create",
					CustomHandler: &model.MappingMicroflowCall{
						Microflow: "XM.MF_GetByKey",
						Parameters: []*model.MappingMicroflowParameter{
							{Parameter: "XM.MF_GetByKey.Key", Source: "parent", LevelOfParent: -1},
						},
					},
					ExposedName: "Customer", XmlPath: "Order|Customer", MinOccurs: 0, MaxOccurs: 1,
					Children: []*model.ImportMappingElement{
						value("XM.Customer.Key", "Key", "Order|Customer|Key"),
						value("XM.Customer.Name", "Name", "Order|Customer|Name"),
					},
				},
			},
		}},
	}
}

// xmlMappingBackend serves one stored import mapping and records every write.
func xmlMappingBackend(mod *model.Module, im *model.ImportMapping, written *[]*model.ImportMapping) *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetImportMappingByQualifiedNameFunc: func(module, name string) (*model.ImportMapping, error) {
			return im, nil
		},
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) { return []*model.ImportMapping{im}, nil },
		UpdateImportMappingFunc: func(w *model.ImportMapping) error {
			*written = append(*written, w)
			return nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{{ContainerID: mod.ID, Name: "MF_GetByKey"}}, nil
		},
	}
}

// droppedXmlState names what a written mapping lost against the stored one.
func droppedXmlState(stored, written *model.ImportMapping) []string {
	var lost []string
	if stored.RootElementName != "" && written.RootElementName != stored.RootElementName {
		lost = append(lost, "root element "+stored.RootElementName)
	}
	want, got := importXmlPathsOf(stored.Elements), importXmlPathsOf(written.Elements)
	if strings.Join(want, ",") != strings.Join(got, ",") {
		lost = append(lost, "XmlPaths "+strings.Join(want, ",")+" became "+strings.Join(got, ","))
	}
	return lost
}

func importXmlPathsOf(es []*model.ImportMappingElement) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.XmlPath)
		out = append(out, importXmlPathsOf(e.Children)...)
	}
	return out
}

// execImportMappingScript parses src and runs its one import mapping statement.
func execImportMappingScript(t *testing.T, ctx *ExecContext, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateImportMappingStmt); ok {
			return execCreateImportMapping(ctx, s)
		}
	}
	t.Fatalf("no import mapping statement in:\n%s", src)
	return nil
}

// mendixlabs/mxcli#1623 (ako/mxcli#1555): describe → exec of an XML-schema
// import mapping rebuilt it from the statement, and the statement cannot carry
// the root element (CE0270 afterwards) or any element's XmlPath. Re-executing
// the mapping's own description must not write at all.
func TestCreateOrModifyImportMapping_XmlSchemaRedescriptionIsUnchanged(t *testing.T) {
	mod := mkModule("XM")
	im := xmlOrderMapping(mod.ID)
	h := mkHierarchy(mod)
	withContainer(h, im.ContainerID, mod.ID)
	var written []*model.ImportMapping
	ctx, buf := newMockCtx(t, withBackend(xmlMappingBackend(mod, im, &written)), withHierarchy(h))

	assertNoError(t, describeImportMapping(ctx, ast.QualifiedName{Module: "XM", Name: "IMM_Order"}))
	described := buf.String()
	if !strings.Contains(described, "by XM.MF_GetByKey(Key: parent)") {
		t.Fatalf("description lost the microflow key:\n%s", described)
	}

	buf.Reset()
	if err := execImportMappingScript(t, ctx, described); err != nil {
		t.Fatalf("re-executing the description: %v\n%s", err, described)
	}
	for _, w := range written {
		t.Errorf("re-executing the description rewrote the mapping, losing: %s",
			strings.Join(droppedXmlState(im, w), "; "))
	}
	if out := buf.String(); !strings.Contains(out, "Unchanged import mapping XM.IMM_Order") {
		t.Errorf("want an Unchanged report, got:\n%s", out)
	}
}

// An edit to an XML-schema mapping cannot be written faithfully — the rebuild
// has no XmlPath for any element and no root element name — so it is refused,
// naming what would be lost, rather than written as a mapping mxbuild rejects.
func TestCreateOrModifyImportMapping_XmlSchemaEditIsRefused(t *testing.T) {
	mod := mkModule("XM")
	im := xmlOrderMapping(mod.ID)
	h := mkHierarchy(mod)
	withContainer(h, im.ContainerID, mod.ID)
	var written []*model.ImportMapping
	ctx, buf := newMockCtx(t, withBackend(xmlMappingBackend(mod, im, &written)), withHierarchy(h))

	assertNoError(t, describeImportMapping(ctx, ast.QualifiedName{Module: "XM", Name: "IMM_Order"}))
	edited := strings.Replace(buf.String(), "Name = Name", "Name = Key", 1)

	err := execImportMappingScript(t, ctx, edited)
	if err == nil {
		t.Fatalf("an edited XML-schema mapping was accepted:\n%s", edited)
	}
	for _, want := range []string{"XM.IMM_Order", "xml schema", "root element", "CE0270"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal should mention %q, got: %v", want, err)
		}
	}
	for _, w := range written {
		t.Errorf("the edit was written, losing: %s", strings.Join(droppedXmlState(im, w), "; "))
	}
}

// Control: an XML-schema mapping that carries no XML state — the shape mxcli
// itself writes — loses nothing on a rebuild, so its rewrite is not refused.
func TestCreateOrModifyImportMapping_XmlSchemaWithoutStoredPathsStillRewrites(t *testing.T) {
	mod := mkModule("XM")
	im := xmlOrderMapping(mod.ID)
	im.RootElementName = ""
	var clear func([]*model.ImportMappingElement)
	clear = func(es []*model.ImportMappingElement) {
		for _, e := range es {
			e.XmlPath = ""
			clear(e.Children)
		}
	}
	clear(im.Elements)
	if storesXmlSchemaState(im.RootElementName, importXmlPaths(im.Elements)) {
		t.Fatal("control: a mapping with no root and no XmlPath reports stored XML state")
	}
	if !storesXmlSchemaState("Order", nil) || !storesXmlSchemaState("", []string{"Order"}) {
		t.Fatal("a stored root or XmlPath must count as XML state")
	}
}

// The export twin: its writer hardcoded the same empty root, so the same replay
// of a Studio Pro export mapping over an XML schema lost it too.
func TestCreateOrModifyExportMapping_XmlSchemaRedescriptionIsUnchanged(t *testing.T) {
	mod := mkModule("XM")
	em := &model.ExportMapping{
		BaseElement:     model.BaseElement{ID: nextID("em")},
		ContainerID:     mod.ID,
		Name:            "EXM_Order",
		XmlSchema:       "XM.XS_Order",
		RootElementName: "Order",
		Elements: []*model.ExportMappingElement{{
			BaseElement: model.BaseElement{ID: nextID("oe"), TypeName: "ExportMappings$ObjectMappingElement"},
			Kind:        "Object", Entity: "XM.Order", ObjectHandling: "Parameter",
			ExposedName: "Order", XmlPath: "Order", MinOccurs: 1, MaxOccurs: 1,
			Children: []*model.ExportMappingElement{{
				BaseElement: model.BaseElement{ID: nextID("ve"), TypeName: "ExportMappings$ValueMappingElement"},
				Kind:        "Value", Attribute: "XM.Order.Number", DataType: "String",
				ExposedName: "Number", XmlPath: "Order|Number", MinOccurs: 1, MaxOccurs: 1,
			}},
		}},
	}
	h := mkHierarchy(mod)
	withContainer(h, em.ContainerID, mod.ID)
	var written []*model.ExportMapping
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetExportMappingByQualifiedNameFunc: func(module, name string) (*model.ExportMapping, error) {
			return em, nil
		},
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) { return []*model.ExportMapping{em}, nil },
		UpdateExportMappingFunc: func(w *model.ExportMapping) error {
			written = append(written, w)
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))

	assertNoError(t, describeExportMapping(ctx, ast.QualifiedName{Module: "XM", Name: "EXM_Order"}))
	described := buf.String()

	run := func(src string) error {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse: %v\n%s", errs[0], src)
		}
		for _, stmt := range prog.Statements {
			if s, ok := stmt.(*ast.CreateExportMappingStmt); ok {
				return execCreateExportMapping(ctx, s)
			}
		}
		t.Fatalf("no export mapping statement in:\n%s", src)
		return nil
	}

	buf.Reset()
	if err := run(described); err != nil {
		t.Fatalf("re-executing the description: %v\n%s", err, described)
	}
	if len(written) != 0 {
		t.Errorf("re-executing the description rewrote the export mapping (root %q -> %q)",
			em.RootElementName, written[0].RootElementName)
	}
	if out := buf.String(); !strings.Contains(out, "Unchanged export mapping XM.EXM_Order") {
		t.Errorf("want an Unchanged report, got:\n%s", out)
	}

	edited := strings.Replace(described, "Number", "Total", 1)
	if edited == described {
		t.Fatalf("could not edit the description:\n%s", described)
	}
	if err := run(edited); err == nil || !strings.Contains(err.Error(), "CE0270") {
		t.Errorf("an edited XML-schema export mapping must be refused, got: %v", err)
	}
	if len(written) != 0 {
		t.Errorf("a refused export statement wrote the mapping")
	}
}
