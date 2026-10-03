// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// getKey() is an expression function; in a retrieve constraint it is CE0161.
// Measured on mxbuild 11.14.0, with St an enumeration attribute and $S an
// enumeration parameter of the same enumeration:
//
//	[St = getKey($S)]   CE0161 "Error(s) in XPath constraint."
//	[St = $S]           clean (the control)
func TestCheckExecAgree_RetrieveConstraintGetKey(t *testing.T) {
	const head = `mdl 1;
create module ModK;
create enumeration ModK.Status (Open 'Open', Closed 'Closed');
create persistent entity ModK.Item (St: Enumeration(ModK.Status));
create microflow ModK.SUB_Find ($S: Enumeration(ModK.Status))
begin
  retrieve $A from ModK.Item where %s;
end;`
	exec, _, dir := openPedAppCopy(t)
	bad := strings.Replace(head, "%s", "[St = getKey($S)]", 1)
	got := strings.Join(agreeCheck(t, exec, dir, bad), "\n")
	if !strings.Contains(got, "MDL091") || !strings.Contains(got, "getKey()") {
		t.Errorf("check -p must report MDL091 for getKey(), reported:\n%s", got)
	}
	if err := agreeExec(t, exec, bad); err == nil || !strings.Contains(err.Error(), "MDL091") {
		t.Errorf("exec must refuse getKey() with MDL091, got: %v", err)
	}

	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, strings.Replace(head, "%s", "[St = $S]", 1), "")
}

func TestXPathExpressionFunctionHits_GetKey(t *testing.T) {
	if got := xpathExpressionFunctionHits("[St = getKey($S)]"); len(got) != 1 || got[0] != "getKey" {
		t.Errorf("getKey not detected: %v", got)
	}
	// Inside a literal it is text, not a call.
	if got := xpathExpressionFunctionHits("[Label = 'getKey(x)']"); len(got) != 0 {
		t.Errorf("a literal must not match: %v", got)
	}
}
