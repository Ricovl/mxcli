// SPDX-License-Identifier: Apache-2.0

// Package executor - Diff command implementation for comparing MDL scripts against project state
package executor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// DiffFormat represents the output format for diff results
type DiffFormat string

const (
	DiffFormatUnified    DiffFormat = "unified"
	DiffFormatSideBySide DiffFormat = "side"
	DiffFormatStructural DiffFormat = "struct"
)

// DiffOptions configures diff output
type DiffOptions struct {
	Format   DiffFormat
	UseColor bool
	Width    int
}

// ChangeType represents the type of structural change
type ChangeType string

const (
	ChangeAdded    ChangeType = "+"
	ChangeRemoved  ChangeType = "-"
	ChangeModified ChangeType = "~"
)

// StructuralChange represents a single structural change within an object
type StructuralChange struct {
	ChangeType  ChangeType
	ElementType string // "Attribute", "Parameter", "Value", etc.
	ElementName string
	Details     string
}

// DiffResult represents the diff for a single object
type DiffResult struct {
	ObjectType string
	ObjectName ast.QualifiedName
	Current    string // MDL from MPR (empty if new)
	Proposed   string // MDL from script
	IsNew      bool
	IsDeleted  bool
	Changes    []StructuralChange
	// Refused is why exec would refuse the statement, writing nothing, when
	// it would (ako/mxcli#839); "" otherwise.
	Refused string
	// Writes is what exec would write when the two renderings are the same
	// but exec still writes (a patch the rendering does not show, a move to
	// another folder); "" otherwise. It makes the statement modified.
	Writes string
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
)

// DiffProgram compares an MDL program against the current project state
func diffProgram(ctx *ExecContext, prog *ast.Program, opts DiffOptions) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Set defaults
	if opts.Format == "" {
		opts.Format = DiffFormatUnified
	}
	if opts.Width == 0 {
		opts.Width = 120
	}

	var results []DiffResult
	var newCount, modifiedCount, unchangedCount, refusedCount int

	// Track processed objects to avoid duplicates (script may have multiple statements for same object)
	processed := make(map[string]bool)

	// Process each statement
	// Statements diff cannot compare, and statements whose comparison failed.
	// Both are reported rather than dropped — see unsupportedDiffError.
	skipped := map[string]int{}
	var failures []string

	for _, stmt := range prog.Statements {
		result, err := diffStatement(ctx, stmt)
		if err != nil {
			var unsupported *unsupportedDiffError
			if errors.As(err, &unsupported) {
				skipped[unsupported.kind]++
			} else {
				failures = append(failures, err.Error())
			}
			continue
		}
		if result != nil {
			// CREATE … IF NOT EXISTS on an element that is already there is
			// skipped by exec and leaves the element untouched (#731), so what
			// the script would leave behind is what is stored now.
			if g, ok := stmt.(ast.IfNotExistsCreate); ok && g.CreateIfNotExists() && !result.IsNew {
				result.Proposed = result.Current
				result.Changes = nil
			}
			// Create unique key for deduplication
			key := result.ObjectType + ":" + result.ObjectName.String()
			if processed[key] {
				// Skip duplicate - already processed this object
				continue
			}
			processed[key] = true

			results = append(results, *result)
			if result.Refused != "" {
				refusedCount++
			} else if result.IsNew {
				newCount++
			} else if result.Current != result.Proposed || result.Writes != "" {
				modifiedCount++
			} else {
				unchangedCount++
			}
		}
	}

	// Output results based on format
	for _, result := range results {
		if result.Refused != "" {
			fmt.Fprintf(ctx.Output, "Refused: %s %s: exec would refuse this statement and write nothing: %s\n",
				result.ObjectType, result.ObjectName, result.Refused)
			continue
		}
		if result.Writes != "" && result.Current == result.Proposed {
			fmt.Fprintf(ctx.Output, "Modified: %s %s: its MDL renders as stored, but exec would write it: %s\n",
				result.ObjectType, result.ObjectName, result.Writes)
			continue
		}
		if result.Current == result.Proposed && !result.IsNew {
			// Skip unchanged objects unless showing structural
			if opts.Format != DiffFormatStructural {
				continue
			}
		}

		switch opts.Format {
		case DiffFormatUnified:
			outputUnifiedDiff(ctx, result, opts.UseColor)
		case DiffFormatSideBySide:
			outputSideBySideDiff(ctx, result, opts.Width, opts.UseColor)
		case DiffFormatStructural:
			outputStructuralDiff(ctx, result, opts.UseColor)
		}
	}

	// Output summary
	summary := fmt.Sprintf("\nSummary: %d new, %d modified, %d unchanged", newCount, modifiedCount, unchangedCount)
	if refusedCount > 0 {
		summary += fmt.Sprintf(", %d refused", refusedCount)
	}
	fmt.Fprintln(ctx.Output, summary)
	reportUndiffed(ctx, skipped, failures)

	return nil
}

// reportUndiffed prints what the summary above does NOT account for.
//
// The counts only ever describe statements diff understands, so a script made
// entirely of statements it does not understand summarises as all zeros. That
// reads as "nothing would change" for a script that may add documents, which
// is exactly the wrong answer from a pre-apply safety gate.
func reportUndiffed(ctx *ExecContext, skipped map[string]int, failures []string) {
	if len(skipped) > 0 {
		kinds := make([]string, 0, len(skipped))
		for k := range skipped {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		total := 0
		for _, n := range skipped {
			total += n
		}
		fmt.Fprintf(ctx.Output, "\nNot compared (%d statement(s)) — diff has no comparison for these,\n"+
			"so they are absent from the summary above, not unchanged:\n", total)
		for _, k := range kinds {
			fmt.Fprintf(ctx.Output, "  %s x%d\n", k, skipped[k])
		}
	}
	for _, f := range failures {
		fmt.Fprintf(ctx.Output, "\nCould not diff: %s\n", f)
	}
}

// DiffProgram is a method wrapper for external callers.
//
// The program runs under its own language header, as exec runs it: what a
// statement means, and whether exec would refuse it, depends on it.
func (e *Executor) DiffProgram(prog *ast.Program, opts DiffOptions) error {
	defer e.enterLanguage(prog.LanguageVersion)()
	return diffProgram(e.newExecContext(context.Background()), prog, opts)
}

// diffStatement generates a diff result for a single statement
func diffStatement(ctx *ExecContext, stmt ast.Statement) (*DiffResult, error) {
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		return diffEntity(ctx, s)
	case *ast.CreateViewEntityStmt:
		return diffViewEntity(ctx, s)
	case *ast.CreateEnumerationStmt:
		return diffEnumeration(ctx, s)
	case *ast.CreateAssociationStmt:
		return diffAssociation(ctx, s)
	case *ast.CreateMicroflowStmt:
		return diffMicroflow(ctx, s)
	case *ast.CreateNanoflowStmt:
		return diffNanoflow(ctx, s)
	default:
		return nil, &unsupportedDiffError{kind: statementKindName(stmt)}
	}
}

// unsupportedDiffError marks a statement diff has no comparison for. It is an
// error rather than a nil result so that diffProgram can SAY so: skipping
// silently made `diff` report "0 new, 0 modified, 0 unchanged" for a script
// that would genuinely add documents, which is worse than a wrong count
// because there is nothing on screen to disbelieve (#997).
type unsupportedDiffError struct{ kind string }

func (e *unsupportedDiffError) Error() string {
	return "diff does not compare " + e.kind + " statements"
}

// statementKindName turns an AST statement type into something an MDL author
// recognises: *ast.GrantMicroflowAccessStmt → "grant microflow access".
func statementKindName(stmt ast.Statement) string {
	name := strings.TrimPrefix(fmt.Sprintf("%T", stmt), "*ast.")
	name = strings.TrimSuffix(name, "Stmt")
	var out []rune
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, ' ')
		}
		out = append(out, unicode.ToLower(r))
	}
	return string(out)
}

// diffEntity compares a CREATE ENTITY statement against the project
func diffEntity(ctx *ExecContext, s *ast.CreateEntityStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Entity",
		ObjectName: s.Name,
		Proposed:   entityStmtToMDL(ctx, s),
	}

	// Try to find existing entity
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			// Found existing entity - get its MDL representation
			result.Current = entityToMDL(ctx, module.Name, entity, dm)
			result.Changes = compareEntities(ctx, result.Current, result.Proposed)
			return result, nil
		}
	}

	result.IsNew = true
	return result, nil
}

// diffViewEntity compares a CREATE VIEW ENTITY statement against the project
func diffViewEntity(ctx *ExecContext, s *ast.CreateViewEntityStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "View Entity",
		ObjectName: s.Name,
		Proposed:   viewEntityStmtToMDL(ctx, s),
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		result.IsNew = true
		return result, nil
	}

	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			result.Current = viewEntityFromProjectToMDL(ctx, module.Name, entity, dm)
			return result, nil
		}
	}

	result.IsNew = true
	return result, nil
}

// diffEnumeration compares a CREATE ENUMERATION statement against the project
func diffEnumeration(ctx *ExecContext, s *ast.CreateEnumerationStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Enumeration",
		ObjectName: s.Name,
		Proposed:   enumerationStmtToMDL(ctx, s),
	}

	// Try to find existing enumeration
	existingEnum := findEnumeration(ctx, s.Name.Module, s.Name.Name)
	if existingEnum == nil {
		result.IsNew = true
		return result, nil
	}

	// ContainerID is a folder when the enumeration is filed in one, so walk up
	// to the module before asking for its name; asking directly rendered the
	// stored side as `create enumeration .Name` and made an untouched
	// enumeration diff as modified (ako/mxcli#794). findEnumeration matched the
	// statement's module through the same walk, so that is the module name.
	h, _ := getHierarchy(ctx)
	modName := h.GetModuleName(h.FindModuleID(existingEnum.ContainerID))
	result.Current = enumerationToMDL(ctx, modName, existingEnum)
	result.Changes = compareEnumerations(ctx, result.Current, result.Proposed)

	return result, nil
}

// diffAssociation compares a CREATE ASSOCIATION statement against the project
func diffAssociation(ctx *ExecContext, s *ast.CreateAssociationStmt) (*DiffResult, error) {
	result := &DiffResult{
		ObjectType: "Association",
		ObjectName: s.Name,
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		result.IsNew = true
		result.Proposed = associationStmtToMDL(ctx, s, "")
		return result, nil
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		result.IsNew = true
		result.Proposed = associationStmtToMDL(ctx, s, "")
		return result, nil
	}

	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			result.Current = associationToMDL(ctx, module.Name, assoc, dm)
			result.Proposed = associationStmtToMDL(ctx, s, assoc.StorageFormat)
			return result, nil
		}
	}
	// A cross-module association is stored apart, in CrossAssociations; without
	// this lookup every existing one diffed as new.
	for _, ca := range dm.CrossAssociations {
		if ca.Name == s.Name.Name {
			result.Current = crossAssociationToMDL(module.Name, ca, dm)
			result.Proposed = associationStmtToMDL(ctx, s, ca.StorageFormat)
			return result, nil
		}
	}

	result.IsNew = true
	result.Proposed = associationStmtToMDL(ctx, s, "")
	return result, nil
}

// diffMicroflow compares a CREATE MICROFLOW statement against the project
func diffMicroflow(ctx *ExecContext, s *ast.CreateMicroflowStmt) (*DiffResult, error) {
	result := &DiffResult{ObjectType: "Microflow", ObjectName: s.Name}

	// Build the flow the script describes, without writing anything, then
	// render it through the SAME describer the stored side goes through. The
	// second AST-to-MDL renderer this replaces dropped every activity type it
	// did not know, which the diff then showed as a deletion (#997).
	built, err := buildMicroflowFromStmt(ctx, s, buildFlowOpts{})
	if err != nil {
		return nil, err
	}
	proposed, err := renderFlowFromModel(ctx, "microflow", built.Microflow, s.Name)
	if err != nil {
		return nil, err
	}
	result.Proposed = proposed

	if built.ExistingID == "" {
		result.IsNew = true
		return result, nil
	}

	stored, err := ctx.Backend.GetMicroflow(built.ExistingID)
	if err != nil || stored == nil {
		result.IsNew = true
		return result, nil
	}
	current, err := renderFlowFromModel(ctx, "microflow", stored, s.Name)
	if err != nil {
		return nil, err
	}
	result.Current = current
	result.Changes = compareMicroflows(ctx, result.Current, result.Proposed)
	if s.CreateOrModify {
		spliceVerdict(ctx, microflowDecl(s), result)
	}
	return result, nil
}

// diffNanoflow compares a CREATE NANOFLOW statement against the project
func diffNanoflow(ctx *ExecContext, s *ast.CreateNanoflowStmt) (*DiffResult, error) {
	result := &DiffResult{ObjectType: "Nanoflow", ObjectName: s.Name}

	built, err := buildNanoflowFromStmt(ctx, s, buildFlowOpts{})
	if err != nil {
		return nil, err
	}
	proposed, err := renderFlowFromModel(ctx, "nanoflow", nanoflowAsMicroflow(built.Nanoflow), s.Name)
	if err != nil {
		return nil, err
	}
	result.Proposed = proposed

	if built.ExistingID == "" {
		result.IsNew = true
		return result, nil
	}

	stored, err := ctx.Backend.GetNanoflow(built.ExistingID)
	if err != nil || stored == nil {
		result.IsNew = true
		return result, nil
	}
	current, err := renderFlowFromModel(ctx, "nanoflow", nanoflowAsMicroflow(stored), s.Name)
	if err != nil {
		return nil, err
	}
	result.Current = current
	result.Changes = compareMicroflows(ctx, result.Current, result.Proposed)
	if s.CreateOrModify {
		spliceVerdict(ctx, nanoflowDecl(s), result)
	}
	return result, nil
}

// spliceVerdict brings the diff of a `create or modify` of a stored flow to the
// verdict exec reaches, which is not the rendered comparison above: exec
// patches the stored flow (diff-then-patch, planFlowModify), and it is the
// patch that decides whether anything is written (ako/mxcli#839, where diff
// said unchanged for a statement exec refused).
//
//   - A change the splice cannot make is refused under mdl 1, and diff says
//     so; under mdl 0 exec rebuilds the flow, which the rendered comparison
//     already shows.
//   - An empty patch in the same folder writes nothing, whatever the two
//     renderings say, so it is unchanged.
//   - A patch to make, or a move to another folder, is a write whatever the
//     two renderings say: when they are the same (the rendering leaves the
//     folder out), what exec would write is stated instead.
//
// An error exec would report itself leaves the rendered comparison as it is.
func spliceVerdict(ctx *ExecContext, d *flowDecl, result *DiffResult) {
	p, err := planFlowModify(ctx, d)
	var why *notSpliceable
	switch {
	case errors.As(err, &why):
		if flowRebuildRefused.Applies(ctx.LanguageVersion) {
			result.Refused = why.reason
		}
	case err != nil || p == nil:
	case p.mut == nil && !movesFolder(d.folder, p.storedFolder):
		result.Proposed = result.Current
		result.Changes = nil
	case result.Current == result.Proposed:
		var what []string
		if s := patchSummary(p.ops, p.moves, p.set); s != "" {
			what = append(what, s)
		}
		if movesFolder(d.folder, p.storedFolder) {
			what = append(what, fmt.Sprintf("moved to folder '%s'", d.folder))
		}
		if len(what) == 0 {
			what = append(what, "patched")
		}
		result.Writes = strings.Join(what, "; ")
	}
}

// ============================================================================
// Structural Comparison Functions
// ============================================================================

// compareEntities extracts structural changes between two entity MDL representations
func compareEntities(ctx *ExecContext, current, proposed string) []StructuralChange {
	var changes []StructuralChange

	// Simple line-based comparison for now
	currentLines := strings.Split(current, "\n")
	proposedLines := strings.Split(proposed, "\n")

	// Extract attributes from both
	currentAttrs := extractAttributes(ctx, currentLines)
	proposedAttrs := extractAttributes(ctx, proposedLines)

	// Find added attributes
	for name, proposed := range proposedAttrs {
		if _, exists := currentAttrs[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Attribute",
				ElementName: name,
				Details:     proposed,
			})
		}
	}

	// Find removed attributes
	for name := range currentAttrs {
		if _, exists := proposedAttrs[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Attribute",
				ElementName: name,
			})
		}
	}

	// Find modified attributes
	for name, proposed := range proposedAttrs {
		if current, exists := currentAttrs[name]; exists && current != proposed {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeModified,
				ElementType: "Attribute",
				ElementName: name,
				Details:     "changed",
			})
		}
	}

	return changes
}

// compareEnumerations extracts structural changes between two enumeration MDL representations
func compareEnumerations(ctx *ExecContext, current, proposed string) []StructuralChange {
	var changes []StructuralChange

	currentValues := extractEnumValues(ctx, strings.Split(current, "\n"))
	proposedValues := extractEnumValues(ctx, strings.Split(proposed, "\n"))

	for name := range proposedValues {
		if _, exists := currentValues[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Value",
				ElementName: name,
			})
		}
	}

	for name := range currentValues {
		if _, exists := proposedValues[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Value",
				ElementName: name,
			})
		}
	}

	return changes
}

// compareMicroflows extracts structural changes between two microflow MDL representations
func compareMicroflows(ctx *ExecContext, current, proposed string) []StructuralChange {
	var changes []StructuralChange

	currentParams := extractParameters(ctx, strings.Split(current, "\n"))
	proposedParams := extractParameters(ctx, strings.Split(proposed, "\n"))

	for name := range proposedParams {
		if _, exists := currentParams[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Parameter",
				ElementName: name,
			})
		}
	}

	for name := range currentParams {
		if _, exists := proposedParams[name]; !exists {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Parameter",
				ElementName: name,
			})
		}
	}

	// Count body statements
	currentStmts := countBodyStatements(ctx, current)
	proposedStmts := countBodyStatements(ctx, proposed)
	if currentStmts != proposedStmts {
		diff := proposedStmts - currentStmts
		if diff > 0 {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeAdded,
				ElementType: "Body",
				ElementName: "statements",
				Details:     fmt.Sprintf("%d statements added", diff),
			})
		} else {
			changes = append(changes, StructuralChange{
				ChangeType:  ChangeRemoved,
				ElementType: "Body",
				ElementName: "statements",
				Details:     fmt.Sprintf("%d statements removed", -diff),
			})
		}
	}

	return changes
}

// extractAttributes extracts attribute definitions from MDL lines
func extractAttributes(_ *ExecContext, lines []string) map[string]string {
	attrs := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && !strings.HasPrefix(line, "create") && !strings.HasPrefix(line, "/**") && !strings.HasPrefix(line, "*") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				if !strings.HasPrefix(name, "$") && !strings.HasPrefix(name, "@") {
					attrs[name] = strings.TrimSuffix(strings.TrimSpace(parts[1]), ",")
				}
			}
		}
	}
	return attrs
}

// extractEnumValues extracts enumeration values from MDL lines
func extractEnumValues(_ *ExecContext, lines []string) map[string]bool {
	values := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "'") && !strings.HasPrefix(line, "create") {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				name := strings.TrimSuffix(parts[0], ",")
				if name != "" && !strings.HasPrefix(name, "/") && !strings.HasPrefix(name, "*") {
					values[name] = true
				}
			}
		}
	}
	return values
}

// extractParameters extracts parameter names from MDL lines
func extractParameters(_ *ExecContext, lines []string) map[string]bool {
	params := make(map[string]bool)
	inParams := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "create microflow") || strings.HasPrefix(line, "create nanoflow") ||
			strings.HasPrefix(line, "create or modify microflow") || strings.HasPrefix(line, "create or modify nanoflow") {
			inParams = true
			continue
		}
		if inParams {
			if strings.HasPrefix(line, ")") {
				inParams = false
				continue
			}
			if strings.HasPrefix(line, "$") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) >= 1 {
					name := strings.TrimPrefix(parts[0], "$")
					name = strings.TrimSuffix(name, ",")
					params[strings.TrimSpace(name)] = true
				}
			}
		}
	}
	return params
}

// countBodyStatements counts statements in a microflow body
func countBodyStatements(_ *ExecContext, mdl string) int {
	count := 0
	inBody := false
	for line := range strings.SplitSeq(mdl, "\n") {
		line = strings.TrimSpace(line)
		if line == "begin" {
			inBody = true
			continue
		}
		if line == "end;" {
			break
		}
		if inBody && line != "" && !strings.HasPrefix(line, "--") {
			count++
		}
	}
	return count
}
