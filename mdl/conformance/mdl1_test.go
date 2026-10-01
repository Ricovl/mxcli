// SPDX-License-Identifier: Apache-2.0

package conformance

import (
	"reflect"
	"testing"
)

// Each case pairs a block CheckMDL1 must report with its control: the same
// block written as the skills now teach it, which must not be reported.
func TestCheckMDL1_FindingsAgainstControls(t *testing.T) {
	for _, tc := range []struct {
		name, bad, good, class string
	}{
		{"script without header",
			"create persistent entity M.E (Name: string(100));",
			"mdl 1;\ncreate persistent entity M.E (Name: string(100));", ClassMissingHeader},
		{"slash terminator",
			"mdl 1;\ncreate persistent entity M.E (Name: string(100))\n/",
			"mdl 1;\ncreate persistent entity M.E (Name: string(100));", ClassNotMDL1},
		{"deprecated spelling mdl 1 refuses",
			"mdl 1;\ncreate persistent entity M.E (D: date);",
			"mdl 1;\ncreate persistent entity M.E (D: datetime);", ClassNotMDL1},
		{"query without terminator",
			"list entities in M",
			"list entities in M;", ClassNotMDL1},
		{"microflow fragment with an mdl 0 meaning",
			"retrieve $P from M.Product where Id = 1 limit 1;",
			"retrieve $P from M.Product where Id = 1 first;", ClassNotMDL1},
		{"session command in a script",
			"mdl 1;\ncreate persistent entity M.E (Name: string(100));\ndisconnect;",
			"mdl 1;\ncreate persistent entity M.E (Name: string(100));", ClassNotMDL1},
		{"header naming mdl 0",
			"mdl 0;\ncreate persistent entity M.E (Name: string(100));",
			"mdl 1;\ncreate persistent entity M.E (Name: string(100));", ClassNotMDL1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classes(CheckMDL1(Unit{Source: "s.md", Line: 1, Text: tc.bad}))
			if !reflect.DeepEqual(got, []string{tc.class}) {
				t.Errorf("%q: findings %v, want [%s]", tc.bad, got, tc.class)
			}
			if got := CheckMDL1(Unit{Source: "s.md", Line: 1, Text: tc.good}); len(got) != 0 {
				t.Errorf("control %q: findings %v, want none", tc.good, got)
			}
		})
	}
}

// What carries no header: a read-only command, a fragment, and a template.
func TestCheckMDL1_NoHeaderNeeded(t *testing.T) {
	for _, text := range []string{
		"list entities in M;\ndescribe entity M.E;", // commands for the REPL / -c
		"$H = head $L;", // a microflow activity
		"actionbutton b (Caption: 'Out', Action: sign out)", // a page widget
		"create entity <Module>.<Name> (...);",              // a template
		"-- only a comment",                                 // nothing
		"mxcli check script.mdl\nlist entities in M;",       // a shell line next to a command
	} {
		if got := CheckMDL1(Unit{Source: "s.md", Line: 1, Text: text}); len(got) != 0 {
			t.Errorf("%q: findings %v, want none", text, got)
		}
	}
}

// A shipped .mdl file is a script whatever it holds.
func TestCheckMDL1_ScriptUnitNeedsHeader(t *testing.T) {
	got := classes(CheckMDL1(Unit{Source: "x.mdl", Line: 1, Text: "list entities;", Script: true}))
	if !reflect.DeepEqual(got, []string{ClassMissingHeader}) {
		t.Fatalf("got %v", got)
	}
	if got := CheckMDL1(Unit{Source: "x.mdl", Line: 1, Text: "mdl 1;\nlist entities;", Script: true}); len(got) != 0 {
		t.Fatalf("control: %v", got)
	}
}

// A template does not hide the real statement next to it.
func TestCheckMDL1_ChunkNextToTemplate(t *testing.T) {
	block := "create entity <Module>.<Name> (...);\n\nlist entities in M"
	got := CheckMDL1(Unit{Source: "s.md", Line: 10, Text: block})
	if len(got) != 1 || got[0].Class != ClassNotMDL1 || got[0].Line != 12 {
		t.Fatalf("got %v, want one mdl1 finding on line 12", got)
	}
}
