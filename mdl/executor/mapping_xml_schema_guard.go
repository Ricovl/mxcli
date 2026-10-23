// SPDX-License-Identifier: Apache-2.0

// Guard-don't-drop for a mapping over an XML SCHEMA.
//
// `with xml schema M.S` names the schema and nothing else of the XML half. A
// mapping Studio Pro built over one also stores the schema element it is rooted
// at (XsdRootElementName) and, on every element, the schema path it binds to
// (XmlPath). MDL has no words for either, and mxcli does not read an XSD, so
// exec has nothing to derive them from: a rebuild wrote XsdRootElementName ""
// and an empty XmlPath on every element, with a JsonPath the schema does not
// have. Replaying a module's description therefore turned a working XML-schema
// mapping into CE0270 "No root element could be found in the schema"
// (mendixlabs/mxcli#1623) — the statement the user ran was their own describe
// output, which changes nothing.
//
// So there are two answers, and neither is a rebuild:
//
//   - a statement that IS the stored mapping's description is Unchanged, and
//     nothing is written — the describe → exec of a module;
//   - anything else is refused, naming what a rewrite would delete. The
//     precedent is the imported-web-service source (checkNoWebServiceSource):
//     carrying the XML half through a rebuild would mean claiming the rebuilt
//     elements still line up with the stored paths, which nothing here can
//     check without the XSD.
//
// A mapping with no stored XML state — the shape mxcli itself writes, root and
// paths empty — loses nothing on a rebuild and is rewritten as before.
package executor

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// storesXmlSchemaState reports whether a mapping carries XML state a rebuild
// would delete: a root element name, or any element's XmlPath.
func storesXmlSchemaState(root string, xmlPaths []string) bool {
	return root != "" || len(xmlPaths) > 0
}

// importXmlPaths lists the non-empty XmlPaths of an import mapping's elements.
func importXmlPaths(es []*model.ImportMappingElement) []string {
	var out []string
	for _, e := range es {
		if e.XmlPath != "" {
			out = append(out, e.XmlPath)
		}
		out = append(out, importXmlPaths(e.Children)...)
	}
	return out
}

// exportXmlPaths is importXmlPaths for an export mapping.
func exportXmlPaths(es []*model.ExportMappingElement) []string {
	var out []string
	for _, e := range es {
		if e.XmlPath != "" {
			out = append(out, e.XmlPath)
		}
		out = append(out, exportXmlPaths(e.Children)...)
	}
	return out
}

// importMappingXmlSchemaRewrite decides a `create or modify` of an existing
// import mapping over an XML schema. unchanged is true when the statement is
// the stored mapping's own description, and the caller writes nothing; a
// non-nil error refuses any other rewrite of one that carries XML state.
func importMappingXmlSchemaRewrite(ctx *ExecContext, s *ast.CreateImportMappingStmt,
	existing *model.ImportMapping,
) (unchanged bool, err error) {
	paths := importXmlPaths(existing.Elements)
	if existing.XmlSchema == "" || !storesXmlSchemaState(existing.RootElementName, paths) {
		return false, nil
	}
	if described, ok := describedMappingStmt(ctx, func() error { return describeImportMapping(ctx, s.Name) }); ok {
		if d, ok := described.(*ast.CreateImportMappingStmt); ok && sameImportMappingStmt(s, d) {
			return true, nil
		}
	}
	return false, xmlSchemaRewriteRefusal("import", s.Name.String(), existing.XmlSchema, existing.RootElementName, len(paths))
}

// exportMappingXmlSchemaRewrite is importMappingXmlSchemaRewrite for an export
// mapping, whose writer hardcoded the same empty root.
func exportMappingXmlSchemaRewrite(ctx *ExecContext, s *ast.CreateExportMappingStmt,
	existing *model.ExportMapping,
) (unchanged bool, err error) {
	paths := exportXmlPaths(existing.Elements)
	if existing.XmlSchema == "" || !storesXmlSchemaState(existing.RootElementName, paths) {
		return false, nil
	}
	if described, ok := describedMappingStmt(ctx, func() error { return describeExportMapping(ctx, s.Name) }); ok {
		if d, ok := described.(*ast.CreateExportMappingStmt); ok && sameExportMappingStmt(s, d) {
			return true, nil
		}
	}
	return false, xmlSchemaRewriteRefusal("export", s.Name.String(), existing.XmlSchema, existing.RootElementName, len(paths))
}

// describedMappingStmt runs a mapping describer in the script's own language
// and parses what it printed, so the stored side and the declared side are read
// by the same rules — the approach describedFlowStmt takes for a flow. ok is
// false when the description cannot be produced or does not parse, which the
// caller treats as "not the same statement".
func describedMappingStmt(ctx *ExecContext, describe func() error) (ast.Statement, bool) {
	var buf bytes.Buffer
	prevOut, prevIn := ctx.Output, ctx.describeIn
	script := ctx.LanguageVersion
	ctx.Output, ctx.describeIn = &buf, &script
	err := describe()
	src := describedSource(ctx, buf.String())
	ctx.Output, ctx.describeIn = prevOut, prevIn
	if err != nil {
		return nil, false
	}
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, false
	}
	for _, st := range prog.Statements {
		switch st.(type) {
		case *ast.CreateImportMappingStmt, *ast.CreateExportMappingStmt:
			return st, true
		}
	}
	return nil, false
}

// sameImportMappingStmt reports whether a statement states exactly what the
// stored mapping's description does. A clause the statement leaves out that
// means "keep what is stored" — folder, doc comment, @excluded — is taken from
// the description, so omitting it is not a difference.
func sameImportMappingStmt(s, described *ast.CreateImportMappingStmt) bool {
	a, b := *s, *described
	a.CreateGuard, a.CreateOrModify = b.CreateGuard, b.CreateOrModify
	if a.Folder == "" {
		a.Folder = b.Folder
	}
	if !a.DocumentationSet {
		a.Documentation, a.DocumentationSet = b.Documentation, b.DocumentationSet
	}
	if !a.Excluded {
		a.Excluded = b.Excluded
	}
	return reflect.DeepEqual(a, b)
}

// sameExportMappingStmt is sameImportMappingStmt for an export mapping.
func sameExportMappingStmt(s, described *ast.CreateExportMappingStmt) bool {
	a, b := *s, *described
	a.CreateGuard, a.CreateOrModify = b.CreateGuard, b.CreateOrModify
	if a.Folder == "" {
		a.Folder = b.Folder
	}
	if !a.DocumentationSet {
		a.Documentation, a.DocumentationSet = b.Documentation, b.DocumentationSet
	}
	if !a.Excluded {
		a.Excluded = b.Excluded
	}
	return reflect.DeepEqual(a, b)
}

// xmlSchemaRewriteRefusal is the message for a rewrite that would delete an
// XML-schema mapping's XML half.
func xmlSchemaRewriteRefusal(kind, qualifiedName, schema, root string, paths int) error {
	what := fmt.Sprintf("the schema path of its %d element(s)", paths)
	if root != "" {
		what = fmt.Sprintf("its root element %q and %s", root, what)
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"%s mapping %s is over the xml schema %s, and MDL cannot state the XML half of it: "+
			"%s. A rewrite from this statement would write none of them — mxcli does not read the "+
			"XSD to derive them — and mxbuild then reports CE0270 \"No root element could be found "+
			"in the schema\". Re-executing the mapping's own describe output is accepted and changes "+
			"nothing; to change the mapping itself, edit it in Studio Pro",
		kind, qualifiedName, schema, what))
}
