// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// flowDocKeys returns the keys of one encoded document.
func flowDocKeys(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	elems, err := bson.Raw(raw).Elements()
	if err != nil {
		t.Fatalf("elements: %v", err)
	}
	out := map[string]bool{}
	for _, e := range elems {
		out[e.Key()] = true
	}
	return out
}

// TestFlowShapeByVersion pins every version-gated key of the flow writers to
// the Model SDK's floor (mendixlabs/mxcli#1410), across the bands: 9.x,
// 10.0–10.1, 10.2–10.7, 10.8–10.11, 10.12–10.14, 10.15, 10.16, 10.17+. The
// 10.12 row is measured, besides 9.24: Administration.ChangeMyPassword after
// `mx convert` to 10.12.19 stores StableId and ReturnVariableName, a
// NewCaseValue, both bezier vectors and LineType "BezierCurve", and no
// parameter IsRequired/DefaultValue.
func TestFlowShapeByVersion(t *testing.T) {
	type want struct {
		stableID, returnVar             bool
		caseList, line, lineType, param bool
	}
	all := want{stableID: true, returnVar: true, caseList: true, line: true, param: true}
	tests := []struct {
		v    flowVersion
		want want
	}{
		{flowVersion{9, 24}, want{}},
		{flowVersion{10, 1}, want{}},
		{flowVersion{10, 2}, want{stableID: true}},
		{flowVersion{10, 8}, want{stableID: true, lineType: true}},
		{flowVersion{10, 12}, want{stableID: true, returnVar: true, lineType: true}},
		{flowVersion{10, 14}, want{stableID: true, returnVar: true, lineType: true}},
		{flowVersion{10, 15}, want{stableID: true, returnVar: true, caseList: true, lineType: true}},
		{flowVersion{10, 16}, want{stableID: true, returnVar: true, caseList: true, line: true}},
		{flowVersion{10, 17}, all},
		{flowVersion{11, 6}, all},
		{newestFlowVersion, all},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d.%d", tt.v.major, tt.v.minor), func(t *testing.T) {
			mf := &microflows.Microflow{
				Name:             "MF",
				ObjectCollection: decisionFlow(),
				Parameters:       []*microflows.MicroflowParameter{{Name: "P", Type: &microflows.BooleanType{}}},
			}
			mf.ObjectCollection.AnnotationFlows = []*microflows.AnnotationFlow{{
				BaseElement:   model.BaseElement{ID: "a1410000-0000-0000-0000-000000000031"},
				OriginID:      "a1410000-0000-0000-0000-000000000001",
				DestinationID: "a1410000-0000-0000-0000-000000000002",
			}}
			gm := microflowToGen(mf, tt.v)
			gm.SetID("a1410000-0000-0000-0000-000000000000")
			assignMicroflowIDs(gm)
			raw, err := microflowEncoder(tt.v).Encode(gm)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			check := func(what string, got, want bool) {
				t.Helper()
				if got != want {
					t.Errorf("%s present = %v, want %v", what, got, want)
				}
			}
			top := flowDocKeys(t, raw)
			check("Microflow.StableId", top["StableId"], tt.want.stableID)
			check("Microflow.ReturnVariableName", top["ReturnVariableName"], tt.want.returnVar)

			flows, _ := bson.Raw(raw).Lookup("Flows").Array().Values()
			var seq, ann map[string]bool
			for _, f := range flows {
				d, ok := f.DocumentOK()
				if !ok {
					continue
				}
				switch typ, _ := d.Lookup("$Type").StringValueOK(); typ {
				case "Microflows$SequenceFlow":
					seq = flowDocKeys(t, d)
				case "Microflows$AnnotationFlow":
					ann = flowDocKeys(t, d)
				}
			}
			for name, keys := range map[string]map[string]bool{"SequenceFlow": seq, "AnnotationFlow": ann} {
				check(name+".Line", keys["Line"], tt.want.line)
				check(name+".OriginBezierVector", keys["OriginBezierVector"], !tt.want.line)
				check(name+".DestinationBezierVector", keys["DestinationBezierVector"], !tt.want.line)
				check(name+".LineType", keys["LineType"], tt.want.lineType)
			}
			check("SequenceFlow.CaseValues", seq["CaseValues"], tt.want.caseList)
			check("SequenceFlow.NewCaseValue", seq["NewCaseValue"], !tt.want.caseList)
			check("SequenceFlow.CaseValue (the SDK name, never stored)", seq["CaseValue"], false)

			objs, _ := bson.Raw(raw).Lookup("ObjectCollection", "Objects").Array().Values()
			found := false
			for _, o := range objs {
				d, ok := o.DocumentOK()
				if !ok {
					continue
				}
				if typ, _ := d.Lookup("$Type").StringValueOK(); typ == "Microflows$MicroflowParameter" {
					found = true
					keys := flowDocKeys(t, d)
					check("MicroflowParameter.IsRequired", keys["IsRequired"], tt.want.param)
					check("MicroflowParameter.DefaultValue", keys["DefaultValue"], tt.want.param)
				}
			}
			if !found {
				t.Errorf("no Microflows$MicroflowParameter written")
			}
		})
	}
}
