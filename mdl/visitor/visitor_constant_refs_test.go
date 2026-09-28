// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// R5 (ako/mxcli#753): `@Module.Const` is the one constant reference. The
// other spellings are deprecated aliases that build the same statement.
func TestConstantReference_OneSpelling(t *testing.T) {
	for _, tc := range []struct{ name, old, canonical, code string }{
		{"rest credential $Const",
			"create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: basic (Username: $ApiUser, Password: $ApiPass)) { };",
			"create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: basic (Username: @M.ApiUser, Password: @M.ApiPass)) { };",
			deprecation.DollarConstant},
		{"model key",
			"create model M.GPT (Provider: MxCloudGenAI, Key: M.ApiKey);",
			"create model M.GPT (Provider: MxCloudGenAI, Key: @M.ApiKey);",
			deprecation.BareConstantKey},
		{"knowledge base key",
			"create knowledge base M.KB (Provider: MxCloudGenAI, Key: M.KbKey);",
			"create knowledge base M.KB (Provider: MxCloudGenAI, Key: @M.KbKey);",
			deprecation.BareConstantKey},
		{"alter model key",
			"alter model M.GPT set Key = M.OtherKey;",
			"alter model M.GPT set Key = @M.OtherKey;",
			deprecation.BareConstantKey},
		{"settings constant value",
			"alter settings constant 'M.ApiUrl' value 'https://x' in configuration 'Default';",
			"alter settings constant @M.ApiUrl value 'https://x' in configuration 'Default';",
			deprecation.QuotedSettingsConstant},
		{"settings drop constant",
			"alter settings drop constant 'M.ApiUrl' in configuration 'Default';",
			"alter settings drop constant @M.ApiUrl in configuration 'Default';",
			deprecation.QuotedSettingsConstant},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, canon := mustBuild(t, tc.old), mustBuild(t, tc.canonical)
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("old built %#v\ncanonical   %#v", old.Statements[0], canon.Statements[0])
			}
			for _, c := range deprecationCodes(old) {
				if c != tc.code {
					t.Errorf("old form recorded %s, want only %s", c, tc.code)
				}
			}
			if len(old.Deprecations) == 0 {
				t.Errorf("old form recorded nothing, want %s", tc.code)
			}
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("canonical form recorded %v", got)
			}
		})
	}
}

// A model's other qualified-name properties name documents, not constants,
// and are not reported.
func TestConstantReference_DocumentNamesAreNotConstants(t *testing.T) {
	prog := mustBuild(t, "create agent M.Helper (UsageType: Task, Model: M.GPT, SystemPrompt: 'x');")
	if got := deprecationCodes(prog); len(got) != 0 {
		t.Errorf("recorded %v", got)
	}
}
