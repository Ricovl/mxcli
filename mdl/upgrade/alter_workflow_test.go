// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// ako/mxcli#712: an old `alter workflow` statement of several actions becomes
// one generic block, each action its generic operation, and the deprecations
// inside the actions (a quoted XPath, an expression in a string, a `comment`
// caption) are rewritten alongside.
func TestUpgrade_AlterWorkflowBlock(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"several actions over several lines",
			"alter workflow Shop.OrderApproval\n" +
				"  set display 'Order approval'\n" +
				"  set activity ReviewOrder targeting xpath '[Active = true()]'\n" +
				"  insert after ReviewOrder call microflow Shop.ACT_Notify comment 'Notify';\n" +
				"  drop path 'Path 2' on split1\n" +
				"  insert condition 'false' on decision1 { }\n" +
				"  drop activity oldTask;\n",
			"alter workflow Shop.OrderApproval {\n" +
				"  set (Display: 'Order approval');\n" +
				"  set (Targeting: xpath [Active = true()]) on ReviewOrder;\n" +
				"  insert after ReviewOrder { call microflow Shop.ACT_Notify caption 'Notify'; }\n" +
				"  drop split1 path 2;\n" +
				"  insert into decision1 { outcomes 'false' -> { } }\n" +
				"  drop oldTask;\n};\n",
		},
		{
			"upper case keeps its case",
			"ALTER WORKFLOW M.W DROP OUTCOME 'Reject' ON Review;",
			"ALTER WORKFLOW M.W { DROP Review OUTCOME 'Reject'; };",
		},
		{
			"a statement ending in an activity gets its own terminator",
			"alter workflow M.W replace activity 'Review order'@2 with call microflow M.F;\nlist entities;",
			"alter workflow M.W { replace 'Review order'@2 with { call microflow M.F; } };\nlist entities;",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := mustUpgrade(t, tc.src, Options{})
			if res.Source != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", res.Source, tc.want)
			}
			if len(res.Unrewritten) != 0 {
				t.Errorf("Unrewritten = %+v", res.Unrewritten)
			}
		})
	}
}

// A path dropped by a caption that is not `Path n` has no generic address, and
// half a block would not parse: the whole statement is left as written and
// every action is reported.
func TestUpgrade_AlterWorkflowWithAnUnrewritableActionIsLeftWhole(t *testing.T) {
	src := "alter workflow M.W set display 'x' drop path 'Left' on split1;"
	res := mustUpgrade(t, src, Options{})
	if res.Source != src {
		t.Fatalf("rewritten to %q", res.Source)
	}
	if len(res.Unrewritten) != 2 {
		t.Fatalf("Unrewritten = %+v, want both actions", res.Unrewritten)
	}
	codes := map[string]bool{}
	for _, u := range res.Unrewritten {
		codes[u.Code] = true
	}
	if !codes[deprecation.AlterWorkflowSet] || !codes[deprecation.AlterWorkflowDropMember] {
		t.Errorf("Unrewritten codes = %v", codes)
	}
}
