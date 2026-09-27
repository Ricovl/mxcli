// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): the reversed entity grant and the quoted targeting XPath
// are rewritten to the canonical forms, keeping the case, the names as written
// and everything outside the statement.
func TestUpgrade_XPathInBrackets(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"grant without where, several roles, upper case",
			"GRANT Shop.User, Shop.Admin ON Shop.Order (CREATE, DELETE, READ *, WRITE (Email, \"Status\")); -- keep\n",
			"GRANT CREATE, DELETE, READ *, WRITE (Email, \"Status\") ON ENTITY Shop.Order TO Shop.User, Shop.Admin; -- keep\n",
		},
		{
			"grant with sibling predicate groups and a token",
			"grant M.R on M.E (read *) where '[a = 1][System.owner = ''[%CurrentUser%]'']';",
			"grant read * on entity M.E to M.R where [a = 1][System.owner = '[%CurrentUser%]'];",
		},
		{
			"grant laid out over lines",
			"grant M.R on M.E (\n  read *,\n  write *\n)\nwhere '[a = 1]';",
			"grant read *,\n  write * on entity M.E to M.R\nwhere [a = 1];",
		},
		{
			"user task targeting users and groups",
			"create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n  user task A 'A'\n    targeting users xpath '[Name = ''x'']'\n    outcomes 'Done' { };\n" +
				"  user task B 'B'\n    targeting groups xpath '[Name = ''y'']'\n    outcomes 'Done' { };\nend workflow;",
			"create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n  user task A 'A'\n    targeting users xpath [Name = 'x']\n    outcomes 'Done' { };\n" +
				"  user task B 'B'\n    targeting groups xpath [Name = 'y']\n    outcomes 'Done' { };\nend workflow;",
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

// A quoted value that is not a bracketed XPath has no mechanical rewrite: the
// brackets are the syntax now, so adding them would change what is stored.
func TestUpgrade_XPathNotBracketedIsReported(t *testing.T) {
	for _, tc := range []struct{ src, code string }{
		{"grant M.R on M.E (read *) where 'a = 1';", deprecation.ReversedEntityGrant},
		{"alter workflow M.WF set activity 'Review' targeting xpath 'Role = 1';", deprecation.QuotedTargetingXPath},
	} {
		res := mustUpgrade(t, tc.src, Options{})
		if res.Source != tc.src {
			t.Errorf("%q was rewritten to %q", tc.src, res.Source)
		}
		if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != tc.code {
			t.Errorf("%q: Unrewritten = %+v, want one %s", tc.src, res.Unrewritten, tc.code)
		}
	}
}
