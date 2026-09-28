// SPDX-License-Identifier: Apache-2.0

package upgrade

import "testing"

// R5 (ako/mxcli#753): the reversed entity revoke is rewritten to the form that
// mirrors the grant, keeping names, rights and case as written.
func TestUpgrade_RevokeEntityMirrorsGrant(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"partial, several roles", "revoke Shop.User, Shop.Admin on Shop.Order (write (Email, \"Status\"), delete); -- keep\n",
			"revoke write (Email, \"Status\"), delete on entity Shop.Order from Shop.User, Shop.Admin; -- keep\n"},
		{"full", "revoke Shop.User on Shop.Order;", "revoke all on entity Shop.Order from Shop.User;"},
		{"upper case", "REVOKE Shop.User ON Shop.Order (READ *);", "REVOKE READ * ON ENTITY Shop.Order FROM Shop.User;"},
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
