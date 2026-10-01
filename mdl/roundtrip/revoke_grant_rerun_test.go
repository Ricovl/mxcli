// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	_ "modernc.org/sqlite"
)

// ako/mxcli#872 (rehearsal W3): a "reset, then authoritative grants" section,
// `revoke all on entity E from R;` followed by the grants that are meant to hold,
// wrote the domain model on every run with a freshly minted access rule, although
// the rules it ended with were the ones it started with. Each statement wrote on
// its own: the revoke removed the rule, the grant built a new one, and the grant's
// write had nothing left on disk to carry the old rule's identity from.
//
// The net state is compared now: a run of access-rule statements is written once,
// at its end, and a run that ends where it started writes nothing.
//
// One harness for both cases (the roundtrip suite is near its time limit, #870).
func TestRevokeGrantRerun(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	t.Run("an identical re-run writes nothing", func(t *testing.T) { revokeGrantRerunWritesNothing(t, h) })
	h.restore()
	t.Run("Studio Pro rules keep their identity", func(t *testing.T) { revokeGrantKeepsStudioProIdentity(t, h) })
}

func revokeGrantRerunWritesNothing(t *testing.T, h *harness) {

	const script = `mdl 1;
create or modify persistent entity MyFirstModule.RerunTx (
  TxDate: DateTime,
  Amount: Decimal
);
revoke all on entity MyFirstModule.RerunTx from MyFirstModule.User;
grant read *, write * on entity MyFirstModule.RerunTx to MyFirstModule.User;
`
	if err := h.exec(script); err != nil {
		t.Fatalf("first run: %v\n%s", err, h.out.String())
	}
	first, tx := h.snapshot(), h.lastTransactionID()
	if err := h.exec(script); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if changed := first.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the identical second run wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
	}
	if got := h.lastTransactionID(); got != tx {
		t.Errorf("the identical second run moved the project's transaction id (%s -> %s): something was written", tx, got)
	}
	if got := h.mustDescribeMdl0(t, "entity MyFirstModule.RerunTx"); !strings.Contains(got,
		"grant read *, write * on entity MyFirstModule.RerunTx to MyFirstModule.User;") {
		t.Errorf("the rule is not there after the second run:\n%s", got)
	}

	// Control: the same reset with a narrower grant is a change, and is written.
	narrower := strings.Replace(script, "grant read *, write *", "grant read *", 1)
	if err := h.exec(narrower); err != nil {
		t.Fatalf("narrower grant: %v\n%s", err, h.out.String())
	}
	if len(first.diff(h.snapshot())) == 0 {
		t.Error("a reset to a narrower grant wrote nothing")
	}
	if got := h.mustDescribeMdl0(t, "entity MyFirstModule.RerunTx"); !strings.Contains(got,
		"grant read * on entity MyFirstModule.RerunTx to MyFirstModule.User;") {
		t.Errorf("a reset to a narrower grant: want the narrower rule:\n%s", got)
	}
	// Control: a revoke that is not followed by a grant still removes the rule.
	if err := h.exec("mdl 1;\nrevoke all on entity MyFirstModule.RerunTx from MyFirstModule.User;\n"); err != nil {
		t.Fatalf("revoke alone: %v\n%s", err, h.out.String())
	}
	if got := h.mustDescribeMdl0(t, "entity MyFirstModule.RerunTx"); strings.Contains(got, "grant ") {
		t.Errorf("a revoke alone left a rule:\n%s", got)
	}
}

// The same reset over rules Studio Pro authored: PedApp's Administration.Account
// carries two rules for Administration.User, one of them XPath-constrained. A
// reset that grants both back keeps the stored rules' identity — every rule and
// member-access $ID Studio Pro gave them — and the identical second run writes
// nothing. An mxcli-created rule could not show the first half: its identity is
// whatever the previous run minted.
//
// (The first run is a write: describe's spelling of these two rules does not
// reproduce Studio Pro's bytes — the stored default member access of the first
// is ReadOnly — so the reset states them in mxcli's spelling. What must not move
// is their identity.)
func revokeGrantKeepsStudioProIdentity(t *testing.T, h *harness) {

	const reset = `mdl 1;
revoke all on entity Administration.Account from Administration.User;
grant read (FullName, Email) on entity Administration.Account to Administration.User;
grant read (FullName), write (FullName) on entity Administration.Account to Administration.User where [id='[%CurrentUser%]'];
`
	stored := accessRuleIDs(t, h.orig, "Account")
	if len(stored) != 3 {
		t.Fatalf("fixture: want Account's 3 Studio Pro rules, got %d", len(stored))
	}
	if err := h.exec(reset); err != nil {
		t.Fatalf("reset: %v\n%s", err, h.out.String())
	}
	first := h.snapshot()
	if got := accessRuleIDs(t, first, "Account"); !equalIDLists(got, stored) {
		t.Errorf("the reset re-minted Studio Pro's rules:\n  stored %v\n  now    %v", stored, got)
	}
	tx := h.lastTransactionID()
	if err := h.exec(reset); err != nil {
		t.Fatalf("second reset: %v\n%s", err, h.out.String())
	}
	if changed := first.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the identical second reset wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
	}
	if got := h.lastTransactionID(); got != tx {
		t.Errorf("the identical second reset moved the transaction id (%s -> %s)", tx, got)
	}

	// Control: a reset that no longer grants the constrained rule removes it.
	withoutXPath := reset[:strings.LastIndex(reset, "grant read (FullName), write")]
	if err := h.exec(withoutXPath); err != nil {
		t.Fatalf("reset without the constrained rule: %v\n%s", err, h.out.String())
	}
	if len(first.diff(h.snapshot())) == 0 {
		t.Error("a reset that drops a rule wrote nothing")
	}
	if got := h.mustDescribeMdl0(t, "entity Administration.Account"); strings.Contains(got, "CurrentUser") {
		t.Errorf("the constrained rule survived a reset that does not grant it:\n%s", got)
	}
}

// accessRuleIDs lists, per access rule of the named entity, the rule's $ID
// followed by its member accesses' $IDs, in stored order.
func accessRuleIDs(t *testing.T, s snapshot, entity string) [][]string {
	t.Helper()
	for _, raw := range s.units {
		var doc bson.D
		if bson.Unmarshal(raw, &doc) != nil || docString(doc, "$Type") != "DomainModels$DomainModel" {
			continue
		}
		for _, ev := range docArray(doc, "Entities") {
			ent, ok := ev.(bson.D)
			if !ok || docString(ent, "Name") != entity {
				continue
			}
			var out [][]string
			for _, rv := range docArray(ent, "AccessRules") {
				rule, ok := rv.(bson.D)
				if !ok {
					continue
				}
				ids := []string{docID(rule)}
				for _, mv := range docArray(rule, "MemberAccesses") {
					if ma, ok := mv.(bson.D); ok {
						ids = append(ids, docID(ma))
					}
				}
				out = append(out, ids)
			}
			return out
		}
	}
	t.Fatalf("no entity %s in any domain model", entity)
	return nil
}

func docString(d bson.D, key string) string {
	for _, e := range d {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}

func docArray(d bson.D, key string) bson.A {
	for _, e := range d {
		if e.Key == key {
			a, _ := e.Value.(bson.A)
			return a
		}
	}
	return nil
}

func docID(d bson.D) string {
	for _, e := range d {
		if e.Key == "$ID" {
			if b, ok := e.Value.(bson.Binary); ok {
				return hex.EncodeToString(b.Data)
			}
		}
	}
	return ""
}

func equalIDLists(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.Join(a[i], ",") != strings.Join(b[i], ",") {
			return false
		}
	}
	return true
}

// lastTransactionID is the value Studio Pro compares to decide the project
// changed on disk; any write that lands moves it.
func (h *harness) lastTransactionID() string {
	h.t.Helper()
	db, err := sql.Open("sqlite", "file:"+h.mpr+"?mode=ro")
	if err != nil {
		h.t.Fatalf("open %s: %v", h.mpr, err)
	}
	defer db.Close()
	var id string
	if err := db.QueryRow(`SELECT LastTransactionID FROM _Transaction`).Scan(&id); err != nil {
		h.t.Fatalf("read LastTransactionID: %v", err)
	}
	return id
}
