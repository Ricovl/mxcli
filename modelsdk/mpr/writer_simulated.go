// SPDX-License-Identifier: Apache-2.0

package mpr

import "errors"

// Simulated writes: a run whose unit updates are held in memory and read back,
// exactly as in a deferred run (writer_deferred.go), but are never written.
//
// `check` uses it to predict a script's statements in order (mendixlabs/mxcli#1517):
// a flow change it predicts as accepted is applied to the simulation, so the
// next statement on the same flow is judged against what exec would see there,
// not against the flow as stored. Without it check gave up on that statement
// ("not predicted") and exec could refuse it after the earlier statements had
// run.
//
// Only an update can be simulated: it is all a later read needs to see. Every
// other write — an insert, a delete, a move, a contents-hash repair — passes
// guardWrite before it touches anything, and is refused there while a
// simulation is open, so a simulation cannot reach storage by any path.

// ErrSimulatedWrite is returned for a write a simulation cannot hold.
var ErrSimulatedWrite = errors.New("this write cannot be simulated: only unit updates are held while writes are simulated, and nothing reaches storage")

// SimulateWrites opens a simulation and returns the function that ends it,
// discarding everything held. It does not open one — ok is false — while a
// deferred run or another simulation is open, since its held writes would mix
// with real ones.
func (w *Writer) SimulateWrites() (discard func(), ok bool) {
	if w.simulating || w.deferred != nil {
		return func() {}, false
	}
	w.simulating = true
	w.DeferUnitWrites()
	return func() {
		for id := range w.deferred {
			w.reader.ClearOverlay(id)
		}
		w.deferred, w.deferredOrder = nil, nil
		w.simulating = false
	}, true
}
