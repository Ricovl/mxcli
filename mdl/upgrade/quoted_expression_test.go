// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"strings"
	"testing"
)

// ako/mxcli#836: the pre-#750 quoted spelling of an expression property keeps
// its meaning under mdl 0, and `fmt --upgrade` writes the expression bare —
// without the header too, since the bare form means the same under both
// versions (versionNeutral).
func TestUpgrade_QuotedExpressionIsWrittenBare(t *testing.T) {
	page := func(v string) string {
		return "create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {\n  container c1 (dynamicclasses: " + v +
			") { } -- keep\n  datagrid dg (datasource: database M.Thing) {\n    column (attribute: Name, caption: 'N', DynamicCellClass: " +
			v + ")\n  }\n};"
	}
	odata := func(user, pass string) string {
		return "create consumed odata service M.Api (\n  ODataVersion: OData4,\n  MetadataUrl: 'https://x/$metadata',\n  UseAuthentication: Yes,\n  HttpUsername: " +
			user + ",\n  HttpPassword: " + pass + "\n)\nheaders ('X-Key': " + pass + ");"
	}
	for _, tc := range []struct{ name, src, want string }{
		{"demo-2 shape", page(`'if $currentObject/X then ''on'' else '''''`), page(`if $currentObject/X then 'on' else ''`)},
		{"constant", page(`'@M.CardClass'`), page(`@M.CardClass`)},
		{"alter page set", `alter page M.P { set (DynamicClasses: '$currentObject/Style + '' card''') on c1 };`,
			`alter page M.P { set (DynamicClasses: $currentObject/Style + ' card') on c1 };`},
		{"odata client", odata(`'''admin'''`, `'@M.ApiPassword'`), odata(`'admin'`, `@M.ApiPassword`)},
		// The old describe quoted a stored compound expression whole.
		{"compound header", odata(`'user@example.com'`, `'''Bearer '' + @M.Token'`), odata(`'user@example.com'`, `'Bearer ' + @M.Token`)},
		{"constant in a call", page(`'toLowerCase(@M.Theme)'`), page(`toLowerCase(@M.Theme)`)},
		{"alter odata client", `alter consumed odata service M.Api set (HttpPassword: '@M.ApiPassword');`,
			`alter consumed odata service M.Api set (HttpPassword: @M.ApiPassword);`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, opts := range []Options{{}, {AddHeader: true}} {
				res := mustUpgrade(t, tc.src, opts)
				want := tc.want
				if opts.AddHeader {
					want = "mdl 1;\n" + want
				}
				if res.Source != want {
					t.Errorf("AddHeader=%v got:\n%s\nwant:\n%s", opts.AddHeader, res.Source, want)
				}
				if res.GatedRewritten["MDL-V1-QUOTEDEXPR"] == 0 {
					t.Errorf("AddHeader=%v GatedRewritten = %v", opts.AddHeader, res.GatedRewritten)
				}
			}
		})
	}
}

// Controls: a class name and a plain credential are strings under both
// versions, and are left alone.
func TestUpgrade_QuotedStringIsNotRewritten(t *testing.T) {
	for _, src := range []string{
		"create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) { container c1 (dynamicclasses: 'btn is-featured') { } };",
		"create consumed odata service M.Api (ODataVersion: OData4, MetadataUrl: 'https://x/$metadata', UseAuthentication: Yes, HttpUsername: 'admin');",
	} {
		res := mustUpgrade(t, src, Options{})
		if res.Source != src || res.Changed() {
			t.Errorf("rewrote a string:\n%s\n->\n%s", src, res.Source)
		}
	}
}

// A quoted expression holding an mdl 0 backslash escape is owned by the
// string-escape rewrite; this one reports rather than overlapping it.
func TestUpgrade_QuotedExpressionWithEscapeIsLeft(t *testing.T) {
	src := `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) { container c1 (dynamicclasses: 'if $currentObject/X then ''a\tb'' else ''''') { } };`
	res := mustUpgrade(t, src, Options{})
	if res.Source != src {
		t.Errorf("rewritten to %q", res.Source)
	}
	if _, err := Upgrade(src, Options{AddHeader: true}); err == nil || !strings.Contains(err.Error(), "MDL-V1-QUOTEDEXPR") {
		t.Errorf("the header must be refused over it, got %v", err)
	}
}
