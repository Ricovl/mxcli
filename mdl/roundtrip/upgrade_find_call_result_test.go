// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"path/filepath"
	"strings"
	"testing"

	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
)

// flowsAnswer is an upgrade.FlowTypes with fixed answers, for the control.
type flowsAnswer map[string]bool

func (f flowsAnswer) ReturnsString(_ bool, qn string) (bool, bool) {
	v, ok := f[qn]
	return v, ok
}

// `find(…)` over a flow call's result (ako/mxcli#860): the upgrade reads the
// called flow's return type from the script, or from the project with -p, and
// picks the string function or the List operation. mdl 0 made that choice in
// the flow builder at exec time; the upgraded mdl 1 script must build what it
// built. Run both on PedApp, whose FeedbackModule.ConvertUUIDToURL (a String)
// and Atlas_Web_Content.DS_LoginContext (an object) are Studio Pro-authored.
func TestUpgradeFindOnACallResult_ExecutesToTheSameModel(t *testing.T) {
	a, b := newHarness(t), newHarness(t)
	defer a.close()
	defer b.close()

	project := modelsdkbackend.New()
	if err := project.ConnectReadOnly(filepath.Join(pedApp.dir, pedApp.mpr)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = project.Disconnect() }()
	flows := executor.NewFlowReturnTypes(project)

	const script = `create module UpFind;
create or modify persistent entity UpFind.E (Name: String(100));
create or modify microflow UpFind.Regions () returns List of UpFind.E as $R begin
  retrieve $R from UpFind.E;
  return $R;
end;
create or modify microflow UpFind.Label () returns String begin
  return 'x';
end;
create or modify microflow UpFind.F ($o: UpFind.E, $f: FeedbackModule.Feedback) begin
  declare $Pos Integer = 0;
  declare $At Integer = 0;
  $Url = call microflow FeedbackModule.ConvertUUIDToURL(uuid = 'x');
  $Pos = find($Url, 'x');
  $Lbl = call microflow UpFind.Label();
  $At = find($Lbl, 'y');
  $R = call microflow UpFind.Regions();
  $Hit = find($R, $currentObject = $o);
  $Resp = call microflow FeedbackModule.SUB_Feedback_PostToAppInsights(Feedback = $f);
  $Hit2 = find($Resp, $currentObject = $o);
end;
`
	res, err := upgrade.Upgrade(script, upgrade.Options{AddHeader: true, Flows: flows})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\n  set $Pos = find($Url, 'x');\n",                   // the project's flow returns a String
		"\n  set $At = find($Lbl, 'y');\n",                    // so does the script's
		"\n  $Hit = find $R where $currentObject = $o;\n",     // the script's returns a list
		"\n  $Hit2 = find $Resp where $currentObject = $o;\n", // the project's returns an object
	} {
		if !strings.Contains(res.Source, want) {
			t.Fatalf("want %q in the upgrade:\n%s", want, res.Source)
		}
	}
	errA, errB, diff := executeBoth(t, a, b, script, res.Source)
	if errA != nil || errB != nil {
		t.Fatalf("original error: %v\nupgraded error: %v", errA, errB)
	}
	if len(diff) > 0 {
		t.Fatalf("the upgrade changed what the script writes:\n  %s", strings.Join(diff, "\n  "))
	}

	// Control: a wrong reading must register, or the comparison above could
	// not tell the two apart. The dangerous one is a non-String read as a
	// String: `set $x = find(…)` is the string function under mdl 1. (The
	// reverse is not visible in the model: the flow builder turns a List
	// operation over a declared String into the string function under every
	// version — addListOperationAction.)
	wrong, err := upgrade.Upgrade(script, upgrade.Options{AddHeader: true,
		Flows: flowsAnswer{"FeedbackModule.ConvertUUIDToURL": true, "FeedbackModule.SUB_Feedback_PostToAppInsights": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wrong.Source, "\n  set $Hit2 = find($Resp, $currentObject = $o);\n") {
		t.Fatalf("the control did not take the String reading:\n%s", wrong.Source)
	}
	errA, errB, diff = executeBoth(t, a, b, script, wrong.Source)
	if len(diff) == 0 && (errA == nil) == (errB == nil) {
		t.Fatal("building the string function where mdl 0 built the List operation registered as the same model")
	}
	t.Logf("the wrong reading registered as: errA=%v errB=%v diff=%v", errA, errB, diff)
}
