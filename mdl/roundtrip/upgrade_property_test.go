// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// The safety property of `mxcli fmt --upgrade` (ako/mxcli#735, ADR-0011
// decision 1): an upgraded script executes to the same model as the original.
// A wrong rewrite is a silent change of meaning — the script still parses and
// still runs, it just builds something else — so the only proof is to run both
// and compare what they wrote.
//
// For every mdl-examples script:
//
//  1. upgrade it, with the language header, which is the strongest form: every
//     alias rewritten and the script run under mdl 1 (a script the header is
//     refused for, HeaderBlockedError, is upgraded without it);
//  2. require zero deprecated spellings in the result;
//  3. execute the original on one copy of the PedApp fixture and the upgrade on
//     another;
//  4. compare the two models unit by unit, as canonical BSON (canon.Digest,
//     which numbers element $IDs by containment so freshly minted IDs do not
//     count as a difference), with units matched by their place in the project
//     tree rather than by unit ID, which is fresh for every created document.
//
// A script whose original does not execute cleanly on PedApp is out of scope
// and listed; the models are still compared, so a rewrite that changes how far
// a failing script gets is caught too.
//
// MXCLI_UPGRADE_EXAMPLES=<regexp> limits the run to matching script paths.
//
// By default only scripts the upgrade rewrites beyond adding the header and
// statement terminators are executed: with those as the only edits, the two
// runs execute the same statements (mdl/upgrade's examples test proves it),
// and what they compare is langver's gating rather than a rewrite.
// Executing the whole corpus takes about 15 minutes, which on its own exceeds
// what the CI integration step has left (ako/mxcli#742 timed out there).
// MXCLI_UPGRADE_ALL=1 executes every script, header-only ones included; run it
// when a langver.Change or a gated rewrite lands. The nightly workflow runs it.
//
// MXCLI_UPGRADE_SHARD=i/n executes only every n-th executable script, starting
// at the i-th, so per-PR CI can split the executions across parallel jobs
// (ako/mxcli#757); the shards together execute each script exactly once
// (TestShardsPartition). Unset is the whole set.
func TestUpgradeExecutesToTheSameModel(t *testing.T) {
	a, b := newHarness(t), newHarness(t)
	defer a.close()
	defer b.close()

	scripts := upgradeExampleScripts(t)
	all := os.Getenv("MXCLI_UPGRADE_ALL") != ""
	sh, err := parseShard(os.Getenv("MXCLI_UPGRADE_SHARD"))
	if err != nil {
		t.Fatal(err)
	}
	executable := 0 // scripts that reached execution, in every shard
	var same, outOfScope, unparsed, headerOnly, keptVersion, otherShard []string
	for _, path := range scripts {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel("../..", path)
		if _, errs := visitor.Build(string(src)); len(errs) > 0 {
			unparsed = append(unparsed, rel)
			continue
		}
		t.Run(rel, func(t *testing.T) {
			res, err := upgrade.Upgrade(string(src), upgrade.Options{AddHeader: true})
			var hb *upgrade.HeaderBlockedError
			if errors.As(err, &hb) {
				// A construct with no mechanical rewrite keeps the script at
				// mdl 0 (mdl/upgrade's keepsItsVersion lists which); its alias
				// rewrites are still proven here.
				keptVersion = append(keptVersion, rel)
				res, err = upgrade.Upgrade(string(src), upgrade.Options{})
			}
			if err != nil {
				t.Fatalf("upgrade: %v", err)
			}
			prog, errs := visitor.Build(res.Source)
			if len(errs) > 0 {
				t.Fatalf("upgraded script does not parse: %v", errs[0])
			}
			// A use the upgrade reported as unrewritable stays by contract
			// (reported, never guessed at); anything beyond those is a rewrite
			// that did not produce the canonical form.
			if n := len(prog.Deprecations); n > len(res.Unrewritten) {
				t.Fatalf("upgraded script still has %d deprecated spelling(s) (%d reported unrewritable), first %s at line %d",
					n, len(res.Unrewritten), prog.Deprecations[0].Code, prog.Deprecations[0].Line)
			}
			if !all && onlyTerminatorsAndHeader(res) {
				headerOnly = append(headerOnly, rel)
				return
			}
			// Every shard upgrades and checks every script above, which is
			// cheap; only the execution below, which is not, is split.
			k := executable
			executable++
			if !sh.has(k) {
				otherShard = append(otherShard, rel)
				return
			}

			errA, errB, diff := executeBoth(t, a, b, string(src), res.Source)
			if len(diff) > 0 {
				t.Errorf("the upgrade changed what the script writes (rewrote %v):\n  %s",
					res.Rewritten, strings.Join(diff, "\n  "))
			}
			if (errA == nil) != (errB == nil) {
				t.Errorf("original error: %v\nupgraded error: %v", errA, errB)
			}
			if errA != nil {
				outOfScope = append(outOfScope, fmt.Sprintf("%s: %s", rel, firstLine(errA.Error())))
				return
			}
			if !t.Failed() {
				same = append(same, rel)
			}
		})
	}

	t.Logf("%d scripts execute on PedApp and upgrade to the same model", len(same))
	if sh.count > 1 {
		t.Logf("shard %s: %d of %d executable scripts belong to other shards and were not executed here",
			sh, len(otherShard), executable)
	}
	t.Logf("%d scripts are out of scope: their original does not execute cleanly on PedApp:\n  %s",
		len(outOfScope), strings.Join(outOfScope, "\n  "))
	if !all {
		t.Logf("%d scripts only gain the header and statement terminators and were not executed "+
			"(MXCLI_UPGRADE_ALL=1 executes them)", len(headerOnly))
	}
	t.Logf("%d scripts keep mdl 0 (a construct with no rewrite) and were upgraded without the header:\n  %s",
		len(keptVersion), strings.Join(keptVersion, "\n  "))
	t.Logf("%d scripts do not parse (negative tests) and cannot be upgraded:\n  %s",
		len(unparsed), strings.Join(unparsed, "\n  "))
	if floor := 50 / sh.count; os.Getenv("MXCLI_UPGRADE_EXAMPLES") == "" && len(same) < floor {
		// Hundreds execute today; a handful means the harness broke, and a
		// property checked on nothing passes. A shard sees 1/n of them.
		t.Errorf("only %d scripts executed cleanly (shard %s) — the harness is not exercising the property",
			len(same), sh)
	}
}

// terminatorCodes are the gated rewrites that only add a `;` or delete a `/`
// line. They never reach the AST — mdl/upgrade's examples test proves every
// script builds the same statements with and without them — so executing a
// script whose only edits they are compares langver's gating, not a rewrite.
var terminatorCodes = map[string]bool{"MDL-V1-SEMI": true, "MDL-V1-SLASH": true}

// onlyTerminatorsAndHeader reports whether the upgrade's edits were the
// header and statement terminators alone.
func onlyTerminatorsAndHeader(res upgrade.Result) bool {
	if len(res.Rewritten) > 0 {
		return false
	}
	for code := range res.GatedRewritten {
		if !terminatorCodes[code] {
			return false
		}
	}
	return true
}

// TestUpgradeExecuteBoth_Controls are the controls CLAUDE.md requires of a
// "nothing changed" assertion. The comparison must see a real difference, and
// must not see the fresh IDs two runs of the same script mint.
func TestUpgradeExecuteBoth_Controls(t *testing.T) {
	a, b := newHarness(t), newHarness(t)
	defer a.close()
	defer b.close()

	const script = `create module UpgradeControl;
create or modify enumeration UpgradeControl.Color (Red 'Red', Green 'Green');
create or modify persistent entity UpgradeControl.Customer (Name: String(200), Color: Enumeration(UpgradeControl.Color));
create or modify microflow UpgradeControl.ACT_Touch ($c: UpgradeControl.Customer) begin
  change $c (Name = 'x');
  commit $c;
end;
`
	// The same script twice: every created element gets a fresh $ID and GUID
	// on each copy, and that must not register as a difference.
	if errA, errB, diff := executeBoth(t, a, b, script, script); errA != nil || errB != nil || len(diff) > 0 {
		t.Fatalf("the same script on both copies differs (errA=%v errB=%v):\n  %s", errA, errB, strings.Join(diff, "\n  "))
	}

	// One changed value MUST register, or every pass above is meaningless.
	changed := strings.Replace(script, "'Green'", "'Groen'", 1)
	_, _, diff := executeBoth(t, a, b, script, changed)
	if len(diff) == 0 {
		t.Fatal("a changed enumeration caption registered as the same model — the comparison cannot see a difference")
	}
	t.Logf("changed caption registered as: %s", strings.Join(diff, "; "))

	// And a rewrite that changes meaning is caught. `create or replace
	// translations` replaces the whole set where `or modify` merges; the
	// registry exempts it, so a rewrite that ignored the exemption would write
	// a different model.
	const tr = `create or modify translations in Administration for nl_NL ('Save' as 'Opslaan');
`
	merged := tr + "create or modify translations in Administration for nl_NL ('Cancel' as 'Annuleren');\n"
	replaced := tr + "create or replace translations in Administration for nl_NL ('Cancel' as 'Annuleren');\n"
	errA, errB, diff := executeBoth(t, a, b, replaced, merged)
	if errA != nil || errB != nil {
		t.Fatalf("translations control did not execute: %v / %v", errA, errB)
	}
	if len(diff) == 0 {
		t.Fatal("rewriting `create or replace translations` to `or modify` registered as the same model — " +
			"the property test cannot catch a rewrite that changes meaning")
	}
	t.Logf("a meaning-changing rewrite registered as: %s", strings.Join(diff, "; "))

	// A re-minted identity on a Studio Pro-authored element must register:
	// maskFresh masks only values the fixture does not contain.
	known := fixtureIdentities(a)
	for id, raw := range a.orig.units {
		var doc bson.D
		if bson.Unmarshal(raw, &doc) != nil {
			continue
		}
		hit := false
		reminted := walkIdentities(doc, func(key string, v any) any {
			b, isBin := v.(bson.Binary)
			if key != "GUID" || !isBin || hit {
				return v
			}
			hit = true
			return bson.Binary{Subtype: b.Subtype, Data: []byte("re-minted guid!!")}
		}).(bson.D)
		if !hit {
			continue
		}
		out, err := bson.Marshal(reminted)
		if err != nil {
			t.Fatal(err)
		}
		d1, err1 := canon.Digest(maskFresh(raw, known, a.dir))
		d2, err2 := canon.Digest(maskFresh(out, known, a.dir))
		if err1 != nil || err2 != nil || d1 == d2 {
			t.Fatalf("a re-minted GUID in unit %s is invisible to the comparison (%v %v)", id, err1, err2)
		}
		return
	}
	t.Fatal("the fixture has no unit with a GUID — the identity control has nothing to change")
}

// executeBoth runs one script on each harness, from the committed fixture, and
// returns each run's error and the differences between the models they wrote.
func executeBoth(t *testing.T, a, b *harness, srcA, srcB string) (errA, errB error, diff []string) {
	t.Helper()
	a.restore()
	b.restore()
	errA = a.exec(srcA)
	errB = b.exec(srcB)
	return errA, errB, compareModels(canonicalModel(t, a), canonicalModel(t, b))
}

// canonicalModel is a working copy as a map from a unit's place in the project
// tree to its canonical digest, plus the files outside the model.
//
// A unit is keyed by the chain of container names down to it and its own
// "$Type Name", not by its unit ID: a document created by the script gets a
// fresh unit ID on each copy. Units that share a key (unnamed units of one type
// in one container) are told apart by their sorted digests.
//
// Identity values minted for a created element — its StableId, its GUID where
// that is not its $ID, string ids — are fresh on each copy too, so they are
// masked by maskFresh. Only values the fixture does not contain are masked: a
// Studio Pro-authored element's GUID or StableId is compared as it is, so a
// rewrite that made the executor re-mint one still registers.
func canonicalModel(t *testing.T, h *harness) map[string]string {
	t.Helper()
	known := fixtureIdentities(h)
	r, err := mmpr.Open(h.mpr)
	if err != nil {
		t.Fatalf("open working copy: %v", err)
	}
	defer r.Close()
	units, err := r.ListUnits()
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	label := map[string]string{}
	parent := map[string]string{}
	digest := map[string]string{}
	for _, u := range units {
		raw, err := r.GetRawUnitBytes(u.ID)
		if err != nil {
			t.Fatalf("read unit %s: %v", u.ID, err)
		}
		typ, name := typeAndName(raw)
		label[u.ID] = strings.TrimSpace(typ + " " + name)
		parent[u.ID] = u.ContainerID + "/" + u.ContainmentName
		d, err := canon.Digest(maskFresh(raw, known, h.dir))
		if err != nil {
			sum := sha256.Sum256(raw)
			d = "raw:" + hex.EncodeToString(sum[:])
		}
		digest[u.ID] = d
	}
	var path func(id string, depth int) string
	path = func(id string, depth int) string {
		p, ok := parent[id]
		if !ok || depth > 64 {
			return ""
		}
		container, containment, _ := strings.Cut(p, "/")
		if _, known := label[container]; !known || container == id {
			return containment + ":" + label[id]
		}
		return path(container, depth+1) + " > " + containment + ":" + label[id]
	}
	byKey := map[string][]string{}
	for id := range label {
		k := path(id, 0)
		byKey[k] = append(byKey[k], digest[id])
	}
	out := map[string]string{}
	for k, ds := range byKey {
		sort.Strings(ds)
		for i, d := range ds {
			key := k
			if len(ds) > 1 {
				key = fmt.Sprintf("%s #%d", k, i+1)
			}
			out[key] = d
		}
	}
	for rel, b := range h.snapshot().files {
		sum := sha256.Sum256(b)
		out["file "+rel] = hex.EncodeToString(sum[:])
	}
	return out
}

// fixtureIdentities is every identity-shaped value in the committed fixture:
// each 16-byte binary, and each string that reads as a UUID.
func fixtureIdentities(h *harness) map[string]bool {
	if knownIdentities != nil {
		return knownIdentities
	}
	knownIdentities = map[string]bool{}
	for _, raw := range h.orig.units {
		var doc bson.D
		if bson.Unmarshal(raw, &doc) != nil {
			continue
		}
		walkIdentities(doc, func(_ string, v any) any {
			if s, ok := v.(string); ok {
				for _, u := range uuidInString.FindAllString(s, -1) {
					knownIdentities[identityKey(u)] = true
				}
				return v
			}
			knownIdentities[identityKey(v)] = true
			return v
		})
	}
	return knownIdentities
}

var knownIdentities map[string]bool

// uuidInString matches the string spellings of a UUID mxcli and Mendix
// store, on their own or inside a larger text such as a JSON document: 32 hex
// digits, or the dashed 8-4-4-4-12 form.
var uuidInString = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\b`)

func identityKey(v any) string {
	switch t := v.(type) {
	case bson.Binary:
		return "b:" + string(t.Data)
	case string:
		return "s:" + strings.ToLower(t)
	}
	return ""
}

// maskFresh prepares a unit for canon.Digest when it may have been written by
// a script, not only by Studio Pro.
//
// Every identity value the fixture does not contain, and that is not one of
// the unit's own element $IDs (canon numbers those, references included), is
// freshly minted — a created element's GUID or StableId, a string id such as
// a business event channel name, or a UUID inside a text such as an agent
// document's JSON — and is replaced by a placeholder
// numbered by first appearance in a key-sorted walk. Two runs that minted the
// same number of identities in the same places are then equal. dir, the
// working copy's path, is replaced too: a statement that stores a local file
// URL stores a path that differs between the two copies.
func maskFresh(raw []byte, known map[string]bool, dir string) []byte {
	var doc bson.D
	if bson.Unmarshal(raw, &doc) != nil {
		return raw
	}
	ids := map[string]bool{}
	walkIdentities(doc, func(key string, v any) any {
		if b, ok := v.(bson.Binary); ok && key == "$ID" {
			ids[identityKey(b)] = true
			ids[identityKey(uuidOf(b.Data))] = true // canon also numbers its string form
		}
		return v
	})
	fresh := map[string]int{}
	placeholder := func(k string) int {
		n, ok := fresh[k]
		if !ok {
			n = len(fresh) + 1
			fresh[k] = n
		}
		return n
	}
	masked := walkIdentities(doc, func(key string, v any) any {
		if str, ok := v.(string); ok {
			if dir != "" {
				str = strings.ReplaceAll(str, dir, "<working copy>")
			}
			return uuidInString.ReplaceAllStringFunc(str, func(u string) string {
				k := identityKey(u)
				if known[k] || ids[k] {
					return u
				}
				return fmt.Sprintf("<fresh %d>", placeholder(k))
			})
		}
		b := v.(bson.Binary)
		k := identityKey(b)
		if known[k] || ids[k] {
			return v
		}
		data := make([]byte, 16)
		copy(data, fmt.Sprintf("fresh%011d", placeholder(k)))
		return bson.Binary{Subtype: b.Subtype, Data: data}
	}).(bson.D)
	out, err := bson.Marshal(masked)
	if err != nil {
		return raw
	}
	return out
}

// uuidOf renders a 16-byte Mendix GUID blob as a UUID string, the way canon
// does (Microsoft layout: first three groups little-endian).
func uuidOf(b []byte) string {
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], b[5], b[4], b[7], b[6], b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

// walkIdentities visits every 16-byte binary and every string in v,
// documents in key order, and returns v with each replaced by what f returns.
func walkIdentities(v any, f func(key string, v any) any) any {
	return walkIdentitiesKey("", v, f)
}

func walkIdentitiesKey(key string, v any, f func(string, any) any) any {
	switch t := v.(type) {
	case bson.D:
		order := make([]int, len(t))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool { return t[order[i]].Key < t[order[j]].Key })
		out := make(bson.D, len(t))
		copy(out, t)
		for _, i := range order {
			out[i].Value = walkIdentitiesKey(t[i].Key, t[i].Value, f)
		}
		return out
	case bson.A:
		out := make(bson.A, len(t))
		for i, e := range t {
			out[i] = walkIdentitiesKey(key, e, f)
		}
		return out
	case bson.Binary:
		if len(t.Data) == 16 {
			return f(key, t)
		}
	case string:
		return f(key, t)
	}
	return v
}

func compareModels(a, b map[string]string) []string {
	var out []string
	for k, da := range a {
		db, ok := b[k]
		switch {
		case !ok:
			out = append(out, "only the original wrote "+k)
		case da != db:
			out = append(out, "different content: "+k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out = append(out, "only the upgrade wrote "+k)
		}
	}
	sort.Strings(out)
	return out
}

func upgradeExampleScripts(t *testing.T) []string {
	t.Helper()
	var filter *regexp.Regexp
	if f := os.Getenv("MXCLI_UPGRADE_EXAMPLES"); f != "" {
		filter = regexp.MustCompile(f)
	}
	var out []string
	err := filepath.Walk("../../mdl-examples", func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, ".mdl") && (filter == nil || filter.MatchString(p)) {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("no mdl-examples scripts found")
	}
	return out
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}
