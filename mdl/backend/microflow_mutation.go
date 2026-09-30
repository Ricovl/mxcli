// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// MicroflowFragment is what an `alter microflow` insert or replace splices in:
// the objects and flows a fragment of MDL builds to, written exactly as
// `create microflow` writes the same statements, and the two objects the
// surrounding flow connects to. Entry is where the flow into the fragment
// ends; Exit is where the flow out of it starts. For a one-activity fragment
// they are the same object.
//
// Positions are the builder's own; the mutator translates the fragment into
// place (plan item 4.2c) before writing it.
type MicroflowFragment struct {
	Objects         []microflows.MicroflowObject
	Flows           []*microflows.SequenceFlow
	AnnotationFlows []*microflows.AnnotationFlow
	Entry, Exit     model.ID
	// Placed says the script stated where the fragment goes (an @position on
	// its first statement): it is written where the builder put it, and
	// nothing around it is moved to make room (ako/mxcli#818). Unset, the
	// mutator translates it into the gap it is spliced into.
	Placed bool
}

// MicroflowMutator splices into one stored microflow or nanoflow (ADR-0012
// decision 3). Targets are activity IDs, already resolved from their content
// address by mfmutator.Resolve. Every operation edits the stored document in
// place: untouched elements stay byte-identical, and nothing is rebuilt.
// Call Save to persist.
type MicroflowMutator interface {
	// InsertAfter splices frag onto the one flow that leaves target.
	InsertAfter(target model.ID, frag *MicroflowFragment) error
	// InsertBefore splices frag onto the one flow that enters target.
	InsertBefore(target model.ID, frag *MicroflowFragment) error
	// Replace puts frag in target's place and removes target.
	Replace(target model.ID, frag *MicroflowFragment) error
	// Drop removes target and joins its incoming flows to its successor.
	Drop(target model.ID) error
	// RemoveNotes removes the annotations attached to target, with their
	// lines, so that a Replace or Drop after it does not keep them
	// (ako/mxcli#859: a `create or modify` states an activity's notes). A
	// note also attached to another object is refused.
	RemoveNotes(target model.ID) error
	// SetReturnValue sets the expression the end event target returns, in
	// place; "" is no value.
	SetReturnValue(target model.ID, value string) error
	// Move sets where the top-level node target is drawn, and nothing else:
	// its flows keep their ends, sides and curves (ako/mxcli#818).
	Move(target model.ID, to model.Point) error
	// SetHeader writes the document properties of declared — a
	// *microflows.Microflow or *microflows.Nanoflow built from the statement,
	// with every property it does not state carried from the stored one — onto
	// the stored document: the header (return type, URL, export level,
	// concurrency, documentation, …) and the parameters (added, retyped or
	// removed). Nothing else is touched, and an unchanged property is not
	// rewritten. It returns what it changed, for the report.
	SetHeader(declared any) ([]string, error)
	// Save writes the patched unit.
	Save() error
}

// MicroflowMutationBackend opens a microflow or nanoflow for splicing.
type MicroflowMutationBackend interface {
	// OpenMicroflowForMutation loads a Microflows$Microflow or
	// Microflows$Nanoflow unit and returns a mutator over its stored form.
	OpenMicroflowForMutation(unitID model.ID) (MicroflowMutator, error)
}
