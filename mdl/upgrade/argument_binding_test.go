// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// ako/mxcli#751 (R4): fmt --upgrade rewrites every argument to `Param =
// expression` and every positional text-template list to `with ({n} = …)`,
// touching nothing else — comments, layout, keyword case and the arguments'
// own text survive.
func TestUpgrade_ArgumentBinding(t *testing.T) {
	src := `create microflow M.F ($O: M.E, $N: String) begin
  call microflow M.G($Order = $O, Force = false); -- keep me
  show page M.P(Order: $O);
  SHOW MESSAGE 'Hi {1} {2}' TYPE Warning OBJECTS [$N, $O/Name] BLOCKING;
  validation feedback $O/Name message '{1}' objects [ $N ];
  log info 'x {1}' parameters ['a'];
  $R = send rest request M.C.Get with ($id = $N);
end;
create page M.Q (Title: 'Q', Layout: Atlas_Core.Atlas_Default) {
  dataview dv (DataSource: $O) {
    actionbutton b (Caption: 'Go', Action: microflow M.G(Order:$currentObject))
  }
};
create workflow M.W parameter $WorkflowContext: M.E begin
  call microflow M.G as act1 comment 'Go'
    with (M.G.Order = '$WorkflowContext');
end workflow;
`
	want := `create microflow M.F ($O: M.E, $N: String) begin
  call microflow M.G(Order = $O, Force = false); -- keep me
  show page M.P(Order = $O);
  SHOW MESSAGE 'Hi {1} {2}' TYPE Warning WITH ({1} = $N, {2} = $O/Name) BLOCKING;
  validation feedback $O/Name message '{1}' with ( {1} = $N );
  log info 'x {1}' with ({1} = 'a');
  $R = send rest request M.C.Get with (id = $N);
end;
create page M.Q (Title: 'Q', Layout: Atlas_Core.Atlas_Default) {
  dataview dv (DataSource: $O) {
    actionbutton b (Caption: 'Go', Action: call microflow M.G(Order = $currentObject))
  }
};
create workflow M.W parameter $WorkflowContext: M.E begin
  call microflow M.G(Order = $WorkflowContext) as act1 comment 'Go';
end workflow;
`
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	for code, n := range map[string]int{
		deprecation.DollarArgumentName:          2,
		deprecation.ColonArgument:               2,
		deprecation.WorkflowStringArgument:      1,
		deprecation.PositionalTemplateArguments: 3,
	} {
		if res.Rewritten[code] != n {
			t.Errorf("Rewritten[%s] = %d, want %d (all: %v)", code, res.Rewritten[code], n, res.Rewritten)
		}
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("a second upgrade changed the script again:\n%s", again.Source)
	}
}

// A workflow string argument that does not read back as the same bare
// expression is left as it is and reported, never guessed at.
func TestUpgrade_WorkflowStringArgumentWithoutBareForm(t *testing.T) {
	src := "create workflow M.W parameter $WorkflowContext: M.E begin\n" +
		"  call microflow M.G with (Order = ' $WorkflowContext');\nend workflow;\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != src {
		t.Errorf("the script changed:\n%s", res.Source)
	}
	if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.WorkflowStringArgument {
		t.Errorf("Unrewritten = %+v, want the one MDL-DEPR008 use", res.Unrewritten)
	}
}
