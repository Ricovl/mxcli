// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// translationUnitSnapshot snapshots every unit's stored bytes, keyed by unit and
// tagged with whether it lives in a Marketplace module.
func translationUnitSnapshot(t *testing.T, exec *Executor) (map[model.ID]string, map[model.ID]bool) {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		t.Fatal(err)
	}
	mp := map[model.ID]bool{}
	for _, m := range modules {
		if isMarketplaceModule(ctx, m) {
			mp[m.ID] = true
		}
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	units, err := ctx.Backend.ListUnits()
	if err != nil {
		t.Fatal(err)
	}
	raw := map[model.ID]string{}
	inMP := map[model.ID]bool{}
	for _, u := range units {
		b, err := ctx.Backend.GetRawUnitBytes(u.ID)
		if err != nil {
			continue
		}
		raw[u.ID] = string(b)
		inMP[u.ID] = mp[u.ID] || mp[h.FindModuleID(u.ID)]
	}
	return raw, inMP
}

func changedUnits(before, after map[model.ID]string, inMP map[model.ID]bool) (marketplace, own int) {
	for id, b := range before {
		if after[id] == b {
			continue
		}
		if inMP[id] {
			marketplace++
		} else {
			own++
		}
	}
	return
}

// ako/mxcli#970: on TestApp, `'Cancel' as 'Annuleren'` unscoped set 50
// translations in 38 documents, 35 of them in Marketplace modules — Atlas page
// templates and building blocks among them — which the next module update
// overwrites. The unscoped statement keeps its documented whole-project reach
// (ADR-0011: what a committed script writes does not change in place), and must
// SAY where it wrote.
func TestCreateTranslations_UnscopedWarnsAboutMarketplaceWrites(t *testing.T) {
	exec, out := openTestAppCopy(t)
	before, inMP := translationUnitSnapshot(t, exec)

	if err := afRun(t, exec, "create or modify translations for nl_NL ('Cancel' as 'Annuleren');"); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	after, _ := translationUnitSnapshot(t, exec)
	mpChanged, ownChanged := changedUnits(before, after, inMP)
	// Control: the default still reaches Marketplace modules — this test pins a
	// warning, not a change of meaning.
	if mpChanged == 0 || ownChanged == 0 {
		t.Fatalf("precondition: unscoped run changed %d marketplace and %d own unit(s); want both > 0", mpChanged, ownChanged)
	}
	got := out.String()
	for _, want := range []string{
		"landed in " + strconv.Itoa(mpChanged) + " document(s) of Marketplace",
		"WorkflowCommons",
		"page templates or building blocks",
		"create or modify translations without marketplace for nl_NL",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

// `without marketplace` writes nothing into a Marketplace module, still writes
// the app's own modules (the control: a fix that wrote nothing would pass the
// first half), says what it left alone, and does not warn.
func TestCreateTranslations_WithoutMarketplaceSkipsMarketplaceModules(t *testing.T) {
	exec, out := openTestAppCopy(t)
	before, inMP := translationUnitSnapshot(t, exec)

	if err := afRun(t, exec, "create or modify translations without marketplace for nl_NL ('Cancel' as 'Annuleren');"); err != nil {
		t.Fatalf("exec: %v\n%s", err, out.String())
	}
	after, _ := translationUnitSnapshot(t, exec)
	mpChanged, ownChanged := changedUnits(before, after, inMP)
	if mpChanged != 0 {
		t.Errorf("without marketplace changed %d unit(s) in Marketplace modules, want 0", mpChanged)
	}
	if ownChanged == 0 {
		t.Errorf("without marketplace changed none of the app's own units — it must still translate them")
	}
	got := out.String()
	if strings.Contains(got, "of Marketplace\nmodules") {
		t.Errorf("without marketplace must not warn about marketplace writes:\n%s", got)
	}
	if !strings.Contains(got, "which `without marketplace` skipped") {
		t.Errorf("without marketplace must say what it left alone:\n%s", got)
	}
	// Not drift: 'Cancel' matched, just outside the scope.
	if strings.Contains(got, "matched nothing in the project") {
		t.Errorf("an entry skipped by without marketplace was reported as drift:\n%s", got)
	}
}

// Naming a module is taken as meaning it: no warning for `in <Module>`, even
// when that module is a Marketplace one.
func TestCreateTranslations_ScopedRunDoesNotWarn(t *testing.T) {
	for _, mod := range []string{"MyFirstModule", "WorkflowCommons"} {
		t.Run(mod, func(t *testing.T) {
			exec, out := openTestAppCopy(t)
			if err := afRun(t, exec, "create or modify translations in "+mod+" for nl_NL ('Cancel' as 'Annuleren');"); err != nil {
				t.Fatalf("exec: %v\n%s", err, out.String())
			}
			if strings.Contains(out.String(), "of Marketplace\nmodules") {
				t.Errorf("a scoped run warned about marketplace writes:\n%s", out.String())
			}
		})
	}
}

// DESCRIBE emits the clause it was given, so the described file round-trips.
func TestDescribeTranslations_WithoutMarketplaceHeader(t *testing.T) {
	exec, out := openTestAppCopy(t)
	if err := afRun(t, exec, "describe translations without marketplace for nl_NL;"); err != nil {
		t.Fatalf("describe: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "create or modify translations without marketplace for nl_NL (") {
		t.Errorf("describe header lacks the clause:\n%s", out.String())
	}
}
