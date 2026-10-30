// SPDX-License-Identifier: Apache-2.0

package mprread

import "sync/atomic"

// unitsDecoded counts the units decoded into typed elements, process-wide.
// Decoding is most of what a document listing costs, so a test can hold a
// pass to a budget by counting it (ako/mxcli#1804: an incremental catalog
// build decodes the units it rebuilds, not the project).
var unitsDecoded atomic.Int64

// UnitsDecoded returns how many units have been decoded so far.
func UnitsDecoded() int64 {
	return unitsDecoded.Load()
}
