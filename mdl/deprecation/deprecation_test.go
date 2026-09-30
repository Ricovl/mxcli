// SPDX-License-Identifier: Apache-2.0

package deprecation

import (
	"regexp"
	"strings"
	"testing"
)

// The registry is append-only and its codes are what scripts, CI allowlists
// and docs key on, so each entry must be complete and its code well-formed and
// unique.
func TestRegistryEntriesAreWellFormed(t *testing.T) {
	code := regexp.MustCompile(`^MDL-DEPR\d{3}$`)
	seen := map[string]bool{}
	for _, e := range All() {
		if !code.MatchString(e.Code) {
			t.Errorf("code %q is not MDL-DEPRnnn", e.Code)
		}
		if seen[e.Code] {
			t.Errorf("code %s registered twice", e.Code)
		}
		seen[e.Code] = true
		if !IsDeprecationCode(e.Code) {
			t.Errorf("IsDeprecationCode(%q) = false", e.Code)
		}
		if got, ok := Lookup(e.Code); !ok || got.Code != e.Code {
			t.Errorf("Lookup(%q) = %v, %v", e.Code, got.Code, ok)
		}
		swap := e.Rewrite.Token != "" && e.Rewrite.Replacement != ""
		if swap == (e.Rewrite.Structural != "") {
			t.Errorf("%s: a rewrite is either a keyword swap or structural, exactly one: %+v", e.Code, e.Rewrite)
		}
		// The warning prints "Rewrite the <Structural>", so a Structural that
		// starts with "the" reads "Rewrite the the …".
		if strings.HasPrefix(strings.ToLower(e.Rewrite.Structural), "the ") {
			t.Errorf("%s: Structural %q starts with \"the\"; it is printed after \"Rewrite the\"", e.Code, e.Rewrite.Structural)
		}
		if e.Old == "" || e.Canonical == "" || e.Example == "" || e.CanonicalExample == "" {
			t.Errorf("%s is incomplete: %+v", e.Code, e)
		}
		// ADR-0011: an alias warns under the version that deprecates it (1)
		// and is refused from a later one.
		// The exception is a form mdl 1 never had: a no-op that parsed under
		// mdl 0 only by accident, refused from mdl 1 (ADR-0011: only the
		// header refuses). Listed by code, so a new one is a decision.
		if e.RemovedIn == 1 && refusedFromMdl1[e.Code] {
			continue
		}
		if e.RemovedIn < 2 {
			t.Errorf("%s RemovedIn = %d, want >= 2", e.Code, e.RemovedIn)
		}
	}
	if _, ok := Lookup("MDL-DEPR999"); ok {
		t.Error("Lookup found an unregistered code")
	}
	if IsDeprecationCode("MDL065") {
		t.Error("MDL065 treated as a registry code")
	}
}

// A typo in --deprecations must be an error, not a silent `warn`: that would
// let a CI gate pass that was meant to fail.
func TestParsePolicy(t *testing.T) {
	for in, want := range map[string]Policy{"": Warn, "warn": Warn, "error": Error, " ERROR ": Error} {
		got, err := ParsePolicy(in)
		if err != nil || got != want {
			t.Errorf("ParsePolicy(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"errors", "fail", "warning", "bogus"} {
		if _, err := ParsePolicy(in); err == nil {
			t.Errorf("ParsePolicy(%q) accepted an unknown value", in)
		}
	}
}

// refusedFromMdl1 are the entries whose old form mdl 1 never accepted.
var refusedFromMdl1 = map[string]bool{
	ConstantPrivate: true, // a no-op the old catch-all swallowed (ako/mxcli#865)
}
