// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// A mapping's two message-definition keys are version-dependent: 11.10 added
// MessageDefinition2, 11.15 removed MessageDefinition and moved the source to
// MessageDefinition2 as Module.MessageName (ako/mxcli#987, measured with
// `mx convert` 11.15.0). The writer used to put a MessageDefinition key on
// every mapping, so an 11.15 mapping did not round-trip and a message-definition
// source on 11.15 landed in the key 11.15 no longer reads (CE0270).
func TestMappingMessageDefinitionKeys(t *testing.T) {
	pv := func(major, minor int) *types.ProjectVersion {
		return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor}
	}
	strp := func(s string) *string { return &s }
	show := func(p *string) string {
		if p == nil {
			return "<absent>"
		}
		return "\"" + *p + "\""
	}
	cases := []struct {
		name             string
		pv               *types.ProjectVersion
		update           bool
		storedMD, stored *string
		isMD             bool
		ref              string
		wantMD, wantMD2  *string
	}{
		// Creates: the version decides the key set.
		{name: "create 10.24", pv: pv(10, 24), wantMD: strp("")},
		{name: "create 11.14", pv: pv(11, 14), wantMD: strp(""), wantMD2: strp("")},
		{name: "create 11.15", pv: pv(11, 15), wantMD2: strp("")},
		{name: "create unknown version", wantMD: strp("")},
		{name: "create 11.14 with source", pv: pv(11, 14), isMD: true, ref: "M.C.D",
			wantMD: strp("M.C.D"), wantMD2: strp("")},
		{name: "create 11.15 with source", pv: pv(11, 15), isMD: true, ref: "M.Msg",
			wantMD2: strp("M.Msg")},
		// Updates: the stored document's key set is carried.
		{name: "update 11.15 shape", pv: pv(11, 15), update: true, stored: strp(""),
			wantMD2: strp("")},
		{name: "update 11.15 shape with source", pv: pv(11, 15), update: true, stored: strp("M.Old"),
			isMD: true, ref: "M.Msg", wantMD2: strp("M.Msg")},
		{name: "update clears a source the statement drops", pv: pv(11, 15), update: true,
			stored: strp("M.Old"), wantMD2: strp("")},
		{name: "update pre-11.10 shape on 11.14", pv: pv(11, 14), update: true, storedMD: strp("M.C.D"),
			isMD: true, ref: "M.C.D", wantMD: strp("M.C.D")},
		// A pre-11.15 document in an 11.15 project keeps its own shape.
		{name: "update 11.14 shape on 11.15", pv: pv(11, 15), update: true,
			storedMD: strp("M.C.D"), stored: strp(""), isMD: true, ref: "M.C.D",
			wantMD: strp("M.C.D"), wantMD2: strp("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md, md2 := mappingMessageDefinitionKeys(tc.pv, tc.update, tc.storedMD, tc.stored, tc.isMD, tc.ref)
			if show(md) != show(tc.wantMD) {
				t.Errorf("MessageDefinition = %s, want %s", show(md), show(tc.wantMD))
			}
			if show(md2) != show(tc.wantMD2) {
				t.Errorf("MessageDefinition2 = %s, want %s", show(md2), show(tc.wantMD2))
			}
		})
	}
}
