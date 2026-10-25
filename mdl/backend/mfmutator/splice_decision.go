// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// # Taking out a decision (ako/mxcli#1694)
//
// `alter … insert` puts a decision into a flow — `if … then … [else …] end if`
// as a fragment — and the script that created the flow, run again, has to take
// it out. The splice takes out exactly that shape: an exclusive split whose two
// paths hold plain activities (or nothing) and rejoin at one merge, which leads
// on to one node. Dropping it joins the flows that entered the split to the
// node after the merge; replacing it puts a fragment where the split began.
// Either way the split, the activities on its paths, the merge and the flows
// between them go, and every other element keeps its $ID.
//
// Any other decision — a path that returns, raises an error, holds a decision
// or a loop, an activity on a path with an error handler, a path from
// elsewhere joining one of its paths or its merge — is a change of where the
// flow's paths end or meet, and is refused with the part that is out of reach.

// decisionShape is a decision of the shape `alter … insert` produces.
type decisionShape struct {
	split, merge *node
	// inner are the activities on the decision's paths, in path order.
	inner []*node
	// in are the flows that enter the split; out the one that leaves the
	// merge.
	in  []flowRef
	out flowRef
	// flows are the flows from the split to the merge, on both paths.
	flows map[string]bool
}

func (sh *decisionShape) nodes() []*node {
	out := append([]*node{sh.split}, sh.inner...)
	return append(out, sh.merge)
}

// insertedDecision checks that x is a decision of the shape `alter … insert`
// produces, and returns it. verb says what is being done to it, for the
// refusal.
func insertedDecision(g *graph, x *node, verb string) (*decisionShape, error) {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("cannot %s %s: %s; the splice takes out a decision only of the shape `alter … insert` "+
			"puts in — two paths of plain activities, or none, that rejoin at one merge (ako/mxcli#1694)",
			verb, describeNode(x), fmt.Sprintf(format, args...))
	}
	switch x.typ {
	case "Microflows$ExclusiveSplit":
	case "Microflows$InheritanceSplit":
		return nil, refuse("it splits by type, not on a condition")
	default:
		return nil, refuse("it is not a decision")
	}
	normal, errs := g.outgoing(x.id)
	if len(errs) > 0 {
		return nil, refuse("it has an error handler")
	}
	if len(normal) != 2 {
		return nil, refuse("it has %d paths, not two", len(normal))
	}
	sh := &decisionShape{split: x, flows: map[string]bool{}}
	for _, f := range normal {
		label := caseLabel(f)
		sh.flows[f.id] = true
		n := g.nodes[f.dest]
		for n != nil && n.typ == "Microflows$ActionActivity" {
			if in := g.incoming(n.id); len(in) != 1 {
				return nil, refuse("%d flows enter %s on its %s path, so a path from elsewhere joins it there",
					len(in), describeNode(n), label)
			}
			out, errs := g.outgoing(n.id)
			if len(errs) > 0 {
				return nil, refuse("%s on its %s path has an error handler", describeNode(n), label)
			}
			if len(out) != 1 {
				return nil, refuse("%s on its %s path has %d outgoing flows, not one", describeNode(n), label, len(out))
			}
			sh.inner = append(sh.inner, n)
			sh.flows[out[0].id] = true
			n = g.nodes[out[0].dest]
		}
		if n == nil {
			return nil, refuse("its %s path leads to an object that is not in the flow", label)
		}
		switch n.typ {
		case "Microflows$ExclusiveMerge":
		case "Microflows$EndEvent":
			return nil, refuse("its %s path ends in a return", label)
		case "Microflows$ErrorEvent":
			return nil, refuse("its %s path ends in a raise error", label)
		case "Microflows$ExclusiveSplit", "Microflows$InheritanceSplit":
			return nil, refuse("its %s path holds a decision (%s)", label, describeNode(n))
		case "Microflows$LoopedActivity":
			return nil, refuse("its %s path holds a loop (%s)", label, describeNode(n))
		case "Microflows$BreakEvent", "Microflows$ContinueEvent":
			return nil, refuse("its %s path ends in a %s", label, describeNode(n))
		default:
			return nil, refuse("its %s path reaches %s", label, describeNode(n))
		}
		if sh.merge != nil && sh.merge != n {
			return nil, refuse("its paths do not rejoin at one merge")
		}
		sh.merge = n
	}
	if in := g.incoming(sh.merge.id); len(in) != 2 {
		return nil, refuse("%d flows join at its merge, %s, not its two paths alone", len(in), describeNode(sh.merge))
	}
	out, errs := g.outgoing(sh.merge.id)
	if len(out) != 1 || len(errs) > 0 {
		return nil, refuse("its merge, %s, does not lead on to one node", describeNode(sh.merge))
	}
	sh.out = out[0]
	sh.in = g.incoming(x.id)
	if len(sh.in) == 0 {
		return nil, refuse("no flow enters it")
	}
	return sh, nil
}

// caseLabel is how a decision's path is named in a message: the case value
// its flow carries ("true", "false"), or "one" when it carries none.
func caseLabel(f flowRef) string {
	for _, el := range arrayElements(dGet(f.doc, "CaseValues")) {
		if d, ok := el.(bson.D); ok {
			if v := dString(d, "Value"); v != "" {
				return v
			}
		}
	}
	if d := dDoc(f.doc, "NewCaseValue"); d != nil {
		if v := dString(d, "Value"); v != "" {
			return v
		}
	}
	return "one"
}

// dropDecision takes the decision x out, when it has the shape `alter …
// insert` produces: the flows that entered the split now enter the node the
// merge led to, with the end that flow had there; the split, the activities
// on its paths, the merge and the flows between them go, and so do the notes
// on its paths (pathNotes). The split's own note lines go and the notes stay,
// as for a dropped activity. Nothing else moves: the gap stays, as it does
// for a drop.
func (m *Mutator) dropDecision(g *graph, x *node) error {
	sh, err := insertedDecision(g, x, "drop")
	if err != nil {
		return err
	}
	for _, in := range sh.in {
		if in.origin == sh.out.dest {
			return fmt.Errorf("dropping %s would leave a flow from an object to itself", describeNode(x))
		}
	}
	destIdx, destVec := flowEnd(sh.out.doc, "Destination")
	destPtr := dGet(sh.out.doc, "DestinationPointer")
	for _, in := range sh.in {
		setBinary(in.doc, "DestinationPointer", destPtr)
		setInt(in.doc, "DestinationConnectionIndex", destIdx)
		setVector(in.doc, "DestinationControlVector", destVec)
	}
	drop := map[string]bool{sh.out.id: true}
	for id := range sh.flows {
		drop[id] = true
	}
	for _, af := range g.annotationFlows(x.id) {
		drop[af.id] = true
	}
	notes := pathNotes(g, sh, drop)
	m.removeFlows(drop)
	for _, n := range append(sh.nodes(), notes...) {
		if err := m.removeObject(n); err != nil {
			return err
		}
	}
	return nil
}

// pathNotes adds to drop the lines of the notes attached to the activities on
// the decision's paths and to its merge, and returns the notes attached to
// nothing else: those go with the decision, which takes what is on its paths
// out wholesale — a note left behind would be a free note on the canvas about
// an activity that is gone. A note also attached to a node that stays keeps
// that line, and stays. The split's own notes are the caller's: they follow
// the decision as a dropped or replaced activity's do.
func pathNotes(g *graph, sh *decisionShape, drop map[string]bool) []*node {
	gone := map[string]bool{}
	for _, n := range append(append([]*node(nil), sh.inner...), sh.merge) {
		gone[n.id] = true
	}
	var notes []*node
	seen := map[string]bool{}
	for _, n := range append(append([]*node(nil), sh.inner...), sh.merge) {
		for _, af := range g.annotationFlows(n.id) {
			drop[af.id] = true
			other := af.origin
			if other == n.id {
				other = af.dest
			}
			note := g.nodes[other]
			if note == nil || note.typ != "Microflows$Annotation" || seen[note.id] {
				continue
			}
			seen[note.id] = true
			alone := true
			for _, f := range g.annotationFlows(note.id) {
				end := f.origin
				if end == note.id {
					end = f.dest
				}
				if !gone[end] {
					alone = false
				}
			}
			if alone {
				notes = append(notes, note)
			}
		}
	}
	return notes
}

// replaceDecision puts frag where the decision x is, when it has the shape
// `alter … insert` produces: the flows that entered the split enter the
// fragment, the flow that left the merge leaves the fragment's exit, and the
// notes on the split are attached to the fragment's entry, as for a replaced
// activity. The fragment starts where the split started; when it is longer
// than the decision was, what is past the merge moves along.
func (m *Mutator) replaceDecision(g *graph, x *node, frag *backend.MicroflowFragment) error {
	sh, err := insertedDecision(g, x, "replace")
	if err != nil {
		return err
	}
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	ax, s, err := flowAxis(sh.split, sh.merge)
	if err != nil {
		return err
	}
	near := sh.split.pos.get(ax) - s*sh.split.size.get(ax)/2
	far := sh.merge.pos.get(ax) + s*sh.merge.size.get(ax)/2
	span := s * (far - near)
	cross := sh.split.pos.get(ax.other())
	mergeAt := sh.merge.pos.get(ax)

	// The decision goes first, so the room the fragment needs is judged on
	// the flow without it. The flows into it and out of it stay, re-pointed
	// below; the split's note lines go to the fragment's entry.
	drop := map[string]bool{}
	for id := range sh.flows {
		drop[id] = true
	}
	notes := pathNotes(g, sh, drop)
	m.removeFlows(drop)
	for _, n := range append(sh.nodes(), notes...) {
		if err := m.removeObject(n); err != nil {
			return err
		}
	}
	if !frag.Placed {
		g2 := m.graph()
		if extra := fb.length(ax) - span; extra > 0 {
			cut := float64(mergeAt)
			if p := m.pinnedPast(g2, "", ax, s, cut, ""); p != nil {
				return fmt.Errorf("the replacing statements are %dpx longer than %s was, and making room for them would "+
					"move %s, which the script draws where it is; %s", extra, describeNode(x), describeNode(p), roomAdvice)
			}
			m.shiftIn(g2, "", ax, s, cut, s*extra, "")
		}
		centre := point{}
		centre = centre.set(ax, near+s*fb.length(ax)/2)
		centre = centre.set(ax.other(), cross)
		fb.placeCentred(ax, centre)
		if err := fb.checkRoom(g2, "", ""); err != nil {
			return err
		}
	}
	if err := m.addFragment(frag); err != nil {
		return err
	}
	for _, in := range sh.in {
		setPointer(in.doc, "DestinationPointer", frag.Entry)
	}
	setPointer(sh.out.doc, "OriginPointer", frag.Exit)
	if frag.ExitCase != nil {
		if err := m.setCase(sh.out.doc, frag.ExitCase); err != nil {
			return err
		}
	}
	for _, af := range g.annotationFlows(x.id) {
		key := "DestinationPointer"
		if af.origin == x.id {
			key = "OriginPointer"
		}
		setPointer(af.doc, key, frag.Entry)
	}
	return nil
}

// DecisionNodes returns the stored activities on the paths of the decision
// target, when it has the shape `alter … insert` produces — what taking it out
// takes with it, besides the split and its merge — and nil otherwise.
func (m *Mutator) DecisionNodes(target model.ID) []model.ID {
	g := m.graph()
	x, ok := g.nodes[string(target)]
	if !ok {
		return nil
	}
	sh, err := insertedDecision(g, x, "take out")
	if err != nil {
		return nil
	}
	var out []model.ID
	for _, n := range sh.inner {
		out = append(out, model.ID(n.id))
	}
	return out
}
