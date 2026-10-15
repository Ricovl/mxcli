// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"sort"
	"testing"
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
		out = append(out, fmt.Sprintf("%s %s %v %v %+v", f.Kind, h.GetQualifiedName(f.containerID, f.Name), f.Excluded, roles, f.Sig))
	}
	sort.Strings(out)
	return out
}

// flowListingView is the same, decoded from every flow.
func flowListingView(t *testing.T, ctx *ExecContext) []string {
	t.Helper()
	h, _ := getHierarchy(ctx)
	var out []string
	add := func(kind, qn string, excluded bool, roleIDs []string, sig *flowSignature) {
		sort.Strings(roleIDs)
		out = append(out, fmt.Sprintf("%s %s %v %v %+v", kind, qn, excluded, roleIDs, *sig))
	}
	mfs, _ := microflowSignatures(ctx.Backend)
	for _, mf := range mfs {
		var r []string
		for _, id := range mf.AllowedModuleRoles {
			r = append(r, string(id))
		}
		add("microflow", h.GetQualifiedName(mf.ContainerID, mf.Name), mf.Excluded, r, sdkFlowSignature(mf.Parameters, mf.ReturnType))
	}
	nfs, _ := nanoflowSignatures(ctx.Backend)
	for _, nf := range nfs {
		var r []string
		for _, id := range nf.AllowedModuleRoles {
			r = append(r, string(id))
		}
		add("nanoflow", h.GetQualifiedName(nf.ContainerID, nf.Name), nf.Excluded, r, sdkFlowSignature(nf.Parameters, nf.ReturnType))
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

	run(t, exec, `create microflow Administration.IdxFlow ($Name: String) returns Boolean begin return true; end;`)
	run(t, exec, `grant execute on microflow Administration.IdxFlow to Administration.User;`)
	run(t, exec, `grant execute on microflow Administration.ChangePassword to Administration.User;`)
	ctx = exec.newExecContext(context.Background())
	check("after a flow was created and an existing one granted")

	ctx.Cache = nil // a fresh run: the index comes from disk
	check("loaded from disk")
}
