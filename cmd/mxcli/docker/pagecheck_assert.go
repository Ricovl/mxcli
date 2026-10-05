// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"strconv"
	"strings"
)

// pagecheck_assert.go turns page signals into a pass/fail answer.
//
// The verdict line says what the page looks like; this decides whether that is
// a failure, so `mxcli playwright check` can exit non-zero and a script (or an
// agent) can branch on the exit code instead of reading the output.

// CountAssertion is one `--assert-count` check: SELECTOR OP N.
type CountAssertion struct {
	Raw      string
	Selector string
	Op       string // >=, <=, ==, !=, >, <
	N        int
}

// ParseCountAssertion parses `selector>=N`-style assertions. The operator is
// read from the RIGHT, so a selector may use the child combinator:
// `.mx-listview>ul>li>=1` is selector `.mx-listview>ul>li`, op `>=`, 1.
// A bare selector means "at least one".
func ParseCountAssertion(s string) (CountAssertion, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return CountAssertion{}, fmt.Errorf("empty --assert-count")
	}
	// Trailing digits.
	i := len(raw)
	for i > 0 && raw[i-1] >= '0' && raw[i-1] <= '9' {
		i--
	}
	if i == len(raw) {
		return CountAssertion{Raw: raw, Selector: raw, Op: ">=", N: 1}, nil
	}
	n, err := strconv.Atoi(raw[i:])
	if err != nil {
		return CountAssertion{}, fmt.Errorf("--assert-count %q: %w", raw, err)
	}
	rest := strings.TrimRight(raw[:i], " ")
	op := ""
	for _, cand := range []string{">=", "<=", "==", "!=", ">", "<", "="} {
		if strings.HasSuffix(rest, cand) {
			op = cand
			break
		}
	}
	if op == "" {
		// Digits with no operator are part of the selector (e.g. `.col-6`).
		return CountAssertion{Raw: raw, Selector: raw, Op: ">=", N: 1}, nil
	}
	sel := strings.TrimSpace(strings.TrimSuffix(rest, op))
	if sel == "" {
		return CountAssertion{}, fmt.Errorf("--assert-count %q has no selector", raw)
	}
	if op == "=" {
		op = "=="
	}
	return CountAssertion{Raw: raw, Selector: sel, Op: op, N: n}, nil
}

// Holds reports whether a measured count satisfies the assertion. A negative
// count means the selector itself was invalid, which never holds.
func (c CountAssertion) Holds(got int) bool {
	if got < 0 {
		return false
	}
	switch c.Op {
	case ">=":
		return got >= c.N
	case "<=":
		return got <= c.N
	case "==":
		return got == c.N
	case "!=":
		return got != c.N
	case ">":
		return got > c.N
	case "<":
		return got < c.N
	}
	return false
}

// CheckSpec is what a page must satisfy beyond rendering cleanly.
type CheckSpec struct {
	Texts  []string         // --assert-text: body text must contain each
	Counts []CountAssertion // --assert-count
	// AllowConsoleErrors keeps console errors in the verdict but out of the
	// pass/fail decision, for apps with known console noise.
	AllowConsoleErrors bool
}

// CountSelectors lists the selectors the probe must count.
func (s CheckSpec) CountSelectors() []string {
	out := make([]string, len(s.Counts))
	for i, c := range s.Counts {
		out[i] = c.Selector
	}
	return out
}

// EvaluatePage returns the reasons a page fails, empty when it passes.
//
// A page fails when the browser was refused it (HTTP >= 400), when it shows
// the sign-in form instead of itself, when it shows an error banner or error
// dialog, when the console reports an error, when a same-origin request it
// made failed, or when an assertion does not hold. Warnings and validation
// messages are reported in the verdict but do not fail it.
func EvaluatePage(sig PageSignals, spec CheckSpec) []string {
	var fails []string
	if sig.Status >= 400 {
		fails = append(fails, fmt.Sprintf("HTTP %d", sig.Status))
	}
	switch {
	case sig.LoginForm:
		fails = append(fails, "the sign-in form is showing, not the page (pass --user/--password or --role)")
	case sig.NeedsLogin():
		fails = append(fails, "not signed in: the runtime answered 401 (pass --user/--password or --role)")
	}
	if n := len(sig.ErrorAlerts); n > 0 {
		fails = append(fails, fmt.Sprintf("%d error banner(s)", n))
	}
	// On the sign-in form the client's own 401s are the consequence of not
	// being signed in, not separate failures; counting them would bury the one
	// reason that matters.
	consoleErrs, httpErrs := sig.ConsoleErrors, sig.HTTPErrors
	if sig.NeedsLogin() {
		consoleErrs, httpErrs = without401(consoleErrs), without401(httpErrs)
	}
	if n := len(consoleErrs); n > 0 && !spec.AllowConsoleErrors {
		fails = append(fails, fmt.Sprintf("%d console error(s)", n))
	}
	if n := len(httpErrs); n > 0 {
		fails = append(fails, fmt.Sprintf("%d failed request(s)", n))
	}
	for i, t := range spec.Texts {
		if i >= len(sig.TextFound) || !sig.TextFound[i] {
			fails = append(fails, fmt.Sprintf("assert-text %q not found", clip(t, 60)))
		}
	}
	for i, c := range spec.Counts {
		got := -1
		if i < len(sig.Counts) {
			got = sig.Counts[i]
		}
		if !c.Holds(got) {
			if got < 0 {
				fails = append(fails, fmt.Sprintf("assert-count %q: selector did not evaluate", c.Raw))
			} else {
				fails = append(fails, fmt.Sprintf("assert-count %q: got %d", c.Raw, got))
			}
		}
	}
	return fails
}

// NeedsLogin reports that the page was not shown because the browser is not
// signed in: the sign-in form is up, or the runtime refused the client's
// session with a 401. A session the runtime has forgotten (it restarted) shows
// the second: the client gets a 401 and renders nothing, with no form at all.
func (s PageSignals) NeedsLogin() bool {
	if s.LoginForm || s.Status == 401 {
		return true
	}
	for _, h := range s.HTTPErrors {
		if strings.HasPrefix(h, "401 ") {
			return true
		}
	}
	return false
}

func without401(ss []string) []string {
	var out []string
	for _, s := range ss {
		if !strings.Contains(s, "401") {
			out = append(out, s)
		}
	}
	return out
}

// formatCheckResult is the verdict plus a FAIL line per reason.
func formatCheckResult(label string, sig PageSignals, fails []string) string {
	var b strings.Builder
	b.WriteString(formatPageVerdict(label, sig))
	for _, f := range fails {
		fmt.Fprintf(&b, "  FAIL   %s\n", f)
	}
	if sig.Screenshot != "" {
		fmt.Fprintf(&b, "  PNG    %s\n", sig.Screenshot)
	}
	return b.String()
}
