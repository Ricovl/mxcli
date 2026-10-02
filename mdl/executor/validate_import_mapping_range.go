// SPDX-License-Identifier: Apache-2.0

// Check-time validation of an import activity's Range against the shape of the
// mapping it calls (ako/mxcli#570).
//
// An OBJECT-rooted import mapping returns one object; Studio Pro's Range only
// means something on a mapping that returns a list. Two ranges were accepted on
// an object-rooted mapping, and neither is caught before the activity runs:
//
//	FIRST   check, exec and mx check all pass (0 errors). The activity then
//	        throws when it runs — `first` stores ForceSingleOccurrence=true, and
//	        on an object-rooted mapping that is the
//	          MicroflowException: key not found: Path(QName(None,),None,)
//	        the builder's comments record (measured on 11.13.0). Reported on
//	        11.14.0 as a bare "exception during execution"; removing `first` —
//	        one token — made the tests pass.
//	OFFSET  mx check rejects it: [CE6100] "This entity does not support
//	        offset." at Import with mapping activity (measured on 11.14.0).
//
// The fix for both is to drop the range: an object-rooted mapping binds an
// object under ALL, Studio Pro's own default. `limit` alone is accepted by
// mxbuild and stores no ForceSingleOccurrence, so it is not judged here.
//
// The mapping's shape is read from the script when the script creates it (and
// its JSON structure), otherwise from the project. A shape that cannot be
// established — an XML or message-definition mapping, a `root a/b` path into a
// structure only the snippet knows, no project — leaves the statement alone:
// the rule may be quieter than the build, never louder.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

const importRangeRule = "MDL-MAP04"

// mappingShape is what the rule knows about a mapping's root.
type mappingShape int

const (
	shapeUnknown mappingShape = iota
	shapeObject
	shapeList
)

// importRangeUse is one `import from mapping … first|offset` in a flow.
type importRangeUse struct {
	flowKind string // "microflow" / "nanoflow"
	flow     ast.QualifiedName
	stmt     *ast.ImportFromMappingStmt
}

// ValidateImportMappingRange reports MDL-MAP04 for FIRST or OFFSET on an import
// whose mapping is object-rooted.
func ValidateImportMappingRange(prog *ast.Program, projectPath string) []linter.Violation {
	if prog == nil {
		return nil
	}
	uses := importRangeUses(prog)
	if len(uses) == 0 {
		return nil
	}

	shapes := newMappingShapeResolver(prog, projectPath)
	defer shapes.close()

	var out []linter.Violation
	for _, u := range uses {
		if shapes.shape(u.stmt.Mapping) != shapeObject {
			continue
		}
		out = append(out, importRangeViolation(u))
	}
	return out
}

// importRangeUses collects every import activity carrying a range the rule
// judges, in every flow body, including nested blocks and custom error
// handlers.
func importRangeUses(prog *ast.Program) []importRangeUse {
	var out []importRangeUse
	collect := func(kind string, name ast.QualifiedName, body []ast.MicroflowStatement) {
		var walk func([]ast.MicroflowStatement)
		walk = func(stmts []ast.MicroflowStatement) {
			forEachMicroflowStatement(stmts, func(s ast.MicroflowStatement) {
				if im, ok := s.(*ast.ImportFromMappingStmt); ok && (im.First || im.OffsetExpr != nil) {
					out = append(out, importRangeUse{flowKind: kind, flow: name, stmt: im})
				}
				if h := getErrorHandlerBody(s); len(h) > 0 {
					walk(h)
				}
			})
		}
		walk(body)
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			collect("microflow", s.Name, s.Body)
		case *ast.CreateNanoflowStmt:
			collect("nanoflow", s.Name, s.Body)
		}
	}
	return out
}

func importRangeViolation(u importRangeUse) linter.Violation {
	mapping := u.stmt.Mapping.String()
	var msg, fix string
	if u.stmt.First {
		msg = fmt.Sprintf("`import from mapping %s(…) first`: %s is object-rooted — it already returns "+
			"one object, and `first` narrows a LIST. exec and mx check accept it (0 errors), and the "+
			"activity then throws when it runs (key not found: Path(QName(None,),None,))",
			mapping, mapping)
		fix = fmt.Sprintf("Drop `first` (or write `all`): an object-rooted mapping binds an object under "+
			"every range — `$%s = import from mapping %s($%s);`",
			orDefault(u.stmt.OutputVariable, "Result"), mapping, orDefault(u.stmt.SourceVariable, "Json"))
	} else {
		msg = fmt.Sprintf("`import from mapping %s(…) … offset`: %s is object-rooted, and Mendix accepts "+
			"an offset only on a mapping that returns a list (CE6100 \"This entity does not support offset.\")",
			mapping, mapping)
		fix = "Drop the `offset` (and the `limit`): an object-rooted mapping returns one object, so there " +
			"is nothing to page through. To page, map a JSON structure whose root is an array."
	}
	return linter.Violation{
		RuleID:   importRangeRule,
		Severity: linter.SeverityError,
		Message:  msg,
		Location: linter.Location{
			Module:       u.flow.Module,
			DocumentType: u.flowKind,
			DocumentName: u.flow.Name,
		},
		Suggestion: fix,
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// mappingShapeResolver answers "is this mapping object- or list-rooted" from
// the script first and the project second, opening the project only when a
// mapping is not the script's own.
type mappingShapeResolver struct {
	scriptMappings   map[string]*ast.CreateImportMappingStmt
	scriptStructures map[string]string // Module.Name -> JSON sample
	projectPath      string
	reader           backend.FullBackend
	opened           bool
}

func newMappingShapeResolver(prog *ast.Program, projectPath string) *mappingShapeResolver {
	r := &mappingShapeResolver{
		scriptMappings:   map[string]*ast.CreateImportMappingStmt{},
		scriptStructures: map[string]string{},
		projectPath:      projectPath,
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateImportMappingStmt:
			r.scriptMappings[s.Name.String()] = s
		case *ast.CreateJsonStructureStmt:
			r.scriptStructures[s.Name.String()] = s.JsonSnippet
		}
	}
	return r
}

func (r *mappingShapeResolver) close() {
	if r.reader != nil {
		_ = r.reader.Disconnect()
	}
}

func (r *mappingShapeResolver) project() backend.FullBackend {
	if !r.opened {
		r.opened = true
		r.reader = openProjectForValidation(r.projectPath)
	}
	return r.reader
}

func (r *mappingShapeResolver) shape(mapping ast.QualifiedName) mappingShape {
	if s, ok := r.scriptMappings[mapping.String()]; ok {
		return r.scriptMappingShape(s)
	}
	b := r.project()
	if b == nil {
		return shapeUnknown
	}
	im, err := b.GetImportMappingByQualifiedName(mapping.Module, mapping.Name)
	if err != nil || im == nil {
		return shapeUnknown
	}
	list, known := jsonMappingRootIsList(b, im)
	if !known {
		return shapeUnknown
	}
	if list {
		return shapeList
	}
	return shapeObject
}

// scriptMappingShape decides from the statement: a JSON-structure mapping
// rooted at the structure's own root has the structure's root shape.
func (r *mappingShapeResolver) scriptMappingShape(s *ast.CreateImportMappingStmt) mappingShape {
	if s.SchemaKind != "JSON_STRUCTURE" || s.SchemaRef.Module == "" || s.SchemaRoot != "" {
		return shapeUnknown
	}
	if sample, ok := r.scriptStructures[s.SchemaRef.String()]; ok {
		return jsonSampleRootShape(sample)
	}
	b := r.project()
	if b == nil {
		return shapeUnknown
	}
	js, err := b.GetJsonStructureByQualifiedName(s.SchemaRef.Module, s.SchemaRef.Name)
	if err != nil || js == nil || len(js.Elements) == 0 {
		return shapeUnknown
	}
	if js.Elements[0].ElementType == "Array" {
		return shapeList
	}
	return shapeObject
}

// jsonSampleRootShape reads the root kind off a JSON sample's first token.
func jsonSampleRootShape(sample string) mappingShape {
	t := strings.TrimSpace(sample)
	switch {
	case strings.HasPrefix(t, "["):
		return shapeList
	case strings.HasPrefix(t, "{"):
		return shapeObject
	}
	return shapeUnknown
}
