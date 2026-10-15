// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"encoding/binary"
	"math"
	"testing"
)

// CVE-2026-107324 / CVE-2026-107325: go.mongodb.org/mongo-driver/v2 before
// v2.9.2 panics inside bson.Raw.Validate and bson.RawArray.Validate instead of
// returning an error, on a binary element declaring a length of math.MaxInt32
// and on a zero-length array respectively.
//
// rawUnitName calls raw.Validate() defensively — it expects an error for bytes
// that are not a valid document and reports ok=false. A panicking Validate
// turns a malformed unit in a .mpr into a crash of the whole CLI, so these
// bytes are reachable from any project file mxcli is pointed at.
//
// Control: against v2.6.0 (the version a customer's release scan flagged) the
// first subtest fails with
// "runtime error: slice bounds out of range [:-2147483641]"; against v2.9.2 it
// passes. The test therefore detects a downgrade of the driver, not just the
// absence of a crash.
func TestRawUnitName_MalformedBSONDoesNotPanic(t *testing.T) {
	// A document holding one binary element whose declared length is
	// math.MaxInt32, which overflowed the driver's remaining-bytes arithmetic.
	maxInt32Binary := func() []byte {
		doc := []byte{0, 0, 0, 0} // total length, filled in below
		doc = append(doc, 0x05)   // element type: binary
		doc = append(doc, 'a', 0) // element key: "a"
		l := make([]byte, 4)
		binary.LittleEndian.PutUint32(l, uint32(math.MaxInt32))
		doc = append(doc, l...) // declared payload length
		doc = append(doc, 0x00) // binary subtype
		doc = append(doc, 0x00) // document terminator
		binary.LittleEndian.PutUint32(doc[0:4], uint32(len(doc)))
		return doc
	}()

	cases := []struct {
		name     string
		contents []byte
	}{
		{"binary element declaring math.MaxInt32 (CVE-2026-107324)", maxInt32Binary},
		{"empty contents (CVE-2026-107325)", []byte{}},
		{"truncated length prefix", []byte{0xFF, 0xFF, 0xFF, 0x7F}},
		{"length prefix larger than the buffer", []byte{0x40, 0x00, 0x00, 0x00, 0x00}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("rawUnitName panicked on malformed BSON: %v\n"+
						"this is CVE-2026-107324/107325 — check the "+
						"go.mongodb.org/mongo-driver/v2 version in go.mod "+
						"(v2.9.2 or later is required)", r)
				}
			}()

			name, ok := rawUnitName(tc.contents)
			if ok {
				t.Errorf("rawUnitName accepted malformed BSON as a valid document (name=%q)", name)
			}
			if name != "" {
				t.Errorf("rawUnitName returned name %q for malformed BSON, want empty", name)
			}
		})
	}
}
