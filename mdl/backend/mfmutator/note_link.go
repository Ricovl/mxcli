// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// LinkNote draws a line from the stored note to object, both in the flow — the
// object may be one a fragment put there earlier — unless one is drawn
// between them already (a replace carries the replaced activity's lines to its
// replacement). Nothing else changes: the note keeps its $ID, text and place.
//
// The line attaches at the sides that face each other where the two are drawn
// NOW, after the fragment was placed and the flow moved to make room for it —
// which is why it is drawn here and not by the builder, which never knew where
// a stored note is relative to what it builds (ako/mxcli#1695).
func (m *Mutator) LinkNote(note, object model.ID) error {
	g := m.graph()
	n, ok := g.nodes[string(note)]
	if !ok || n.typ != "Microflows$Annotation" {
		return fmt.Errorf("note %s is not in the flow", note)
	}
	o, ok := g.nodes[string(object)]
	if !ok {
		return fmt.Errorf("object %s is not in the flow", object)
	}
	for _, f := range g.annotationFlows(n.id) {
		if (f.origin == n.id && f.dest == o.id) || (f.origin == o.id && f.dest == n.id) {
			return nil
		}
	}
	sides := microflows.FacingSides(g.canvasBox(n), g.canvasBox(o))
	return m.addFragment(&backend.MicroflowFragment{AnnotationFlows: []*microflows.AnnotationFlow{{
		BaseElement:   model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:      note,
		DestinationID: object,
		Sides:         &sides,
	}}})
}

// canvasBox is n's box in absolute canvas coordinates: a loop body's objects
// are drawn relative to the loop box's top-left corner, and a note outside a
// loop may be attached to an object inside it.
func (g *graph) canvasBox(n *node) microflows.CanvasBox {
	x, y := n.pos.X, n.pos.Y
	for l := g.nodes[n.loop]; l != nil; l = g.nodes[l.loop] {
		x += l.pos.X - l.size.X/2
		y += l.pos.Y - l.size.Y/2
	}
	return microflows.CanvasBox{Left: x - n.size.X/2, Top: y - n.size.Y/2, Right: x + n.size.X/2, Bottom: y + n.size.Y/2}
}
