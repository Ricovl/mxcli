// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// altMixin returns the alt-palette mixin of a scaffolded theme's partial,
// header line included, so a test can compare it and check its layout.
func altMixin(t *testing.T, partial, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)@mixin\s+mxcli-` + regexp.QuoteMeta(name) + `-(?:dark|light)\s*\{.*?\n\}`).FindString(partial)
	if m == "" {
		t.Fatalf("no alt-palette mixin in the scaffolded partial:\n%s", partial)
	}
	return m
}

// scaffoldPartial creates a theme from the design css on base and returns its
// partial and the CreateResult.
func scaffoldPartial(t *testing.T, base, name, css string) (string, *CreateResult) {
	t.Helper()
	dir := newProject(t)
	design := filepath.Join(dir, "design.css")
	write(t, design, css)
	res, err := Create(dir, name, CreateOptions{From: design, Base: base})
	if err != nil {
		t.Fatal(err)
	}
	return read(t, filepath.Join(res.Dir, "files", "theme", "web", "_mxcli-"+name+".scss")), res
}

// ako/mxcli#970: a design that declares only a base palette describes ONE
// variant. Copying its light ground and ink into the dark mixin, whose other
// surfaces stay dark, gave an unreadable mixed palette. With no block for the
// alt variant, the mixin must come out exactly as the base theme ships it.
func TestCreate_BaseOnlyDesignLeavesTheAltMixinAlone(t *testing.T) {
	baseOnly := `:root { --mxt-brand: #10069f; --mxt-ground: #f7f9fb; --mxt-ink: #1b2733; }`
	for _, base := range []string{"signal", "console", "ledger"} {
		t.Run(base, func(t *testing.T) {
			// The control: the same scaffold with no design at all.
			ctlDir := newProject(t)
			ctl, err := Create(ctlDir, "probe", CreateOptions{From: base})
			if err != nil {
				t.Fatal(err)
			}
			want := altMixin(t, read(t, filepath.Join(ctl.Dir, "files", "theme", "web", "_mxcli-probe.scss")), "probe")

			partial, res := scaffoldPartial(t, base, "probe", baseOnly)
			if got := altMixin(t, partial, "probe"); got != want {
				t.Errorf("a base-only design rewrote the alt-palette mixin.\n got:\n%s\nwant:\n%s", got, want)
			}
			if res.UnseededVariant == "" {
				t.Errorf("the result must name the variant the design did not seed, so the CLI can say so")
			}
		})
	}
}

// The other half of the decision: a design that DOES declare the alt variant
// still seeds it — the fix keys on the block, not on giving up.
func TestCreate_DesignWithAVariantBlockSeedsTheAltMixin(t *testing.T) {
	for base, block := range map[string]string{
		"signal":  `@media (prefers-color-scheme: dark) { :root { --mxt-brand: #a78bfa; } }`,
		"ledger":  `@media (prefers-color-scheme: dark) { :root { --mxt-brand: #a78bfa; } }`,
		"console": `@media (prefers-color-scheme: light) { :root { --mxt-brand: #a78bfa; } }`,
	} {
		t.Run(base, func(t *testing.T) {
			partial, res := scaffoldPartial(t, base, "probe", `:root { --mxt-brand: #10069f; }`+"\n"+block)
			mixin := altMixin(t, partial, "probe")
			if !strings.Contains(mixin, "--mxt-brand: #a78bfa;") {
				t.Errorf("the design's alt-variant brand did not reach the mixin:\n%s", mixin)
			}
			if strings.Contains(mixin, "#10069f") {
				t.Errorf("the base palette leaked into the alt mixin:\n%s", mixin)
			}
			if res.UnseededVariant != "" {
				t.Errorf("UnseededVariant = %q, want empty: the design declared that variant", res.UnseededVariant)
			}
		})
	}
}

// The cosmetic half of #970: the first rewritten token used to land on the
// `@mixin … {` line itself, unindented, because the match's leading \s*
// swallowed the newline after the brace.
func TestApplyTokens_KeepsTheFirstDeclarationOnItsOwnLine(t *testing.T) {
	body := "\n  --mxt-brand: #2aa39f;\n  --mxt-ink: #000;\n"
	got, unplaced := applyTokens(body, TokenSet{"--mxt-brand": "#a78bfa"})
	if len(unplaced) != 0 {
		t.Fatalf("unplaced = %v", unplaced)
	}
	want := "\n  --mxt-brand: #a78bfa;\n  --mxt-ink: #000;\n"
	if got != want {
		t.Errorf("applyTokens moved the declaration.\n got: %q\nwant: %q", got, want)
	}
}
