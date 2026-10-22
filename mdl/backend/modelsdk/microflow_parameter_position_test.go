// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// #993, codec engine side. The two engines derive the same grid, so a fix
// applied to one and not the other would make a parameter's placement depend on
// which engine happened to run.
func TestMicroflowParameterToGenKeepsAuthoredPosition(t *testing.T) {
	authored := &microflows.MicroflowParameter{
		Name:     "Feedback",
		Position: &model.Point{X: -77, Y: 0},
	}
	g := microflowParameterToGen(authored, 0, nil, newestFlowVersion).(*genMf.MicroflowParameter)
	if got := g.RelativeMiddlePoint(); got != "-77;0" {
		t.Errorf("authored position = %q, want -77;0", got)
	}

	// Control: unannotated parameters still derive from the index.
	derived := &microflows.MicroflowParameter{Name: "Feedback"}
	g = microflowParameterToGen(derived, 0, nil, newestFlowVersion).(*genMf.MicroflowParameter)
	if got := g.RelativeMiddlePoint(); got != "200;53" {
		t.Errorf("derived position at index 0 = %q, want 200;53", got)
	}
	g = microflowParameterToGen(derived, 1, nil, newestFlowVersion).(*genMf.MicroflowParameter)
	if got := g.RelativeMiddlePoint(); got != "300;53" {
		t.Errorf("derived position at index 1 = %q, want 300;53", got)
	}
}

// Parameters with long names were written 100px apart whatever their names
// (mendixlabs/mxcli#1598, case 5): LongParameterNameOne / Two / Three at
// 200;53, 300;53 and 400;53, labels of 20-22 characters overlapping. Studio Pro
// authors space a row by the names: in 95 Studio Pro-authored rows the median
// pitch is 95-106px for names under 15 characters and 160px for 20-24.
func TestDerivedParameterRowIsSpacedByTheWidestName(t *testing.T) {
	positions := func(names ...string) []int {
		mf := &microflows.Microflow{}
		for _, n := range names {
			mf.Parameters = append(mf.Parameters, &microflows.MicroflowParameter{Name: n})
		}
		var xs []int
		for _, el := range microflowToGen(mf, newestFlowVersion).ObjectCollection().(*genMf.MicroflowObjectCollection).ObjectsItems() {
			if p, ok := el.(*genMf.MicroflowParameter); ok {
				var x, y int
				fmt.Sscanf(p.RelativeMiddlePoint(), "%d;%d", &x, &y)
				xs = append(xs, x)
			}
		}
		return xs
	}

	long := positions("LongParameterNameOne", "LongParameterNameTwo", "LongParameterNameThree")
	// A 22-character label at Studio Pro's ~7px per character.
	const widestLabel = 22 * 7
	for i := 1; i < len(long); i++ {
		if pitch := long[i] - long[i-1]; pitch < widestLabel {
			t.Errorf("parameters %d and %d are %dpx apart, under the %dpx a 22-character name needs (row %v)",
				i-1, i, pitch, widestLabel, long)
		}
	}

	// Control: short names keep the 100px row every earlier write produced, so
	// an ordinary flow's parameters do not move.
	if short := positions("Customer", "Order"); len(short) != 2 || short[0] != 200 || short[1] != 300 {
		t.Errorf("short names moved: %v, want [200 300]", short)
	}
}

// A flow an earlier mxcli wrote has its long-named parameters on the old 100px
// row. That row is mxcli's arithmetic, not a person's placement, so a reader
// must not mistake it for one and pin it.
func TestLegacyParameterRowReadsAsDerived(t *testing.T) {
	params := []*microflows.MicroflowParameter{
		{Name: "LongParameterNameOne"}, {Name: "LongParameterNameTwo"}, {Name: "LongParameterNameThree"},
	}
	for i := range params {
		legacy := model.Point{X: 200 + i*100, Y: 53}
		if p := microflows.AuthoredParameterPosition(legacy, i, params); p != nil {
			t.Errorf("parameter %d on the old 100px row read as authored at %v", i, *p)
		}
	}
}
