// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
)

// ako/mxcli#1717: `Annotation` reaches the mutator as the workflow's note
// without `on` and the activity's with it, and the empty string reaches it as
// the text that removes the note. A note is no template, so no parameters are
// set.
func TestAlterWorkflow_AnnotationReachesTheMutator(t *testing.T) {
	var calls []string
	rec := func(format string, a ...any) { calls = append(calls, fmt.Sprintf(format, a...)) }
	mut := &mock.MockWorkflowMutator{
		SetPropertyFunc: func(prop, v string) error { rec("set %s=%q", prop, v); return nil },
		SetActivityPropertyFunc: func(ref string, pos int, prop, v string) error {
			rec("set %s=%q on %s@%d", prop, v, ref, pos)
			return nil
		},
		SetTemplateParametersFunc: func(ref string, _ int, prop string, _ []string) error {
			rec("params %s on %q", prop, ref)
			return nil
		},
	}
	ctx := genericWfCtx(t, mut)
	err := execAlterWf(t, ctx, `alter workflow Sales.WF {
  set (Annotation: 'Started from the request form');
  set (Annotation: 'Check it') on task1;
  set (Annotation: '') on 'Review'@2;
};`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`set annotation="Started from the request form"`,
		`set annotation="Check it" on task1@0`,
		`set annotation="" on Review@2`,
	}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %v\nwant    %v", calls, want)
	}
}
