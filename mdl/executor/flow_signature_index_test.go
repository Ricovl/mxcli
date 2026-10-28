// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// flowIndexView is what check reads of every flow, from the index.
func flowIndexView(t *testing.T, ctx *ExecContext) []string {
	t.Helper()
	h, _ := getHierarchy(ctx)
	flows, ok := storedFlowIndex(ctx)
	if !ok {
		t.Fatal("no flow signature index on an MPR v2 project")
	}
	var out []string
	for _, f := range flows {
		roles := append([]string(nil), f.Roles...)
		sort.Strings(roles)
		var calls []eaCall
		for _, c := range f.Calls {
			calls = append(calls, eaCall{c.Kind, c.Name})
		}
		out = append(out, fmt.Sprintf("%s %s %v %v %+v ea=%v %v", f.Kind, h.GetQualifiedName(f.containerID, f.Name), f.Excluded, roles, f.Sig, f.EA, calls))
	}
	sort.Strings(out)
	return out
}

// flowListingView is the same, decoded from every flow.
func flowListingView(t *testing.T, ctx *ExecContext) []string {
	t.Helper()
	h, _ := getHierarchy(ctx)
	var out []string
	add := func(kind, qn string, excluded bool, roleIDs []string, sig *flowSignature, ea bool, calls []eaCall) {
		sort.Strings(roleIDs)
		out = append(out, fmt.Sprintf("%s %s %v %v %+v ea=%v %v", kind, qn, excluded, roleIDs, *sig, ea, calls))
	}
	raw := map[string][]byte{}
	units, _ := ctx.Backend.ListRawUnitsByType("Microflows$Microflow")
	for _, u := range units {
		raw[string(u.ID)] = u.Contents
	}
	mfs, _ := microflowSignatures(ctx.Backend)
	for _, mf := range mfs {
		var r []string
		for _, id := range mf.AllowedModuleRoles {
			r = append(r, string(id))
		}
		ea, calls := rawEntityAccess(raw[string(mf.ID)])
		add("microflow", h.GetQualifiedName(mf.ContainerID, mf.Name), mf.Excluded, r, sdkFlowSignature(mf.Parameters, mf.ReturnType), ea, calls)
	}
	nfs, _ := nanoflowSignatures(ctx.Backend)
	for _, nf := range nfs {
		var r []string
		for _, id := range nf.AllowedModuleRoles {
			r = append(r, string(id))
		}
		add("nanoflow", h.GetQualifiedName(nf.ContainerID, nf.Name), nf.Excluded, r, sdkFlowSignature(nf.Parameters, nf.ReturnType), false, nil)
	}
	sort.Strings(out)
	return out
}

// TestFlowSignatureIndex_MatchesTheListing: the index answers what decoding
// every flow answers — on a first build, after a flow is changed, and when a
// fresh run loads it from disk.
func TestFlowSignatureIndex_MatchesTheListing(t *testing.T) {
	exec := typeCheckFixture(t)
	ctx := exec.newExecContext(context.Background())
	check := func(step string) {
		t.Helper()
		got, want := flowIndexView(t, ctx), flowListingView(t, ctx)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: index (%d flows) differs from the listing (%d)", step, len(got), len(want))
		}
	}
	run(t, exec, "REFRESH CATALOG FULL FORCE") // creates .mxcli, where the index persists
	check("first build")

	run(t, exec, `@applyentityaccess(true)
create microflow Administration.IdxFlow ($Name: String) returns Boolean begin call microflow Administration.ChangePassword(); return true; end;`)
	run(t, exec, `grant execute on microflow Administration.IdxFlow to Administration.User;`)
	run(t, exec, `grant execute on microflow Administration.ChangePassword to Administration.User;`)
	ctx = exec.newExecContext(context.Background())
	check("after a flow was created and an existing one granted")

	ctx.Cache = nil // a fresh run: the index comes from disk
	check("loaded from disk")
}

// TestEntityAccessCalls_FromTheFlowIndex: MDL-SEC22 on a real project, where
// the stored microflows come from the flow index. Caller and callee are both
// stored without entity access; the script turns it on in the callee, so the
// call that breaks CE0114 is one only the index knows.
func TestEntityAccessCalls_FromTheFlowIndex(t *testing.T) {
	exec := typeCheckFixture(t)
	run(t, exec, "REFRESH CATALOG FULL FORCE")
	run(t, exec, `@applyentityaccess(false)
create microflow Administration.IdxCallee () returns Boolean begin return true; end;`)
	run(t, exec, `@applyentityaccess(false)
create microflow Administration.IdxCaller () returns Boolean begin $b = call microflow Administration.IdxCallee(); return $b; end;`)
	prog, errs := visitor.Build(`@applyentityaccess(true)
create or modify microflow Administration.IdxCallee () returns Boolean begin return true; end;`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	ctx := exec.newExecContext(context.Background())
	if _, ok := storedFlowIndex(ctx); !ok {
		t.Fatal("no flow signature index on an MPR v2 project")
	}
	var got []string
	for _, v := range CheckEntityAccessCalls(ctx, prog) {
		got = append(got, v.RuleID)
	}
	if fmt.Sprint(got) != "["+entityAccessCallRule+"]" {
		t.Errorf("violations = %v, want one %s", got, entityAccessCallRule)
	}
}
