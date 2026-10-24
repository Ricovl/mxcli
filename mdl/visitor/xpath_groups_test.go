// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"
)

func TestSplitXPathPredicateGroups(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			// The constraint from mendixlabs/mxcli#772, verbatim: a group with a
			// nested bracket followed by two flat ones, newline-separated.
			name: "issue 772 — nested group plus siblings",
			in: "[Reminders.Task_TaskGroup/Reminders.TaskGroup[EndDate = $EndDateLimit]]\n" +
				"[Status != 'Completed']\n[CompletionDate = empty]",
			want: []string{
				"[Reminders.Task_TaskGroup/Reminders.TaskGroup[EndDate = $EndDateLimit]]",
				"[Status != 'Completed']",
				"[CompletionDate = empty]",
			},
		},
		{
			name: "single group",
			in:   "[Status = 'Open']",
			want: []string{"[Status = 'Open']"},
		},
		{
			name: "adjacent groups, no separator",
			in:   "[a = 1][b = 2]",
			want: []string{"[a = 1]", "[b = 2]"},
		},
		{
			// A "][" split would cut this literal in half.
			name: "bracket inside a string literal",
			in:   "[Name = 'a][b'][Status = 'Open']",
			want: []string{"[Name = 'a][b']", "[Status = 'Open']"},
		},
		{
			name: "doubled quote escape inside a literal",
			in:   "[Name = 'it''s]here'][Age = 3]",
			want: []string{"[Name = 'it''s]here']", "[Age = 3]"},
		},
		{name: "empty", in: "", want: nil},
		{name: "not bracketed", in: "Status = 'Open'", want: nil},
		{name: "unbalanced open", in: "[a = 1", want: nil},
		{name: "unbalanced close", in: "[a = 1]]", want: nil},
		{name: "content between groups", in: "[a = 1] and [b = 2]", want: nil},
		{name: "unterminated literal", in: "[Name = 'x]", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitXPathPredicateGroups(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitXPathPredicateGroups(%q)\n got %#v\nwant %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseXPathConstraint_RejectsPartialParse is the core of #772: the rule matches
// one bracket group, and with the error listeners removed ANTLR parsed the first and
// left the rest on the stream while still reporting success. Callers treated that as
// a full parse and re-rendered only what came back.
func TestParseXPathConstraint_RejectsPartialParse(t *testing.T) {
	multi := "[Status != 'Completed'][CompletionDate = empty]"
	if _, ok := ParseXPathConstraint(multi); ok {
		t.Error("ParseXPathConstraint reported success on a multi-group constraint it only partly consumed")
	}
	// A single group must still parse.
	if _, ok := ParseXPathConstraint("[Status != 'Completed']"); !ok {
		t.Error("ParseXPathConstraint rejected a single well-formed group")
	}
}

// TestXPathPredicateGroupSpans pins that the spans index the string as given, not
// a trimmed copy of it: a caller that edits one group splices it back at Start:End
// and copies everything else verbatim, so an offset that ignored leading
// whitespace would shift every edit (ako/mxcli#1506).
func TestXPathPredicateGroupSpans(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []XPathGroupSpan
	}{
		{name: "single group", in: "[a = 1]", want: []XPathGroupSpan{{0, 7}}},
		{name: "adjacent groups", in: "[a = 1][b = 2]", want: []XPathGroupSpan{{0, 7}, {7, 14}}},
		{
			name: "line breaks between groups and around them",
			in:   "\n  [a = 1]\r\n\t[b = ']['] \n",
			want: []XPathGroupSpan{{3, 10}, {13, 23}},
		},
		{name: "nested group", in: "[A/B[c = 1]] [d = 2]", want: []XPathGroupSpan{{0, 12}, {13, 20}}},
		{name: "empty", in: "", want: nil},
		{name: "blank", in: " \n ", want: nil},
		{name: "not bracketed", in: "a = 1", want: nil},
		{name: "content between groups", in: "[a = 1] or [b = 2]", want: nil},
		{name: "unbalanced", in: "[a = 1", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := XPathPredicateGroupSpans(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("XPathPredicateGroupSpans(%q)\n got %#v\nwant %#v", tc.in, got, tc.want)
			}
			// The spans and the split must agree: same groups, same text.
			groups := SplitXPathPredicateGroups(tc.in)
			if len(groups) != len(got) {
				t.Fatalf("%d spans but %d groups", len(got), len(groups))
			}
			for i, sp := range got {
				if tc.in[sp.Start:sp.End] != groups[i] {
					t.Errorf("span %d is %q, group is %q", i, tc.in[sp.Start:sp.End], groups[i])
				}
			}
		})
	}
}
