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
	// SetReturnValue sets the expression the end event target returns, in
	// place; "" is no value.
	SetReturnValue(target model.ID, value string) error
	// Save writes the patched unit.
	Save() error
}

// MicroflowMutationBackend opens a microflow or nanoflow for splicing.
type MicroflowMutationBackend interface {
	// OpenMicroflowForMutation loads a Microflows$Microflow or
	// Microflows$Nanoflow unit and returns a mutator over its stored form.
	OpenMicroflowForMutation(unitID model.ID) (MicroflowMutator, error)
}
