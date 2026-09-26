// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"errors"
	"strings"
	"testing"
)

func TestPickAlterTargetMatch(t *testing.T) {
	one := []AlterTargetMatch{{Kind: "activity", Name: "Approve"}}
	two := []AlterTargetMatch{{Kind: "activity", Name: "Approve"}, {Kind: "activity", Name: "Approve2"}}

	if m, err := PickAlterTargetMatch(AlterTarget{Caption: "Approve"}, one); err != nil || m.Name != "Approve" {
		t.Errorf("single match: %v %v", m, err)
	}
	if m, err := PickAlterTargetMatch(AlterTarget{Caption: "Approve", Ordinal: 2}, two); err != nil || m.Name != "Approve2" {
		t.Errorf("@2: %v %v", m, err)
	}

	// Ambiguity is an error that lists the matches with their ordinals: never a guess.
	_, err := PickAlterTargetMatch(AlterTarget{Caption: "Approve"}, two)
	var te *AlterTargetError
	if !errors.As(err, &te) || len(te.Matches) != 2 {
		t.Fatalf("ambiguous: want AlterTargetError with 2 matches, got %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "@1 activity Approve") || !strings.Contains(msg, "@2 activity Approve2") {
		t.Errorf("ambiguity message must list the matches with ordinals: %s", msg)
	}

	if _, err := PickAlterTargetMatch(AlterTarget{Path: []string{"x"}}, nil); err == nil ||
		!strings.Contains(err.Error(), "x not found") {
		t.Errorf("miss: %v", err)
	}
	if _, err := PickAlterTargetMatch(AlterTarget{Path: []string{"x"}, Ordinal: 3}, two); err == nil {
		t.Error("an ordinal past the matches must be refused")
	}
}

func TestAlterTargetString(t *testing.T) {
	for _, c := range []struct {
		t    AlterTarget
		want string
	}{
		{AlterTarget{Path: []string{"btnSave"}}, "btnSave"},
		{AlterTarget{Path: []string{"dg", "Total"}, Ordinal: 2}, "dg.Total@2"},
		{AlterTarget{Caption: "it's"}, "'it''s'"},
	} {
		if got := c.t.String(); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}
