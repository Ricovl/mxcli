// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Studio Pro's stored shape (Administration.Account_Edit, Mendix 11.13.0):
//
//	ConditionalVisibilitySettings: Forms$ConditionalVisibilitySettings
//	  Attribute: "Administration.Account.IsLocalUser"
//	  Conditions [marker=2]:
//	    - Enumerations$Condition { AttributeValue: "true",  EditableVisible: false }
//	    - Enumerations$Condition { AttributeValue: "false", EditableVisible: true }
//	  Expression: ""
//	  IgnoreSecurity: false
//	  ModuleRoles [marker=1]: []
//	  SourceVariable: null
//
// Markers: Conditions is [2] and ModuleRoles [1] on all 12 conditional
// settings in that project, empty or not; mxcli wrote [3] for both.
func TestConditionalVisibilityToGen_AttributeConditions(t *testing.T) {
	d := encodeToD(t, conditionalVisibilityToGen(&pages.ConditionalVisibilitySettings{
		Attribute: "Administration.Account.IsLocalUser",
		Conditions: []pages.ValueCondition{
			{Value: "true", Visible: false},
			{Value: "false", Visible: true},
		},
	}))
	get := func(k string) any {
		for _, e := range d {
			if e.Key == k {
				return e.Value
			}
		}
		t.Fatalf("key %q missing: %v", k, d)
		return nil
	}
	if get("Attribute") != "Administration.Account.IsLocalUser" {
		t.Errorf("Attribute = %v", get("Attribute"))
	}
	conds, ok := get("Conditions").(bson.A)
	if !ok || len(conds) != 3 || conds[0] != int32(2) {
		t.Fatalf("Conditions = %#v, want [2, cond, cond]", get("Conditions"))
	}
	first, _ := conds[1].(bson.D)
	want := map[string]any{"$Type": "Enumerations$Condition", "AttributeValue": "true", "EditableVisible": false}
	for k, v := range want {
		found := false
		for _, e := range first {
			if e.Key == k {
				found = true
				if e.Value != v {
					t.Errorf("condition[0].%s = %v, want %v", k, e.Value, v)
				}
			}
		}
		if !found {
			t.Errorf("condition[0] lacks %s: %v", k, first)
		}
	}
	if roles, _ := get("ModuleRoles").(bson.A); len(roles) != 1 || roles[0] != int32(1) {
		t.Errorf("ModuleRoles = %#v, want [1]", get("ModuleRoles"))
	}
}

// Read from a snippet parameter (TestApp
// WorkflowCommons.Snip_UserTask_NameColumnWithIcon, an image outside every data
// container): SourceVariable is a Forms$PageVariable naming the parameter in
// SnippetParameter, no Widget. Null for a context-relative condition.
func TestConditionalVisibilityToGen_ParameterSource(t *testing.T) {
	encodeSV := func(sv *pages.WidgetVariable) any {
		d := encodeToD(t, conditionalVisibilityToGen(&pages.ConditionalVisibilitySettings{
			Attribute:      "System.WorkflowUserTask.CompletionType",
			Conditions:     []pages.ValueCondition{{Value: "Single", Visible: true}},
			SourceVariable: sv,
		}))
		for _, e := range d {
			if e.Key == "SourceVariable" {
				return e.Value
			}
		}
		t.Fatalf("no SourceVariable key: %v", d)
		return nil
	}
	sv, ok := encodeSV(&pages.WidgetVariable{Variable: "WorkflowUserTask", Kind: "snippet"}).(bson.D)
	if !ok {
		t.Fatal("SourceVariable not written")
	}
	want := map[string]any{"$Type": "Forms$PageVariable", "SnippetParameter": "WorkflowUserTask",
		"PageParameter": "", "LocalVariable": "", "Widget": ""}
	for _, e := range sv {
		if w, ok := want[e.Key]; ok {
			if e.Value != w {
				t.Errorf("SourceVariable.%s = %#v, want %#v", e.Key, e.Value, w)
			}
			delete(want, e.Key)
		}
	}
	for k := range want {
		t.Errorf("SourceVariable lacks %s: %v", k, sv)
	}
	// Control: no source, null.
	if got := encodeSV(nil); got != nil {
		t.Errorf("context-relative SourceVariable = %#v, want null", got)
	}
}

// Editability's empty Conditions list carries marker [2], as visibility's does:
// measured on both Studio Pro-authored settings in TestApp (the combo boxes of
// WorkflowCommons.Snip_TaskDashboard_Header and Snip_WorkflowDashboard_TaskNumbers,
// `Editable: $DashboardContext/… != empty`). mxcli wrote the default [3], so a
// describe → exec of either rewrote the snippet.
func TestConditionalEditabilityToGen_EmptyConditionsMarker(t *testing.T) {
	d := encodeToD(t, conditionalEditabilityToGen(&pages.ConditionalEditabilitySettings{Expression: "$currentObject/Name != empty"}))
	for _, e := range d {
		if e.Key == "Conditions" {
			if a, _ := e.Value.(bson.A); len(a) != 1 || a[0] != int32(2) {
				t.Errorf("empty Conditions = %#v, want [2]", e.Value)
			}
			return
		}
	}
	t.Fatalf("no Conditions key: %v", d)
}

// The expression form keeps its shape, with the corrected empty markers.
func TestConditionalVisibilityToGen_ExpressionMarkers(t *testing.T) {
	d := encodeToD(t, conditionalVisibilityToGen(&pages.ConditionalVisibilitySettings{Expression: "$currentObject/ImageB64 != empty"}))
	for _, e := range d {
		switch e.Key {
		case "Conditions":
			if a, _ := e.Value.(bson.A); len(a) != 1 || a[0] != int32(2) {
				t.Errorf("empty Conditions = %#v, want [2]", e.Value)
			}
		case "ModuleRoles":
			if a, _ := e.Value.(bson.A); len(a) != 1 || a[0] != int32(1) {
				t.Errorf("empty ModuleRoles = %#v, want [1]", e.Value)
			}
		case "Attribute":
			if e.Value != "" {
				t.Errorf("Attribute = %v, want \"\"", e.Value)
			}
		}
	}
}
