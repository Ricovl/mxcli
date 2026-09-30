// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#859 (rehearsal M4): a `$` inside a string literal is text, not a
// variable reference, for the splice's scope checks.
func TestWithoutStringLiterals(t *testing.T) {
	for in, want := range map[string][]string{
		`set $Url = '/odata/v1?$filter=' + $Filter;`: {"Url", "Filter"},
		`log info node 'N' 'it''s $x' + $y;`:         {"y"},
		`$A = $B + '' + $C;`:                         {"A", "B", "C"},
		`set $S = 'a\' + $T;`:                        {"S", "T"}, // mdl 1: a backslash is a character
	} {
		var got []string
		for _, m := range variableRef.FindAllStringSubmatch(withoutStringLiterals(in), -1) {
			got = append(got, m[1])
		}
		if len(got) != len(want) {
			t.Errorf("%s: variables %v, want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: variables %v, want %v", in, got, want)
				break
			}
		}
	}
}
