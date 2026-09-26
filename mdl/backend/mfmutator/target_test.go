// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The fixture mirrors FeedbackModule.VAL_Feedback's shape: one output
// variable, captioned splits, and a statement (`set $ValidFeedback = false`)
// that occurs three times, which is the case the resolver must refuse to guess.
func feedbackCandidates() []Candidate {
	mk := func(id string, obj microflows.MicroflowObject, stmt string) Candidate {
		c, ok := NewCandidate(obj, func(microflows.MicroflowObject) string { return stmt })
		if !ok {
			panic("not addressable: " + id)
		}
		return c
	}
	act := func(id string, x int, action microflows.MicroflowAction) *microflows.ActionActivity {
		a := &microflows.ActionActivity{Action: action}
		a.ID = model.ID(id)
		a.Position = model.Point{X: x, Y: 200}
		a.AutoGenerateCaption = true
		return a
	}
	split := func(id, caption string) *microflows.ExclusiveSplit {
		s := &microflows.ExclusiveSplit{Caption: caption}
		s.ID = model.ID(id)
		return s
	}
	end := &microflows.EndEvent{ReturnValue: "$ValidFeedback"}
	end.ID = "end"

	return []Candidate{
		mk("declare", act("declare", -390, &microflows.CreateVariableAction{VariableName: "ValidFeedback"}), "declare $ValidFeedback Boolean = true;"),
		mk("s1", split("s1", "Subject not empty?"), "if trim($Feedback/Subject) != empty and\ntrim($Feedback/Subject) != '' then"),
		mk("vf1", act("vf1", -40, &microflows.ValidationFeedbackAction{}), "validation feedback $Feedback/Subject message 'Subject is too long';"),
		mk("set1", act("set1", 135, &microflows.ChangeVariableAction{}), "set $ValidFeedback = false;"),
		mk("set2", act("set2", 420, &microflows.ChangeVariableAction{}), "set $ValidFeedback = false;"),
		mk("java", act("java", 980, &microflows.JavaActionCallAction{UseReturnVariable: true, ResultVariableName: "IsValidEmail"}),
			"$IsValidEmail = call java action FeedbackModule.ValidateEmail(EmailAddress = $Feedback/SubmitterEmail);"),
		mk("s2", split("s2", "Email is Valid?"), "if not($IsValidEmail) then"),
		mk("vf2", act("vf2", 1155, &microflows.ValidationFeedbackAction{}), "validation feedback $Feedback/SubmitterEmail message 'Email is not valid';"),
		mk("set3", act("set3", 1305, &microflows.ChangeVariableAction{}), "set $ValidFeedback = false;"),
		mk("vf3", act("vf3", 770, &microflows.ValidationFeedbackAction{}), "validation feedback $Feedback/SubmitterEmail message 'Email is required';"),
		mk("end", end, "return $ValidFeedback;"),
	}
}

func TestResolve_EachAddressForm(t *testing.T) {
	cands := feedbackCandidates()
	cases := []struct {
		target string
		want   model.ID
	}{
		{"$IsValidEmail", "java"},
		{"$ValidFeedback", "declare"},
		{"'Email is Valid?'", "s2"},
		{"if not($IsValidEmail) then", "s2"},
		{"set $ValidFeedback = false @2", "set2"},
		{"set $ValidFeedback=false@3", "set3"},
		{"validation feedback $Feedback/SubmitterEmail * 'Email is required'", "vf3"},
		{"validation feedback * 'Subject is too long'", "vf1"},
		{"$IsValidEmail = call java action *", "java"},
		{"CALL JAVA ACTION feedbackmodule.validateemail * @1", ""}, // wildcard needed at the front
		{"* call java action feedbackmodule.validateemail *", "java"},
		{"return $ValidFeedback;", "end"},
		{"if trim($Feedback/Subject) != empty and trim($Feedback/Subject) != '' then", "s1"},
	}
	for _, tc := range cases {
		got, err := ResolveText(cands, tc.target)
		if tc.want == "" {
			if err == nil {
				t.Errorf("%q: resolved to %s, want an error", tc.target, got.ID)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.target, err)
			continue
		}
		if got.ID != tc.want {
			t.Errorf("%q: resolved to %s, want %s", tc.target, got.ID, tc.want)
		}
	}
}

// The resolver never guesses: three identical statements are an error that
// lists all three, each under the address that selects it.
func TestResolve_AmbiguityIsAnErrorListingOrdinals(t *testing.T) {
	_, err := ResolveText(feedbackCandidates(), "set $ValidFeedback = false")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("want *AmbiguousError, got %v", err)
	}
	if len(amb.Matches) != 3 {
		t.Fatalf("want 3 matches, got %d", len(amb.Matches))
	}
	msg := err.Error()
	for _, want := range []string{
		"matches 3 activities",
		"set $ValidFeedback = false @1  -- set $ValidFeedback = false  at (135, 200)",
		"set $ValidFeedback = false @2  -- set $ValidFeedback = false  at (420, 200)",
		"set $ValidFeedback = false @3  -- set $ValidFeedback = false  at (1305, 200)",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
}

func TestResolve_OrdinalPastLastMatch(t *testing.T) {
	_, err := ResolveText(feedbackCandidates(), "set $ValidFeedback = false @4")
	var amb *AmbiguousError
	if !errors.As(err, &amb) || !strings.Contains(err.Error(), "there are only 3 matches") {
		t.Fatalf("want an 'only 3 matches' error, got %v", err)
	}
}

// A lone $var is an output-variable address and nothing else: `set` assigns
// $ValidFeedback without outputting it, and a pattern search must not step in
// when the variable is not an output.
func TestResolve_FormsDoNotFallBack(t *testing.T) {
	cands := feedbackCandidates()
	_, err := ResolveText(cands, "$Feedback")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want *NotFoundError, got %v", err)
	}
	if !strings.Contains(err.Error(), "$IsValidEmail, $ValidFeedback") {
		t.Errorf("hint should list the output variables: %v", err)
	}

	_, err = ResolveText(cands, "'Email is valid?'") // case differs from the stored caption
	if !errors.As(err, &nf) || !strings.Contains(err.Error(), "'Email is Valid?'") {
		t.Errorf("caption is matched exactly and the hint lists the captions: %v", err)
	}
}

// A pattern is anchored at both ends; the error says so when only the tail
// is missing.
func TestResolve_PatternIsAnchored(t *testing.T) {
	_, err := ResolveText(feedbackCandidates(), "if not($IsValidEmail)")
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want *NotFoundError, got %v", err)
	}
	if !strings.Contains(err.Error(), "end it with *") {
		t.Errorf("error should suggest a trailing *: %v", err)
	}
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in      string
		kind    Kind
		ordinal int
	}{
		{"$Lines", ByOutputVariable, 0},
		{"$Lines @2", ByOutputVariable, 2},
		{"'Email is valid?'", ByCaption, 0},
		{"'it''s @2'", ByCaption, 0}, // an @ inside a literal is not an ordinal
		{"commit $Order", ByPattern, 0},
		{"log * node 'Debug' * @3", ByPattern, 3},
	}
	for _, tc := range cases {
		got, err := ParseTarget(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if got.Kind != tc.kind || got.Ordinal != tc.ordinal {
			t.Errorf("%q: got kind %v ordinal %d, want %v %d", tc.in, got.Kind, got.Ordinal, tc.kind, tc.ordinal)
		}
	}
	if got, _ := ParseTarget("'it''s @2'"); got.Caption != "it's @2" {
		t.Errorf("caption unescaping: got %q", got.Caption)
	}
	for _, bad := range []string{"", "  ", "@2", "$X @0", "'unterminated"} {
		if _, err := ParseTarget(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

// Every handle must resolve back to the activity it was printed for, and must
// prefer output variable, then caption, then statement.
func TestHandle_RoundTripsAndPrefersNamedForms(t *testing.T) {
	cands := feedbackCandidates()
	want := map[model.ID]string{
		"declare": "$ValidFeedback",
		"s1":      "'Subject not empty?'",
		"java":    "$IsValidEmail",
		"s2":      "'Email is Valid?'",
		"set1":    "set $ValidFeedback = false @1",
		"set2":    "set $ValidFeedback = false @2",
		"set3":    "set $ValidFeedback = false @3",
		"vf3":     "validation feedback $Feedback/SubmitterEmail message 'Email is required'",
		"end":     "return $ValidFeedback",
	}
	for i, c := range cands {
		h := Handle(cands, i)
		if w, ok := want[c.ID]; ok && h != w {
			t.Errorf("%s: handle %q, want %q", c.ID, h, w)
		}
		got, err := ResolveText(cands, h)
		if err != nil {
			t.Errorf("%s: handle %q does not resolve: %v", c.ID, h, err)
			continue
		}
		if got.ID != c.ID {
			t.Errorf("%s: handle %q resolves to %s", c.ID, h, got.ID)
		}
	}
}

// A statement containing a multiplication reads back as a wildcard pattern,
// so it can only be its own handle when that pattern still selects it alone.
func TestHandle_MultiplicationIsCheckedNotAssumed(t *testing.T) {
	a := &microflows.ActionActivity{Action: &microflows.ChangeVariableAction{}}
	a.ID = "a"
	b := &microflows.ActionActivity{Action: &microflows.ChangeVariableAction{}}
	b.ID = "b"
	ca, _ := NewCandidate(a, func(microflows.MicroflowObject) string { return "set $X = $Y * 2;" })
	cb, _ := NewCandidate(b, func(microflows.MicroflowObject) string { return "set $X = $Y + $Z * 2;" })
	cands := []Candidate{ca, cb}
	for i := range cands {
		h := Handle(cands, i)
		got, err := ResolveText(cands, h)
		if err != nil || got.ID != cands[i].ID {
			t.Errorf("handle %q for %s resolved to %v, %v", h, cands[i].ID, got.ID, err)
		}
	}
}

func TestCollect_IncludesLoopBodiesAndSkipsStructure(t *testing.T) {
	start := &microflows.StartEvent{}
	start.ID = "start"
	merge := &microflows.ExclusiveMerge{}
	merge.ID = "merge"
	inner := &microflows.ActionActivity{Action: &microflows.CommitObjectsAction{}}
	inner.ID = "inner"
	loop := &microflows.LoopedActivity{ObjectCollection: &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{inner},
	}}
	loop.ID = "loop"
	oc := &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{start, loop, merge}}

	got := Collect(oc, func(o microflows.MicroflowObject) string { return string(o.GetID()) })
	var ids []string
	for _, c := range got {
		ids = append(ids, string(c.ID))
	}
	if strings.Join(ids, ",") != "loop,inner" {
		t.Errorf("got %v, want [loop inner]", ids)
	}

	ordered := OrderBy(got, map[model.ID]int{"inner": 1})
	if ordered[0].ID != "inner" || ordered[1].ID != "loop" {
		t.Errorf("OrderBy: ranked first, unranked after; got %s, %s", ordered[0].ID, ordered[1].ID)
	}
}
