// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// An error handler's row sat 90px under its activity (mendixlabs/mxcli#1598,
// case 3): a 30px gap, under the output variable and type Studio Pro prints
// below the box (about 35px of text, see RowGap). Studio Pro's own handler rows
// sit a median 130px below the activity — 160 distinct rows measured in
// ako/TestApp, PedApp and seven customer apps, quartiles 125..150.
func TestErrorHandlerRowClearsTheOutputVariableLabel(t *testing.T) {
	fb := buildFlowFromMDL(t, `
  retrieve $List from MyFirstModule.Item on error begin log error node 'x' 'failed'; end error;
  log info node 'x' 'done';`)
	var retrieve, handler microflows.MicroflowObject
	for _, f := range fb.flows {
		if f.IsErrorHandler {
			retrieve, handler = fb.objectByID(f.OriginID), fb.objectByID(f.DestinationID)
		}
	}
	if retrieve == nil || handler == nil {
		t.Fatal("no error handler flow built")
	}
	// Studio Pro's median, written out rather than read from the constant the
	// builder uses, so the test cannot agree with whatever the constant says.
	const studioProHandlerPitch = 130
	if dy := handler.GetPosition().Y - retrieve.GetPosition().Y; dy < studioProHandlerPitch {
		t.Errorf("handler row %dpx under its activity, want at least %d", dy, studioProHandlerPitch)
	}
}

// A handler in the THEN branch is laid out under its activity, and the ELSE row
// is laid out under the THEN row — from a measured THEN height that left the
// handler out, so the two rows overlapped.
func TestErrorHandlerInTheThenBranchClearsTheElseRow(t *testing.T) {
	fb := buildFlowFromMDL(t, `
  if $Flag then
    retrieve $List from MyFirstModule.Item on error begin log error node 'x' 'failed'; end error;
    log info node 'x' 'then';
  else
    log info node 'x' 'else';
  end if;`)
	var boxes []canvasBox
	var names []string
	for _, o := range fb.objects {
		if _, ok := o.(*microflows.ActionActivity); !ok {
			continue
		}
		p, sz := o.GetPosition(), objectSize(o)
		boxes = append(boxes, canvasBox{p.X - sz.Width/2, p.Y - sz.Height/2, p.X + sz.Width/2, p.Y + sz.Height/2})
		names = append(names, activitySummary(o))
	}
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			if a.left < b.right && b.left < a.right && a.top < b.bottom && b.top < a.bottom {
				t.Errorf("%s %+v overlaps %s %+v", names[i], a, names[j], b)
			}
		}
	}
}

func activitySummary(o microflows.MicroflowObject) string {
	if a, ok := o.(*microflows.ActionActivity); ok {
		switch act := a.Action.(type) {
		case *microflows.LogMessageAction:
			return "log"
		default:
			_ = act
			return "activity"
		}
	}
	return "object"
}
