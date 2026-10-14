// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

func TestLookupSpellings(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		want  string
		exact bool
	}{
		// The three spellings of the example `mxcli syntax` advertises.
		{"separate arguments", []string{"workflow", "user-task", "targeting"}, "workflow.user-task.targeting", true},
		{"one quoted argument", []string{"workflow user-task targeting"}, "workflow.user-task.targeting", true},
		{"dotted path", []string{"workflow.user-task.targeting"}, "workflow.user-task.targeting", true},
		// Words instead of hyphens — what the REPL's `help` has always taken.
		{"words", []string{"workflow", "user", "task"}, "workflow.user-task", true},
		{"one quoted argument, words", []string{"workflow user task targeting"}, "workflow.user-task.targeting", true},
		{"security prefix", []string{"security entity access"}, "security.entity-access", true},
		{"domain model", []string{"domain model"}, "domain-model", true},
		// Aliases still resolve, and still only as whole paths.
		{"legacy alias", []string{"entity"}, "domain-model.entity", true},
		// A leaf name that is nobody's first segment (#955).
		{"segment match", []string{"parallel-split"}, "parallel-split", false},
		// A category word in front of a topic that lives elsewhere (#1389).
		{"qualified topic", []string{"integration", "message-definition"}, "message-definition", true},
		// ...but not when the qualifier has its own topic for the word: under
		// workflow, "microflow" means workflow.call-microflow, and the segment
		// match that finds it must not be overridden by the top-level page.
		{"qualifier with its own match", []string{"workflow", "microflow"}, "workflow.microflow", false},
		{"unknown", []string{"nonexistent-topic"}, "nonexistent-topic", false},
		{"no words", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Lookup(tt.args)
			if m.Path != tt.want {
				t.Errorf("Lookup(%q).Path = %q, want %q", tt.args, m.Path, tt.want)
			}
			if m.Exact != tt.exact {
				t.Errorf("Lookup(%q).Exact = %v, want %v", tt.args, m.Exact, tt.exact)
			}
			if tt.exact && len(m.Features) == 0 {
				t.Errorf("Lookup(%q) resolved to %q but returned no features", tt.args, m.Path)
			}
		})
	}
}

// Whatever a topic's listed path is, all three ways of typing it must reach it:
// as separate arguments, as one string, and dotted. `mxcli syntax` prints the
// dotted paths and then tells the reader to drill down with the words, so a
// spelling that does not resolve is the command contradicting its own output
// (mendixlabs/mxcli#1025).
func TestEveryRegisteredPathIsReachableBySpelling(t *testing.T) {
	for _, f := range All() {
		segments := strings.Split(f.Path, ".")
		spellings := map[string][]string{
			"dotted":             {f.Path},
			"separate arguments": segments,
			"one argument":       {strings.Join(segments, " ")},
		}
		for name, args := range spellings {
			m := Lookup(args)
			if !m.Exact || m.Path != f.Path {
				t.Errorf("%s (%s): Lookup(%q) = %q exact=%v, want %q exact=true",
					f.Path, name, args, m.Path, m.Exact, f.Path)
			}
		}
	}
}

// Aliases are whole-path spellings, and every one must land on a real topic.
func TestAliasesResolveToRegisteredTopics(t *testing.T) {
	for alias := range topicAliases {
		if m := Lookup([]string{alias}); !m.Exact {
			t.Errorf("alias %q resolves to %q, which names no topic", alias, m.Path)
		}
	}
}

// A verb is how a statement is looked up, and no path starts with one; the
// topics list their verbs as keywords (#1389).
func TestLookupByKeyword(t *testing.T) {
	m := Lookup([]string{"grant"})
	if len(m.Features) == 0 {
		t.Fatalf("Lookup(grant) found nothing")
	}
	if m.Exact || m.Via != "keyword" || m.Fallback != "grant" {
		t.Errorf("Lookup(grant) = exact=%v via=%q fallback=%q, want a keyword fallback on grant",
			m.Exact, m.Via, m.Fallback)
	}
	found := false
	for _, f := range m.Features {
		if f.Path == "security.entity-access" {
			found = true
		}
	}
	if !found {
		t.Errorf("Lookup(grant) does not include security.entity-access")
	}
	// A topic listing the word itself ranks ahead of one listing it in a phrase.
	if !hasKeyword(m.Features[0], "grant") {
		t.Errorf("Lookup(grant) ranks %s first, which does not list grant as a keyword", m.Features[0].Path)
	}
	// Whole words, not substrings: "ant" is not "grant".
	for _, f := range ByKeyword("ant") {
		t.Errorf("ByKeyword(ant) matched %s by substring", f.Path)
	}
}

func hasKeyword(f SyntaxFeature, k string) bool {
	for _, kw := range f.Keywords {
		if strings.EqualFold(kw, k) {
			return true
		}
	}
	return false
}
