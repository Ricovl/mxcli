// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// execAlterFlow handles `alter microflow|nanoflow Module.Name { … }`: a patch
// of the stored flow (ADR-0012 decision 3), never a rebuild.
//
// Every target is resolved against the flow AS STORED, before any operation
// runs, so an address means what `describe … with handles` showed: an
// ambiguity or a miss refuses the whole statement before anything changes, and
// a later operation cannot address what an earlier one inserted.
func execAlterFlow(ctx *ExecContext, s *ast.AlterFlowStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	a, err := loadAlterFlow(ctx, s)
	if err != nil {
		return err
	}

	targets := make([]mfmutator.Candidate, len(s.Operations))
	for i, op := range s.Operations {
		c, err := mfmutator.ResolveText(a.cands, op.Target)
		if err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("alter %s %s: %s %s: %v", s.Kind(), s.Name, op.Op, op.Target, err))
		}
		targets[i] = c
	}

	mut, err := a.apply(ctx, s.Operations, targets)
	if err != nil {
		return err
	}
	if err := mut.Save(); err != nil {
		return mdlerrors.NewBackend("save altered "+s.Kind(), err)
	}
	fmt.Fprintf(ctx.Output, "Altered %s %s\n", s.Kind(), s.Name)
	return nil
}

// apply opens the stored flow for splicing and applies ops, each aimed at the
// candidate at the same index of targets (resolved against the flow as stored).
// It returns the mutator unsaved: the caller saves, or discards it on error so
// nothing is written.
func (a *alterFlowContext) apply(ctx *ExecContext, ops []*ast.AlterFlowOperation, targets []mfmutator.Candidate) (backend.MicroflowMutator, error) {
	s := a.stmt
	mut, err := ctx.Backend.OpenMicroflowForMutation(a.mf.ID)
	if err != nil {
		return nil, mdlerrors.NewBackend("open "+s.Kind()+" for alter", err)
	}
	for i, op := range ops {
		target := targets[i]
		fail := func(err error) error {
			return mdlerrors.NewValidation(fmt.Sprintf("alter %s %s: %s %s: %v", s.Kind(), s.Name, op.Op, op.Target, err))
		}
		if op.Op == ast.AlterFlowDrop {
			if err := a.checkOutputUnused(target, nil); err != nil {
				return nil, fail(err)
			}
			if err := mut.Drop(target.ID); err != nil {
				return nil, fail(err)
			}
			a.noteRemoved(target, nil)
			continue
		}
		frag, err := a.buildFragment(ctx, op.Body)
		if err != nil {
			return nil, fail(err)
		}
		if err := a.checkFragmentScope(ctx, op, target, frag); err != nil {
			return nil, fail(err)
		}
		switch op.Op {
		case ast.AlterFlowInsertAfter:
			err = mut.InsertAfter(target.ID, frag)
		case ast.AlterFlowInsertBefore:
			err = mut.InsertBefore(target.ID, frag)
		case ast.AlterFlowReplace:
			if err = a.checkOutputUnused(target, frag); err == nil {
				err = mut.Replace(target.ID, frag)
			}
		default:
			err = fmt.Errorf("unknown operation")
		}
		if err != nil {
			return nil, fail(err)
		}
		if op.Op == ast.AlterFlowReplace {
			a.noteRemoved(target, frag)
		}
		a.noteFragment(ctx, frag)
	}
	return mut, nil
}

// alterFlowContext is what the operations of one statement share: the stored
// flow (a nanoflow wrapped as a microflow, the way describe renders one), its
// addressable activities, and the name maps rendering needs.
type alterFlowContext struct {
	stmt           *ast.AlterFlowStmt
	mf             *microflows.Microflow
	cands          []mfmutator.Candidate
	entityNames    map[model.ID]string
	microflowNames map[model.ID]string

	// What the statement's earlier operations did to the variables, so a later
	// one is checked against the flow as it will be written, not as stored:
	// the variables their fragments declare, the ones their fragments read
	// (with who reads them), and the stored outputs they took away.
	declaredByOps map[string]bool
	readByOps     map[string][]string
	removedByOps  map[string]bool
	// removedIDs are the stored activities earlier operations took out; what
	// they read no longer counts as a use.
	removedIDs map[model.ID]bool
}

// noteRemoved records that target's output is gone, unless the fragment that
// replaces it declares it again.
func (a *alterFlowContext) noteRemoved(target mfmutator.Candidate, replacement *backend.MicroflowFragment) {
	a.removedIDs[target.ID] = true
	v := target.OutputVariable
	if v == "" || fragmentDeclares(replacement, v) {
		return
	}
	a.removedByOps[v] = true
}

// noteFragment records what an inserted or replacing fragment declares and
// reads.
func (a *alterFlowContext) noteFragment(ctx *ExecContext, frag *backend.MicroflowFragment) {
	for _, obj := range frag.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok {
			if v := mfmutator.OutputVariable(act.Action); v != "" {
				a.declaredByOps[v] = true
			}
		}
		text := formatActivity(ctx, obj, a.entityNames, a.microflowNames)
		for _, m := range variableRef.FindAllStringSubmatch(text, -1) {
			a.readByOps[m[1]] = append(a.readByOps[m[1]], text)
		}
	}
}

func fragmentDeclares(frag *backend.MicroflowFragment, v string) bool {
	if frag == nil {
		return false
	}
	for _, obj := range frag.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok && mfmutator.OutputVariable(act.Action) == v {
			return true
		}
	}
	return false
}

func loadAlterFlow(ctx *ExecContext, s *ast.AlterFlowStmt) (*alterFlowContext, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("build hierarchy", err)
	}
	a := &alterFlowContext{stmt: s, entityNames: getEntityNames(ctx, h),
		declaredByOps: map[string]bool{}, readByOps: map[string][]string{}, removedByOps: map[string]bool{},
		removedIDs: map[model.ID]bool{}}
	// A copy: nanoflow names are added below, and the cached map is shared.
	a.microflowNames = map[model.ID]string{}
	for id, n := range getMicroflowNames(ctx, h) {
		a.microflowNames[id] = n
	}
	inModule := func(container model.ID, name string) bool {
		return h.GetModuleName(h.FindModuleID(container)) == s.Name.Module && name == s.Name.Name
	}
	if s.Nanoflow {
		nfs, err := ctx.Backend.ListNanoflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list nanoflows", err)
		}
		for _, nf := range nfs {
			a.microflowNames[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
		}
		nf, ok := pickLive(nfs,
			func(nf *microflows.Nanoflow) bool { return inModule(nf.ContainerID, nf.Name) },
			func(nf *microflows.Nanoflow) bool { return nf.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("nanoflow", s.Name.String())
		}
		a.mf = &microflows.Microflow{
			BaseElement:        nf.BaseElement,
			ContainerID:        nf.ContainerID,
			Name:               nf.Name,
			Parameters:         nf.Parameters,
			ReturnType:         nf.ReturnType,
			ReturnVariableName: nf.ReturnVariableName,
			ObjectCollection:   nf.ObjectCollection,
		}
	} else {
		mfs, err := ctx.Backend.ListMicroflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list microflows", err)
		}
		mf, ok := pickLive(mfs,
			func(mf *microflows.Microflow) bool { return inModule(mf.ContainerID, mf.Name) },
			func(mf *microflows.Microflow) bool { return mf.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("microflow", s.Name.String())
		}
		a.mf = mf
	}
	if a.mf.ObjectCollection == nil {
		return nil, mdlerrors.NewValidation(fmt.Sprintf("%s %s has no flow to alter", s.Kind(), s.Name))
	}
	a.cands, _, _, _ = microflowTargets(ctx, a.mf, a.entityNames, a.microflowNames)
	return a, nil
}

// buildFragment builds a fragment's statements with the builder `create
// microflow` uses, seeded with the variables the stored flow declares, and
// cuts it out of the start and end events the builder wraps it in.
func (a *alterFlowContext) buildFragment(ctx *ExecContext, body []ast.MicroflowStatement) (*backend.MicroflowFragment, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("the fragment is empty; use drop to remove an activity")
	}
	varTypes, declared := a.storedVariables(ctx)
	hierarchy, _ := getHierarchy(ctx)
	restServices, _ := loadRestServices(ctx)
	fb := &flowBuilder{
		textLang:     authoringLanguage(ctx),
		posX:         200,
		posY:         200,
		baseY:        200,
		spacing:      HorizontalSpacing,
		varTypes:     varTypes,
		declaredVars: declared,
		measurer:     &layoutMeasurer{varTypes: varTypes},
		backend:      ctx.Backend,
		hierarchy:    hierarchy,
		restServices: restServices,
		isNanoflow:   a.stmt.Nanoflow,
	}
	oc := fb.buildFlowGraph(body, nil)
	if errs := fb.GetErrors(); len(errs) > 0 {
		return nil, fmt.Errorf("the fragment has errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	if fb.endsWithReturn {
		return nil, fmt.Errorf("the fragment ends the flow with a return, so nothing would lead on to the rest of it; " +
			"a return inside an inserted fragment is not supported yet")
	}
	return cutFragment(oc)
}

// cutFragment removes the builder's start event and final end event, and says
// where the fragment is entered and left. Several paths reaching the end (an
// if without a merge before it, an error handler that rejoins at the end) are
// joined by a merge, which becomes the exit.
func cutFragment(oc *microflows.MicroflowObjectCollection) (*backend.MicroflowFragment, error) {
	var start, end microflows.MicroflowObject
	ends := 0
	for _, obj := range oc.Objects {
		switch obj.(type) {
		case *microflows.StartEvent:
			start = obj
		case *microflows.EndEvent:
			ends++
			end = obj
		}
	}
	if start == nil || end == nil {
		return nil, fmt.Errorf("the fragment does not continue: its last statement ends the flow, so nothing would lead on to the rest of it")
	}
	if ends > 1 {
		return nil, fmt.Errorf("the fragment returns; a return inside an inserted fragment is not supported yet")
	}
	frag := &backend.MicroflowFragment{}
	var intoEnd []*microflows.SequenceFlow
	for _, f := range oc.Flows {
		switch {
		case f.OriginID == start.GetID():
			if frag.Entry != "" {
				return nil, fmt.Errorf("the fragment starts with more than one flow")
			}
			frag.Entry = f.DestinationID
		case f.DestinationID == end.GetID():
			intoEnd = append(intoEnd, f)
		default:
			frag.Flows = append(frag.Flows, f)
		}
	}
	for _, obj := range oc.Objects {
		if obj != start && obj != end {
			frag.Objects = append(frag.Objects, obj)
		}
	}
	frag.AnnotationFlows = oc.AnnotationFlows
	switch {
	case frag.Entry == "" || frag.Entry == end.GetID() || len(frag.Objects) == 0:
		return nil, fmt.Errorf("the fragment builds no activity")
	case len(intoEnd) == 0:
		return nil, fmt.Errorf("no path through the fragment leads on to the rest of the flow")
	case len(intoEnd) == 1:
		frag.Exit = intoEnd[0].OriginID
	default:
		p := end.GetPosition()
		merge := &microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    p,
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		}}
		for _, f := range intoEnd {
			f.DestinationID = merge.ID
			frag.Flows = append(frag.Flows, f)
		}
		frag.Objects = append(frag.Objects, merge)
		frag.Exit = merge.ID
	}
	return frag, nil
}

// storedVariables returns the variables the stored flow declares, in the two
// maps the builder keeps: entity-typed ones with their entity (a change or a
// member access resolves attributes through it), and the rest as declared.
func (a *alterFlowContext) storedVariables(ctx *ExecContext) (varTypes, declared map[string]string) {
	varTypes, declared = map[string]string{}, map[string]string{}
	add := func(name string, dt microflows.DataType) {
		if name == "" {
			return
		}
		t := "Unknown"
		if dt != nil {
			t = formatMicroflowDataType(ctx, dt, a.entityNames)
		}
		switch dt.(type) {
		case *microflows.ObjectType, *microflows.ListType:
			varTypes[name] = t
		default:
			declared[name] = t
		}
	}
	for _, p := range a.mf.Parameters {
		add(p.Name, p.Type)
	}
	for _, c := range a.cands {
		act, ok := c.Object.(*microflows.ActionActivity)
		if !ok || c.OutputVariable == "" {
			continue
		}
		switch x := act.Action.(type) {
		case *microflows.CreateVariableAction:
			add(c.OutputVariable, x.DataType)
		case *microflows.CreateObjectAction:
			if x.EntityQualifiedName != "" {
				varTypes[c.OutputVariable] = x.EntityQualifiedName
			} else if n, ok := a.entityNames[x.EntityID]; ok {
				varTypes[c.OutputVariable] = n
			} else {
				declared[c.OutputVariable] = "Object"
			}
		default:
			declared[c.OutputVariable] = "Unknown"
		}
	}
	return varTypes, declared
}

// systemVariables are in scope everywhere they exist at all; the platform
// reports a misuse (a $latestError outside an error handler) itself.
var systemVariables = map[string]bool{
	"currentUser": true, "currentSession": true, "currentObject": true, "currentDeviceType": true,
	"latestError": true, "latestHttpResponse": true, "latestSoapFault": true,
}

var variableRef = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// checkFragmentScope is plan item 4.2d: the fragment is checked in the scope
// of its insertion point. A variable it declares that the flow already has is
// an error (it would shadow or clash with the stored one); a variable it uses
// that is not declared upstream of where it goes, nor by the fragment itself,
// is an error too, since the fragment would read something that does not exist
// yet on that path.
func (a *alterFlowContext) checkFragmentScope(ctx *ExecContext, op *ast.AlterFlowOperation, target mfmutator.Candidate, frag *backend.MicroflowFragment) error {
	existing := map[string]bool{}
	for _, p := range a.mf.Parameters {
		existing[p.Name] = true
	}
	for _, c := range a.cands {
		if c.OutputVariable != "" {
			existing[c.OutputVariable] = true
		}
	}
	own := map[string]bool{}
	for _, obj := range frag.Objects {
		// A loop's iterator exists only inside the loop, which the fragment
		// brings along; it reads it there, so it is the fragment's own.
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			if src, ok := loop.LoopSource.(*microflows.IterableList); ok && src.VariableName != "" {
				own[src.VariableName] = true
			}
			continue
		}
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		v := mfmutator.OutputVariable(act.Action)
		if v == "" {
			continue
		}
		replacingSame := op.Op == ast.AlterFlowReplace && v == target.OutputVariable
		if existing[v] && !replacingSame {
			return fmt.Errorf("the fragment declares $%s, which the %s already has; choose another name", v, a.stmt.Kind())
		}
		if a.declaredByOps[v] {
			return fmt.Errorf("the fragment declares $%s, which an earlier operation of this alter already declares; choose another name", v)
		}
		own[v] = true
	}

	inScope := map[string]bool{}
	for _, p := range a.mf.Parameters {
		inScope[p.Name] = true
	}
	for id := range a.upstreamOf(target.ID, op.Op == ast.AlterFlowInsertAfter) {
		for _, c := range a.cands {
			if c.ID == id && c.OutputVariable != "" {
				inScope[c.OutputVariable] = true
			}
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, obj := range frag.Objects {
		for _, m := range variableRef.FindAllStringSubmatch(formatActivity(ctx, obj, a.entityNames, a.microflowNames), -1) {
			v := m[1]
			if seen[v] || systemVariables[v] || own[v] || (inScope[v] && !a.removedByOps[v]) {
				continue
			}
			seen[v] = true
			missing = append(missing, "$"+v)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		where := "before " + op.Target
		if op.Op == ast.AlterFlowInsertAfter {
			where = "after " + op.Target
		}
		return fmt.Errorf("the fragment uses %s, which is not declared on the path %s", strings.Join(missing, ", "), where)
	}
	return nil
}

// upstreamOf returns every object from which id can be reached along the
// stored flows — the activities whose outputs exist when the flow gets there.
// id itself is included only when including says so (an insert after it runs
// once it has).
func (a *alterFlowContext) upstreamOf(id model.ID, including bool) map[model.ID]bool {
	preds := map[model.ID][]model.ID{}
	for _, f := range a.mf.ObjectCollection.Flows {
		preds[f.DestinationID] = append(preds[f.DestinationID], f.OriginID)
	}
	out := map[model.ID]bool{}
	queue := append([]model.ID(nil), preds[id]...)
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if out[n] {
			continue
		}
		out[n] = true
		queue = append(queue, preds[n]...)
	}
	if including {
		out[id] = true
	}
	return out
}

// checkOutputUnused refuses to take away an activity whose output variable
// another activity still reads — unless the replacement declares it again.
func (a *alterFlowContext) checkOutputUnused(target mfmutator.Candidate, replacement *backend.MicroflowFragment) error {
	v := target.OutputVariable
	if v == "" || fragmentDeclares(replacement, v) {
		return nil
	}
	if readers := a.readByOps[v]; len(readers) > 0 {
		return fmt.Errorf("$%s is read by what an earlier operation of this alter adds: %s", v, strings.Join(readers, "; "))
	}
	ref := regexp.MustCompile(`\$` + regexp.QuoteMeta(v) + `\b`)
	var users []string
	for _, c := range a.cands {
		if c.ID == target.ID || a.removedIDs[c.ID] {
			continue
		}
		for _, text := range append([]string{c.Statement}, c.Alternates...) {
			if ref.MatchString(text) {
				users = append(users, c.Statement)
				break
			}
		}
	}
	if len(users) > 0 {
		return fmt.Errorf("$%s is still used by: %s", v, strings.Join(users, "; "))
	}
	return nil
}
