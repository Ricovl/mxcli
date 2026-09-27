// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// # create or modify microflow|nanoflow as diff-then-patch (plan item 4.2g)
//
// ADR-0012 decision 3: `create or modify` on a flow that exists compares the
// declared definition with the stored document, derives the minimal patch and
// applies it with the splice engine `alter microflow` uses (mfmutator). An
// empty patch writes nothing.
//
// The stored side is the flow as `describe` prints it, parsed back: that is the
// one rendering of a stored flow MDL already guarantees to re-parse, and it
// puts both sides in the same AST, so "the same definition" is a structural
// comparison rather than a second renderer (the #997 lesson). Statements are
// matched with declaredMatches — the whole statement, so its signature and its
// output variable — by a longest common subsequence; each run of unmatched
// statements between two matched ones becomes one insert, replace or drop,
// aimed at the stored activity by its @position and applied through the same
// alterFlowContext an `alter` statement uses. An `if` that differs only inside
// its branches is diffed branch by branch.
//
// What the splice cannot express — a changed header, a change inside a loop
// body or an error handler, a moved node — is not rebuilt under `mdl 1`: the whole-document rebuild is what reset curves, dropped merges and
// moved element IDs on Studio Pro-authored flows (#721 class A). It is refused
// with the reason instead. Under mdl 0 the rebuild still runs, with the
// MDL-V1-REBUILD warning (ADR-0011: a new refusal applies only under the
// header that opts into it).

// flowRebuildRefused is the language change: the lossy fallback becomes a
// refusal under mdl 1. It is gated in the executor rather than the visitor
// because whether a statement needs the fallback depends on what is stored, so
// no parse of the script can tell.
var flowRebuildRefused = langver.Change{
	Code:  "MDL-V1-REBUILD",
	Since: langver.V1,
	Old: "`create or modify microflow|nanoflow` rebuilds the whole stored flow when the change cannot be " +
		"spliced in, which resets curves, drops merges and renumbers element IDs",
	New: "a refusal that names the change the splice cannot make, with nothing written",
}

// flowDecl is what diff-then-patch needs of a CREATE MICROFLOW or CREATE
// NANOFLOW statement.
type flowDecl struct {
	nanoflow bool
	name     ast.QualifiedName
	body     []ast.MicroflowStatement
	folder   string
	// header returns the declared and the stored statement with body, folder
	// and name cleared, after the rules by which an absent clause keeps what
	// is stored have been applied, ready for declaredMatches.
	header func(stored ast.Statement) (declared, storedHeader any)
}

func (d *flowDecl) kind() string {
	if d.nanoflow {
		return "nanoflow"
	}
	return "microflow"
}

// notSpliceable is the reason a declared change has to fall back.
type notSpliceable struct{ reason string }

func (e *notSpliceable) Error() string { return e.reason }

func cannotSplice(format string, args ...any) error {
	return &notSpliceable{reason: fmt.Sprintf(format, args...)}
}

// modifyFlowInPlace applies a `create or modify` of an existing flow as a
// patch. handled is false when the statement is not this path's to apply — no
// such flow yet, or a change the splice cannot make under mdl 0 — and the
// caller then runs the create / rebuild path.
func modifyFlowInPlace(ctx *ExecContext, d *flowDecl) (handled bool, err error) {
	if _, err := findModule(ctx, d.name.Module); err != nil {
		return false, nil // the create path makes the module
	}
	alter := &ast.AlterFlowStmt{Nanoflow: d.nanoflow, Name: d.name}
	a, err := loadAlterFlow(ctx, alter)
	if err != nil {
		var nf *mdlerrors.NotFoundError
		if errors.As(err, &nf) {
			return false, nil // a create
		}
		return fallBack(ctx, d, cannotSplice("the stored %s cannot be read for splicing: %v", d.kind(), err))
	}

	stored, err := describedFlowStmt(ctx, d, a)
	if err != nil {
		return fallBack(ctx, d, cannotSplice("its description cannot be compared: %v", err))
	}
	decl, storedHeader := d.header(stored)
	if !declaredMatches(decl, storedHeader) {
		return fallBack(ctx, d, cannotSplice("the header changes (parameters, return type or document properties); "+
			"the splice edits the flow's activities only"))
	}

	ops, targets, err := diffFlowBody(a, d.body, storedBody(stored))
	if err != nil {
		return fallBack(ctx, d, err)
	}

	storedFolder := storedFolderOf(stored)
	if len(ops) > 0 {
		mut, err := a.apply(ctx, ops, targets)
		if err != nil {
			return fallBack(ctx, d, cannotSplice("%v", err))
		}
		if err := mut.Save(); err != nil {
			return true, mdlerrors.NewBackend("save modified "+d.kind(), err)
		}
	}
	containerID := a.mf.ContainerID
	if d.folder != storedFolder {
		mod, err := findModule(ctx, d.name.Module)
		if err != nil {
			return true, err
		}
		to := mod.ID
		if d.folder != "" {
			if to, err = resolveFolder(ctx, mod.ID, d.folder); err != nil {
				return true, mdlerrors.NewBackend("resolve folder "+d.folder, err)
			}
		}
		if _, err := applyDocumentFolder(ctx, a.mf.ID, a.mf.ContainerID, to); err != nil {
			return true, err
		}
		containerID = to
	}

	switch {
	case len(ops) > 0:
		ctx.ReportMutation("Modified", "%s: %s (%s)", d.kind(), d.name, patchSummary(ops))
	case d.folder != storedFolder:
		ctx.ReportMutation("Moved", "%s: %s", d.kind(), d.name)
	default:
		reportUnchanged(ctx, fmt.Sprintf("%s: %s", d.kind(), d.name))
	}

	returnEntity := extractEntityFromReturnType(a.mf.ReturnType)
	if d.nanoflow {
		ctx.trackCreatedNanoflow(d.name.Module, d.name.Name, a.mf.ID, containerID, returnEntity)
	} else {
		ctx.trackCreatedMicroflow(d.name.Module, d.name.Name, a.mf.ID, containerID, returnEntity)
	}
	invalidateHierarchy(ctx)
	return true, nil
}

// fallBack decides what a change the splice cannot make does: refused under
// mdl 1, the whole-document rebuild with a warning under mdl 0.
func fallBack(ctx *ExecContext, d *flowDecl, why error) (bool, error) {
	if flowRebuildRefused.Applies(ctx.LanguageVersion) {
		return true, mdlerrors.NewValidation(fmt.Sprintf(
			"create or modify %s %s: this change cannot be spliced into the stored flow: %v. "+
				"Nothing was written: rebuilding the whole flow instead would reset what Studio Pro drew "+
				"(curves, merges, element IDs). Change activities with `alter %s %s { … }`; "+
				"to rebuild the flow deliberately, drop the %s and create it",
			d.kind(), d.name, why, d.kind(), d.name, d.kind()))
	}
	fmt.Fprintf(ctx.progress(), "Warning [%s]: %s %s is rebuilt as a whole: %v. %s\n",
		flowRebuildRefused.Code, d.kind(), d.name, why, flowRebuildRefused.Warning(ctx.LanguageVersion))
	return false, nil
}

// reportUnchanged reports a statement that wrote nothing, collapsing into the
// run's summary like an elided write does.
func reportUnchanged(ctx *ExecContext, what string) {
	line := fmt.Sprintf("Unchanged %s\n", what)
	if ctx.tally.countUnchanged(line) {
		return
	}
	fmt.Fprint(ctx.Output, line)
}

// describedFlowStmt describes the stored flow and parses the description
// under the language version of the script being run, so both sides are read
// by the same rules. See correctAmbiguousRanges for the one stored state the
// description cannot state under mdl 0.
func describedFlowStmt(ctx *ExecContext, d *flowDecl, a *alterFlowContext) (ast.Statement, error) {
	var buf bytes.Buffer
	prev := ctx.Output
	ctx.Output = &buf
	var err error
	if d.nanoflow {
		err = describeNanoflow(ctx, d.name)
	} else {
		err = describeMicroflow(ctx, d.name)
	}
	ctx.Output = prev
	if err != nil {
		return nil, err
	}
	src := buf.String()
	if v := ctx.LanguageVersion; v > langver.V0 {
		src = v.String() + ";\n" + src
	}
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, fmt.Errorf("the description does not parse: %v", errs[0])
	}
	for _, st := range prog.Statements {
		switch st.(type) {
		case *ast.CreateMicroflowStmt:
			if d.nanoflow {
				continue
			}
		case *ast.CreateNanoflowStmt:
			if !d.nanoflow {
				continue
			}
		default:
			continue
		}
		if !limitOneIsListUnder(ctx.LanguageVersion) {
			correctAmbiguousRanges(st, a.mf.ObjectCollection)
		}
		return st, nil
	}
	return nil, fmt.Errorf("the description has no create statement")
}

// limitOneIsListUnder reports whether `limit 1` reads as a list of one under v
// (the visitor's MDL-V1-LIMIT1 change).
func limitOneIsListUnder(v langver.Version) bool { return v >= langver.V1 }

// correctAmbiguousRanges fixes the stored side where its mdl 0 description
// says something other than what is stored.
//
// A retrieve with a Custom range of `limit 1` and no offset — a list of one —
// is described as `limit 1`, which mdl 0 reads as the object range
// (ako/mxcli#734): mdl 0 has no spelling for that exact state. Parsed as is,
// the stored side would claim the object range, and a script stating the
// object range would look unchanged against a flow that holds a list; so the
// parsed statement is set back to the list it stands for. The statement is
// found by its output variable and its @position.
func correctAmbiguousRanges(st ast.Statement, oc *microflows.MicroflowObjectCollection) {
	type key struct {
		v string
		p model.Point
	}
	lists := map[key]bool{}
	var collect func(oc *microflows.MicroflowObjectCollection)
	collect = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, obj := range oc.Objects {
			if loop, ok := obj.(*microflows.LoopedActivity); ok {
				collect(loop.ObjectCollection)
				continue
			}
			act, ok := obj.(*microflows.ActionActivity)
			if !ok {
				continue
			}
			r, ok := act.Action.(*microflows.RetrieveAction)
			if !ok {
				continue
			}
			src, ok := r.Source.(*microflows.DatabaseRetrieveSource)
			if !ok || src.Range == nil || src.Range.RangeType != microflows.RangeTypeCustom ||
				src.Range.Limit != "1" || src.Range.Offset != "" {
				continue
			}
			lists[key{r.OutputVariable, act.Position}] = true
		}
	}
	collect(oc)
	if len(lists) == 0 {
		return
	}
	walkStatements(st, func(r *ast.RetrieveStmt) {
		if !r.First || r.Annotations == nil || r.Annotations.Position == nil {
			return
		}
		k := key{r.Variable, model.Point{X: r.Annotations.Position.X, Y: r.Annotations.Position.Y}}
		if lists[k] {
			r.First, r.Limit, r.Offset = false, "1", ""
		}
	})
}

// walkStatements calls fn for every retrieve statement in v, at any depth.
func walkStatements(v any, fn func(*ast.RetrieveStmt)) {
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Pointer:
			if rv.IsNil() {
				return
			}
			if r, ok := rv.Interface().(*ast.RetrieveStmt); ok {
				fn(r)
			}
			walk(rv.Elem())
		case reflect.Struct:
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i))
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(v))
}

func storedBody(st ast.Statement) []ast.MicroflowStatement {
	switch s := st.(type) {
	case *ast.CreateMicroflowStmt:
		return s.Body
	case *ast.CreateNanoflowStmt:
		return s.Body
	}
	return nil
}

func storedFolderOf(st ast.Statement) string {
	switch s := st.(type) {
	case *ast.CreateMicroflowStmt:
		return s.Folder
	case *ast.CreateNanoflowStmt:
		return s.Folder
	}
	return ""
}

// microflowDecl adapts a CREATE MICROFLOW statement.
func microflowDecl(s *ast.CreateMicroflowStmt) *flowDecl {
	return &flowDecl{
		name: s.Name, body: s.Body, folder: s.Folder,
		header: func(stored ast.Statement) (any, any) {
			st, _ := stored.(*ast.CreateMicroflowStmt)
			if st == nil {
				return s, nil
			}
			dh, sh := *s, *st
			for _, h := range []*ast.CreateMicroflowStmt{&dh, &sh} {
				h.Body, h.Folder, h.Name, h.CreateOrModify = nil, "", ast.QualifiedName{}, false
			}
			// Absent means "keep what is stored" for these (see the field
			// comments on CreateMicroflowStmt), so absent is not a difference.
			if !dh.DocumentationSet {
				dh.Documentation = sh.Documentation
			}
			dh.DocumentationSet, sh.DocumentationSet = false, false
			if !dh.Excluded {
				dh.Excluded = sh.Excluded
			}
			if dh.ApplyEntityAccess == nil {
				dh.ApplyEntityAccess = sh.ApplyEntityAccess
			}
			if dh.Expose == nil {
				dh.Expose = sh.Expose
			}
			if dh.URL == nil {
				dh.URL = sh.URL
			}
			if dh.URLSearchParameters == nil {
				dh.URLSearchParameters = sh.URLSearchParameters
			}
			if dh.ExportLevel == nil {
				dh.ExportLevel = sh.ExportLevel
			}
			if dh.Concurrency == nil {
				dh.Concurrency = sh.Concurrency
			}
			return &dh, &sh
		},
	}
}

// nanoflowDecl adapts a CREATE NANOFLOW statement.
func nanoflowDecl(s *ast.CreateNanoflowStmt) *flowDecl {
	return &flowDecl{
		nanoflow: true, name: s.Name, body: s.Body, folder: s.Folder,
		header: func(stored ast.Statement) (any, any) {
			st, _ := stored.(*ast.CreateNanoflowStmt)
			if st == nil {
				return s, nil
			}
			dh, sh := *s, *st
			for _, h := range []*ast.CreateNanoflowStmt{&dh, &sh} {
				h.Body, h.Folder, h.Name, h.CreateOrModify = nil, "", ast.QualifiedName{}, false
			}
			if !dh.DocumentationSet {
				dh.Documentation = sh.Documentation
			}
			dh.DocumentationSet, sh.DocumentationSet = false, false
			if !dh.Excluded {
				dh.Excluded = sh.Excluded
			}
			if dh.Expose == nil {
				dh.Expose = sh.Expose
			}
			return &dh, &sh
		},
	}
}

// diffFlowBody derives the splice operations that turn the stored top-level
// statements into the declared ones. Matched statements are left alone; each
// run of unmatched statements between two matched ones becomes one operation:
//
//   - declared statements where none are stored: insert after the stored
//     statement before the run (or before the one after it);
//   - stored statements where none are declared: drop each;
//   - both: replace the first stored one with the declared run, drop the rest.
//
// Targets are the stored activities, located by the @position describe printed
// for each statement. A run whose stored end cannot be located, or that the
// splice cannot express, is a notSpliceable error.
func diffFlowBody(a *alterFlowContext, declared, stored []ast.MicroflowStatement) ([]*ast.AlterFlowOperation, []mfmutator.Candidate, error) {
	// Free annotations — notes wired to nothing — belong to the flow, not to
	// the statement describe happens to print them above (the first one), so
	// they are compared as a whole and kept out of the statement match: an
	// insert at the top would otherwise carry them onto a new statement and
	// write them a second time.
	declared, declaredFree := withoutFreeNotes(declared)
	stored, storedFree := withoutFreeNotes(stored)
	if !declaredMatches(declaredFree, storedFree) {
		return nil, nil, cannotSplice("the free annotations change; the splice edits activities only")
	}

	pd := &patchDiff{loc: newStoredLocator(a)}
	if err := pd.statements(declared, stored); err != nil {
		return nil, nil, err
	}
	// Drops go last. Each operation's scope check runs against the flow as the
	// earlier operations left it, so a stored activity whose output only a
	// replaced statement read can be dropped once that statement is replaced,
	// and not before. The splice itself does not care about the order: every
	// target is a stored activity no other operation touches.
	var ops []*ast.AlterFlowOperation
	var targets []mfmutator.Candidate
	for _, drops := range []bool{false, true} {
		for i, op := range pd.ops {
			if (op.Op == ast.AlterFlowDrop) == drops {
				ops = append(ops, op)
				targets = append(targets, pd.targets[i])
			}
		}
	}
	return ops, targets, nil
}

// patchDiff accumulates the operations of one diff.
type patchDiff struct {
	loc     *storedLocator
	ops     []*ast.AlterFlowOperation
	targets []mfmutator.Candidate
}

func (pd *patchDiff) add(op ast.AlterFlowOpKind, c mfmutator.Candidate, body []ast.MicroflowStatement) {
	pd.ops = append(pd.ops, &ast.AlterFlowOperation{Op: op, Target: targetLabel(c), Body: body})
	pd.targets = append(pd.targets, c)
}

// statements diffs one statement list: the flow's top level, or a branch of
// an `if`. The activities in an `if` branch are top-level nodes of the stored
// graph (only a loop nests a collection), so a branch is spliced the same way.
func (pd *patchDiff) statements(declared, stored []ast.MicroflowStatement) error {
	pairs := lcsStatements(declared, stored)
	di, si := 0, 0
	for p := 0; p <= len(pairs); p++ {
		dEnd, sEnd := len(declared), len(stored)
		if p < len(pairs) {
			dEnd, sEnd = pairs[p][0], pairs[p][1]
		}
		if err := pd.gap(declared[di:dEnd], stored, si, sEnd); err != nil {
			return err
		}
		if p < len(pairs) {
			di, si = pairs[p][0]+1, pairs[p][1]+1
		}
	}
	return nil
}

// gap turns one run of unmatched statements — ins declared where stored[si:sEnd]
// is stored — into operations.
func (pd *patchDiff) gap(ins []ast.MicroflowStatement, stored []ast.MicroflowStatement, si, sEnd int) error {
	del := stored[si:sEnd]
	switch {
	case len(ins) == 0 && len(del) == 0:
		return nil
	case len(del) == 0:
		op, c, err := insertAnchor(pd.loc, stored, si, sEnd)
		if err != nil {
			return err
		}
		pd.add(op, c, ins)
		return nil
	}
	if len(ins) == 1 && len(del) == 1 {
		if d, s, ok := sameIfShell(ins[0], del[0]); ok {
			// The same `if` with a change in a branch: splice the branches.
			if err := pd.statements(d.ThenBody, s.ThenBody); err != nil {
				return err
			}
			return pd.statements(d.ElseBody, s.ElseBody)
		}
		if sameIgnoringLayout(ins[0], del[0]) {
			p := statementAnnotations(del[0])
			where := ""
			if p != nil && p.Position != nil {
				where = fmt.Sprintf(" at (%d, %d)", p.Position.X, p.Position.Y)
			}
			return cannotSplice("the %s%s is moved or its connectors are redrawn; the splice places new nodes only and does not move stored ones",
				statementKind(del[0]), where)
		}
	}
	cands := make([]mfmutator.Candidate, len(del))
	for i, st := range del {
		c, err := pd.loc.locate(st)
		if err != nil {
			return err
		}
		cands[i] = c
	}
	rest, restStmts := cands, del
	if len(ins) > 0 {
		body, err := keepStoredNotes(del[0], ins)
		if err != nil {
			return err
		}
		pd.add(ast.AlterFlowReplace, cands[0], body)
		rest, restStmts = cands[1:], del[1:]
	}
	for i, c := range rest {
		if ann := statementAnnotations(restStmts[i]); ann != nil && len(ann.Notes) > 0 {
			return cannotSplice("the %s dropped at (%d, %d) carries an annotation, which would be left behind unattached",
				statementKind(restStmts[i]), c.Object.GetPosition().X, c.Object.GetPosition().Y)
		}
		pd.add(ast.AlterFlowDrop, c, nil)
	}
	return nil
}

// sameIfShell reports whether two statements are the same `if` — condition,
// annotations, whether it has an else — differing at most inside its branches.
func sameIfShell(declared, stored ast.MicroflowStatement) (*ast.IfStmt, *ast.IfStmt, bool) {
	d, ok1 := declared.(*ast.IfStmt)
	s, ok2 := stored.(*ast.IfStmt)
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	dShell, sShell := *d, *s
	dShell.ThenBody, dShell.ElseBody, sShell.ThenBody, sShell.ElseBody = nil, nil, nil, nil
	if !declaredMatches(&dShell, &sShell) {
		return nil, nil, false
	}
	return d, s, true
}

// keepStoredNotes prepares the declared statements that replace a stored one.
// The splice keeps the stored activity's annotation notes and attaches them to
// the replacement's first activity, so the declared statement must carry the
// same notes — and they are taken off it, or the builder would draw each one a
// second time. A replaced statement with other notes than the stored one is a
// change of annotations, which the splice does not make.
func keepStoredNotes(stored ast.MicroflowStatement, declared []ast.MicroflowStatement) ([]ast.MicroflowStatement, error) {
	var storedNotes []ast.MicroflowAnnotation
	if ann := statementAnnotations(stored); ann != nil {
		storedNotes = ann.Notes
	}
	if len(storedNotes) == 0 {
		return declared, nil
	}
	var declaredNotes []ast.MicroflowAnnotation
	if ann := statementAnnotations(declared[0]); ann != nil {
		declaredNotes = ann.Notes
	}
	if !declaredMatches(declaredNotes, storedNotes) {
		return nil, cannotSplice("the annotations on the replaced %s change; the splice keeps a replaced activity's notes as stored",
			statementKind(stored))
	}
	out := append([]ast.MicroflowStatement(nil), declared...)
	out[0] = withAnnotations(declared[0], func(a *ast.ActivityAnnotations) { a.Notes = nil })
	return out, nil
}

// withoutFreeNotes returns the statements with their free annotations taken
// off (as copies; the script's own statements are not modified, since the
// rebuild may still need them), and the free annotations in order.
func withoutFreeNotes(stmts []ast.MicroflowStatement) ([]ast.MicroflowStatement, []ast.MicroflowAnnotation) {
	var free []ast.MicroflowAnnotation
	out := make([]ast.MicroflowStatement, len(stmts))
	for i, st := range stmts {
		out[i] = st
		if ann := statementAnnotations(st); ann != nil && len(ann.FreeNotes) > 0 {
			free = append(free, ann.FreeNotes...)
			out[i] = withAnnotations(st, func(a *ast.ActivityAnnotations) { a.FreeNotes = nil })
		}
	}
	return out, free
}

// withAnnotations returns a shallow copy of st whose annotations are a copy
// edited by edit. st itself is left as it was.
func withAnnotations(st ast.MicroflowStatement, edit func(*ast.ActivityAnnotations)) ast.MicroflowStatement {
	v := reflectElem(st)
	if !v.IsValid() {
		return st
	}
	cp := reflect.New(v.Type())
	cp.Elem().Set(v)
	f := cp.Elem().FieldByName("Annotations")
	ann, ok := f.Interface().(*ast.ActivityAnnotations)
	if !ok || ann == nil {
		return st
	}
	annCopy := *ann
	edit(&annCopy)
	f.Set(reflect.ValueOf(&annCopy))
	out, ok := cp.Interface().(ast.MicroflowStatement)
	if !ok {
		return st
	}
	return out
}

// insertAnchor chooses where a run of new statements goes: after the stored
// activity before it when that is an activity (one flow leaves it), else
// before the stored statement after it. gapStart is the index of the first
// stored statement after the run's predecessor; next is the index of the
// matched statement after the run.
func insertAnchor(loc *storedLocator, stored []ast.MicroflowStatement, gapStart, next int) (ast.AlterFlowOpKind, mfmutator.Candidate, error) {
	if gapStart > 0 {
		if c, err := loc.locate(stored[gapStart-1]); err == nil {
			if _, ok := c.Object.(*microflows.ActionActivity); ok {
				return ast.AlterFlowInsertAfter, c, nil
			}
		}
	}
	if next < len(stored) {
		c, err := loc.locate(stored[next])
		if err != nil {
			return "", mfmutator.Candidate{}, err
		}
		return ast.AlterFlowInsertBefore, c, nil
	}
	if gapStart > 0 {
		c, err := loc.locate(stored[gapStart-1])
		if err != nil {
			return "", mfmutator.Candidate{}, err
		}
		return ast.AlterFlowInsertAfter, c, nil
	}
	return "", mfmutator.Candidate{}, cannotSplice("the stored flow has no statement to insert next to")
}

// lcsStatements pairs declared and stored statements that match, as a longest
// common subsequence; each pair is {declared index, stored index}, ascending.
func lcsStatements(declared, stored []ast.MicroflowStatement) [][2]int {
	n, m := len(declared), len(stored)
	eq := make([][]bool, n)
	for i := range declared {
		eq[i] = make([]bool, m)
		for j := range stored {
			eq[i][j] = declaredMatches(declared[i], stored[j])
		}
	}
	// l[i][j] is the LCS length of declared[i:] and stored[j:].
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case eq[i][j]:
				l[i][j] = l[i+1][j+1] + 1
			case l[i+1][j] >= l[i][j+1]:
				l[i][j] = l[i+1][j]
			default:
				l[i][j] = l[i][j+1]
			}
		}
	}
	var pairs [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case eq[i][j] && l[i][j] == l[i+1][j+1]+1:
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case l[i+1][j] >= l[i][j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}

// storedLocator finds the stored activity a statement of the stored flow's
// description stands for. describe prints each activity's @position, which is
// its RelativeMiddlePoint; among the top-level objects that is unique in any
// flow drawn so that nodes do not sit on top of each other.
type storedLocator struct {
	byPos map[model.Point][]mfmutator.Candidate
}

func newStoredLocator(a *alterFlowContext) *storedLocator {
	top := map[model.ID]bool{}
	for _, obj := range a.mf.ObjectCollection.Objects {
		top[obj.GetID()] = true
	}
	loc := &storedLocator{byPos: map[model.Point][]mfmutator.Candidate{}}
	for _, c := range a.cands {
		if top[c.ID] && c.Object != nil {
			p := c.Object.GetPosition()
			loc.byPos[p] = append(loc.byPos[p], c)
		}
	}
	return loc
}

func (l *storedLocator) locate(st ast.MicroflowStatement) (mfmutator.Candidate, error) {
	ann := statementAnnotations(st)
	if ann == nil || ann.Position == nil {
		return mfmutator.Candidate{}, cannotSplice("the stored statement %q has no activity to address (a join, merge or end of a branch)", statementKind(st))
	}
	p := model.Point{X: ann.Position.X, Y: ann.Position.Y}
	switch cs := l.byPos[p]; len(cs) {
	case 1:
		return cs[0], nil
	case 0:
		return mfmutator.Candidate{}, cannotSplice("no top-level activity is drawn at (%d, %d), where the stored %s is", p.X, p.Y, statementKind(st))
	default:
		return mfmutator.Candidate{}, cannotSplice("%d activities are drawn at (%d, %d); cannot tell which one the stored %s is", len(cs), p.X, p.Y, statementKind(st))
	}
}

// statementAnnotations returns a statement's annotations: every microflow
// statement that has them carries them in a field named Annotations.
func statementAnnotations(st ast.MicroflowStatement) *ast.ActivityAnnotations {
	v := reflectElem(st)
	if !v.IsValid() {
		return nil
	}
	f := v.FieldByName("Annotations")
	if !f.IsValid() {
		return nil
	}
	ann, _ := f.Interface().(*ast.ActivityAnnotations)
	return ann
}

func statementKind(st ast.MicroflowStatement) string {
	return strings.TrimSuffix(strings.TrimPrefix(fmt.Sprintf("%T", st), "*ast."), "Stmt")
}

// targetLabel is how an operation's target is named in a message: its alter
// handle when it has one, else its statement.
func targetLabel(c mfmutator.Candidate) string {
	if c.OutputVariable != "" {
		return "$" + c.OutputVariable
	}
	if c.Statement != "" {
		return c.Statement
	}
	return string(c.ID)
}

// patchSummary says what a patch did, e.g. "1 replaced, 2 inserted".
func patchSummary(ops []*ast.AlterFlowOperation) string {
	var ins, rep, drop int
	for _, op := range ops {
		switch op.Op {
		case ast.AlterFlowInsertAfter, ast.AlterFlowInsertBefore:
			ins++
		case ast.AlterFlowReplace:
			rep++
		case ast.AlterFlowDrop:
			drop++
		}
	}
	var parts []string
	for _, p := range []struct {
		n    int
		verb string
	}{{ins, "inserted"}, {rep, "replaced"}, {drop, "dropped"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.verb))
		}
	}
	return "spliced: " + strings.Join(parts, ", ")
}
