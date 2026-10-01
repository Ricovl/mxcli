// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// ako/mxcli#905: a message written as an expression — `log … 'failed for ' +
// $User/Name` — is stored as the template '{1}' with the expression as its
// parameter, and describe prints it so. The two spellings are one activity;
// compared as written they were two, so a statement re-run against a flow the
// splice had grown saw its activity changed. Outside an error handler that is a
// replace the write elision absorbs; for an activity whose handler holds the
// message (CapTrack ACT_Export_Excel) it was refused on every run after the
// first ("it has an error handler, which would be left with no activity").
func TestDeclaredMatches_MessageExpressionIsItsTemplate(t *testing.T) {
	stored := parseFlowBody(t, `  log node 'N' '{1}' with ({1} = 'v ' + $In);
  show message '{1}' type Warning with ({1} = 'Hi ' + $In);
  validation feedback $In/Name message '{1}' with ({1} = 'bad ' + $In);`)
	for _, c := range []struct {
		name, body string
		want       bool
	}{
		{"as written", `  log info node 'N' 'v ' + $In;
  show message 'Hi ' + $In type Warning;
  validation feedback $In/Name message 'bad ' + $In;`, true},
		{"as described", `  log node 'N' '{1}' with ({1} = 'v ' + $In);
  show message '{1}' type Warning with ({1} = 'Hi ' + $In);
  validation feedback $In/Name message '{1}' with ({1} = 'bad ' + $In);`, true},
		// Controls: a real change is still one.
		{"control: another log text", `  log info node 'N' 'w ' + $In;
  show message 'Hi ' + $In type Warning;
  validation feedback $In/Name message 'bad ' + $In;`, false},
		{"control: another message text", `  log info node 'N' 'v ' + $In;
  show message 'Ho ' + $In type Warning;
  validation feedback $In/Name message 'bad ' + $In;`, false},
		{"control: another feedback text", `  log info node 'N' 'v ' + $In;
  show message 'Hi ' + $In type Warning;
  validation feedback $In/Name message 'worse ' + $In;`, false},
		{"control: the template as a literal", `  log info node 'N' '{1}';
  show message 'Hi ' + $In type Warning;
  validation feedback $In/Name message 'bad ' + $In;`, false},
	} {
		declared := parseFlowBody(t, c.body)
		if got := declaredMatches(declared.Body, stored.Body); got != c.want {
			t.Errorf("%s: declaredMatches = %v, want %v", c.name, got, c.want)
		}
	}
}
