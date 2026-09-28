// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): every constant reference becomes `@Module.Const`.
func TestUpgrade_ConstantReferences(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"rest credentials qualified with the service's module",
			"create consumed rest service Shop.Api (BaseUrl: 'https://x', Authentication: basic (Username: $ApiUser, Password: $ApiPass)) { };",
			"create consumed rest service Shop.Api (BaseUrl: 'https://x', Authentication: basic (Username: @Shop.ApiUser, Password: @Shop.ApiPass)) { };"},
		{"model and alter key",
			"create ai model M.GPT (Provider: MxCloudGenAI, Key: M.ApiKey);\nalter knowledge base M.KB set Key = M.K2;",
			"create ai model M.GPT (Provider: MxCloudGenAI, Key: @M.ApiKey);\nalter knowledge base M.KB set Key = @M.K2;"},
		{"settings",
			"ALTER SETTINGS CONSTANT 'M.ApiUrl' VALUE 'https://x';\nalter settings drop constant 'M.Old' in configuration 'Default';",
			"ALTER SETTINGS CONSTANT @M.ApiUrl VALUE 'https://x';\nalter settings drop constant @M.Old in configuration 'Default';"},
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

// A settings constant string that is not a Module.Constant name is reported.
func TestUpgrade_SettingsConstantNotANameIsReported(t *testing.T) {
	src := "alter settings constant 'NoModule' value 'x';"
	res := mustUpgrade(t, src, Options{})
	if res.Source != src {
		t.Errorf("rewritten to %q", res.Source)
	}
	if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.QuotedSettingsConstant {
		t.Errorf("Unrewritten = %+v", res.Unrewritten)
	}
}
