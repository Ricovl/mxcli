// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A documentNameSpace is a group of element kinds that share one name space per
// module: two of them may not have the same name, whatever their kinds, their
// folders, or the casing of the name (ako/mxcli#793).
//
// The groups are measured, not assumed — mx check 11.14.0 over copies of
// TestApp, every pair of fourteen creatable kinds plus one of each kind the
// project already held. Only these three clash. A microflow shares a name with
// a page, constant, enumeration, java action, workflow or entity without
// complaint, a javascript action with a nanoflow, a page with a building block
// or page template. Each group is one abstract base type in the metamodel
// (MicroflowBase, FormBase, and the domain-model members plus enumerations),
// which is why the rule is per group rather than per module.
type documentNameSpace struct {
	kinds []string // doc-type keys, as stmtCreateKind returns them
	ce    string   // the consistency error Mendix reports
	what  string   // the kinds, for the message
}

var documentNameSpaces = []documentNameSpace{
	// [CE0122] "Duplicate document name 'M.X'." at Microflow 'M.X', Nanoflow 'M.X'
	{kinds: []string{"microflow", "nanoflow", "rule"}, ce: "CE0122", what: "microflows, nanoflows and rules"},
	// [CE0122] … at Page 'M.X', Snippet 'M.X' / Page 'M.X', Layout 'M.X'
	{kinds: []string{"page", "snippet", "layout"}, ce: "CE0122", what: "pages, snippets and layouts"},
	// [CE0065] "Duplicate name 'X' in module 'M'. Entities, associations and
	// enumerations cannot share names." — cross-module associations included.
	{kinds: []string{"entity", "association", "enumeration"}, ce: "CE0065", what: "entities, associations and enumerations"},
}

// nameSpaceOf returns the shared name space docType belongs to, or nil.
func nameSpaceOf(docType string) *documentNameSpace {
	for i := range documentNameSpaces {
		for _, k := range documentNameSpaces[i].kinds {
			if k == docType {
				return &documentNameSpaces[i]
			}
		}
	}
	return nil
}

// nameClashMessage is the one wording every tier reports: the statement's
// element, the element that already has the name, and the rule.
func nameClashMessage(docType, name, otherType, otherName string, ns *documentNameSpace) string {
	return fmt.Sprintf(
		"cannot create %s %s: %s %s already has that name — %s share one name space per module, "+
			"whatever their folder, and names compare case-insensitively (Mendix %s)",
		friendlyDocType(docType), name, friendlyDocType(otherType), otherName, ns.what, ns.ce)
}

const nameClashHint = "rename one of the two; a different folder does not separate them"

// foldedNames maps a lower-cased qualified name to its stored spelling.
type foldedNames map[string]string

func foldNameSet(set map[string]bool) foldedNames {
	out := make(foldedNames, len(set))
	for qn := range set {
		out[strings.ToLower(qn)] = qn
	}
	return out
}

// loadFoldedNames lists the project's elements of one name-space kind. A list
// that cannot be read yields no names, as every other build*QualifiedNames
// does: the handler that would write the element reads the same list and
// reports the failure itself.
func loadFoldedNames(ctx *ExecContext, docType string) foldedNames {
	switch docType {
	case "microflow":
		return foldNameSet(buildMicroflowQualifiedNames(ctx))
	case "nanoflow":
		return foldNameSet(buildNanoflowQualifiedNames(ctx))
	case "rule":
		return foldNameSet(buildRuleQualifiedNames(ctx))
	case "page":
		return foldNameSet(buildPageQualifiedNames(ctx))
	case "snippet":
		return foldNameSet(buildSnippetQualifiedNames(ctx))
	case "layout":
		return foldNameSet(buildLayoutQualifiedNames(ctx))
	case "entity":
		return foldNameSet(buildEntityQualifiedNames(ctx))
	case "association":
		return foldNameSet(buildAssociationQualifiedNames(ctx))
	case "enumeration":
		return foldNameSet(buildEnumerationQualifiedNames(ctx))
	}
	return nil
}

// refuseNameClash is the exec-time refusal: a create that would ADD an element
// whose name another kind in its name space already has in that module. It
// runs in the dispatch, before the handler, so nothing is written.
//
// A create whose own kind already has the name adds nothing — the handler
// refuses it as "already exists" or modifies the stored element in place — so
// it is left alone, even in a project that already holds such a clash.
func refuseNameClash(ctx *ExecContext, stmt ast.Statement) error {
	docType, name, _ := stmtCreateKind(stmt)
	ns := nameSpaceOf(docType)
	if ns == nil || !ctx.Connected() {
		return nil
	}
	key := strings.ToLower(name)
	if _, ok := loadFoldedNames(ctx, docType)[key]; ok {
		return nil
	}
	for _, other := range ns.kinds {
		if other == docType {
			continue
		}
		if stored, ok := loadFoldedNames(ctx, other)[key]; ok {
			return mdlerrors.NewAlreadyExistsMsg(other, stored,
				nameClashMessage(docType, name, other, stored, ns)+" — "+nameClashHint)
		}
	}
	return nil
}

// checkScriptNameClash reports a create whose name another kind in its name
// space already holds, alive in the script at that point (MDL-DUPNAME). reg is
// CheckScriptDuplicates' registry, before this statement is added to it.
func checkScriptNameClash(reg *nameRegistry, docType, name string) *linter.Violation {
	ns := nameSpaceOf(docType)
	if ns == nil {
		return nil
	}
	if _, _, ok := reg.findFold(docType, name); ok {
		return nil // the same kind: MDL-DUPDEF's business, or a modify
	}
	for _, other := range ns.kinds {
		if other == docType {
			continue
		}
		if stored, idx, ok := reg.findFold(other, name); ok {
			return &linter.Violation{
				RuleID:   "MDL-DUPNAME",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf("%s (statement %d)",
					nameClashMessage(docType, name, other, stored, ns), idx),
				Suggestion: nameClashHint,
			}
		}
	}
	return nil
}

// findFold looks name up case-insensitively among the alive names of docType.
func (r *nameRegistry) findFold(docType, name string) (string, int, bool) {
	for qn, idx := range r.alive[docType] {
		if strings.EqualFold(qn, name) {
			return qn, idx, true
		}
	}
	return "", 0, false
}

// CheckProjectNameClashes walks prog in statement order and reports every
// create that would add an element whose name another kind in its name space
// already has in the project (ako/mxcli#793). Unlike a same-kind conflict this
// is never an ordinary re-run — `create or modify` included — so exec's
// pre-flight refuses it too, before anything is written.
//
// Names the script itself creates are CheckScriptDuplicates' (MDL-DUPNAME); a
// project element the script drops or renames first no longer holds its name.
func CheckProjectNameClashes(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}
	project := map[string]foldedNames{}
	for _, ns := range documentNameSpaces {
		for _, k := range ns.kinds {
			project[k] = loadFoldedNames(ctx, k)
		}
	}
	var errs []error
	for i, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.RenameStmt); ok && !s.DryRun {
			if dt := renameDocType(s.ObjectType); dt != "" {
				delete(project[dt], strings.ToLower(s.Name.String()))
			}
			continue
		}
		if dt, name := stmtDropInfo(stmt); dt != "" {
			delete(project[dt], strings.ToLower(name))
			continue
		}
		dt, name, _ := stmtCreateKind(stmt)
		ns := nameSpaceOf(dt)
		if ns == nil {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := project[dt][key]; ok {
			continue // adds nothing: refused as a duplicate, or modified in place
		}
		for _, other := range ns.kinds {
			if other == dt {
				continue
			}
			if stored, ok := project[other][key]; ok {
				errs = append(errs, fmt.Errorf("statement %d: %s — %s",
					i+1, nameClashMessage(dt, name, other, stored, ns), nameClashHint))
				break
			}
		}
	}
	return errs
}
