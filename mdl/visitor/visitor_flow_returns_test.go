// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#977 item 3: a flow call's output mappings (Mendix 11.5, Studio Pro's
// "Return value mappings") had no MDL, so describe → exec could only carry them
// and a script could not write one. `returns ( … )` after the arguments spells
// them, target = source like the argument list.
package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestFlowReturns(t *testing.T) {
	for _, call := range []string{"call microflow", "call nanoflow"} {
		a, errs := buttonAction(t, call+" M.F(Vehicle = $currentObject) returns ("+
			"Brand = $ActionReturnValue/Brand, $stringResult = $ActionReturnValue, "+
			"\"Type\" = 'Demo brand',) with (ProgressBar: blocking)")
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", call, errs)
		}
		want := []ast.FlowReturnV3{
			{Target: "Brand", Source: "$ActionReturnValue/Brand"},
			{Target: "stringResult", Variable: true, Source: "$ActionReturnValue"},
			{Target: "Type", Source: "'Demo brand'"},
		}
		if !reflect.DeepEqual(a.Returns, want) {
			t.Errorf("%s: Returns = %+v, want %+v", call, a.Returns, want)
		}
		if len(a.Args) != 1 || a.Settings == nil || a.Settings.ProgressBar != "Blocking" {
			t.Errorf("%s: the arguments or settings around returns were lost: %+v", call, a)
		}
	}
}

// Control: a call without the list has no mappings.
func TestFlowReturns_AbsentIsNil(t *testing.T) {
	a, errs := buttonAction(t, "call microflow M.F")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if a.Returns != nil {
		t.Errorf("Returns = %+v, want nil", a.Returns)
	}
}
