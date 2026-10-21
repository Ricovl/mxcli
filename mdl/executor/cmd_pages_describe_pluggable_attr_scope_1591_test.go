// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"
)

// mendixlabs/mxcli#1591: a Custom chart inside a data view over the page
// parameter $Entry (MyFirstModule.ChartEntry), its eventDataAttribute bound to
// an attribute of ANOTHER entity, MyFirstModule.ChartContext.EventData. DESCRIBE
// printed `eventDataAttribute: EventData`, and re-running that bound the
// property to ChartEntry.EventData — CE1613 "The selected attribute
// 'MyFirstModule.ChartEntry.EventData' no longer exists".
//
// Two halves: the generic pluggable extractor shortened every attribute
// unconditionally, and a data view over a page parameter put the parameter's
// NAME in scope rather than its entity, so nothing could have compared them.
func chartInParameterDataView(eventAttr string) map[string]any {
	chart := map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "chart1",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.customchart.CustomChart",
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{"$ID": "pt-data", "PropertyKey": "dataAttribute",
						"ValueType": map[string]any{"Type": "Attribute"}},
					map[string]any{"$ID": "pt-event", "PropertyKey": "eventDataAttribute",
						"ValueType": map[string]any{"Type": "Attribute"}},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{"TypePointer": "pt-data", "Value": map[string]any{
					"AttributeRef": map[string]any{"Attribute": "MyFirstModule.ChartEntry.JsonData"}}},
				map[string]any{"TypePointer": "pt-event", "Value": map[string]any{
					"AttributeRef": map[string]any{"Attribute": eventAttr}}},
			},
		},
	}
	return map[string]any{
		"$Type": "Forms$DataView",
		"Name":  "dvEntry",
		"DataSource": map[string]any{
			"$Type": "Forms$DataViewSource",
			"EntityRef": map[string]any{
				"$Type":  "DomainModels$DirectEntityRef",
				"Entity": "MyFirstModule.ChartEntry",
			},
			"SourceVariable": map[string]any{
				"$Type":         "Forms$PageVariable",
				"PageParameter": "Entry",
			},
		},
		"Widgets": []any{chart},
	}
}

func describedChartProps(t *testing.T, eventAttr string) map[string]string {
	t.Helper()
	ctx, _ := newMockCtx(t)
	dvs := parseRawWidget(ctx, chartInParameterDataView(eventAttr))
	if len(dvs) != 1 || len(dvs[0].Children) != 1 {
		t.Fatalf("want one data view with one chart, got %+v", dvs)
	}
	got := map[string]string{}
	for _, p := range dvs[0].Children[0].ExplicitProperties {
		got[p.Key] = p.Value
	}
	return got
}

func TestDescribePluggableAttributeOfAnotherEntityStaysQualified(t *testing.T) {
	got := describedChartProps(t, "MyFirstModule.ChartContext.EventData")
	if got["eventDataAttribute"] != "MyFirstModule.ChartContext.EventData" {
		t.Errorf("eventDataAttribute = %q, want the qualified MyFirstModule.ChartContext.EventData "+
			"(bare, re-exec binds it to ChartEntry: CE1613)", got["eventDataAttribute"])
	}
	// The control: an attribute of the data view's own entity stays bare, as
	// the rest of describe writes one.
	if got["dataAttribute"] != "JsonData" {
		t.Errorf("dataAttribute = %q, want the bare JsonData", got["dataAttribute"])
	}
}

func TestDescribeParameterDataViewPutsTheEntityInScope(t *testing.T) {
	ctx, _ := newMockCtx(t)
	dvs := parseRawWidget(ctx, chartInParameterDataView("MyFirstModule.ChartEntry.JsonLayout"))
	if len(dvs) != 1 || dvs[0].EntityContext != "MyFirstModule.ChartEntry" {
		t.Fatalf("data view context = %q, want the parameter's entity MyFirstModule.ChartEntry", dvs[0].EntityContext)
	}
	if got := describedChartProps(t, "MyFirstModule.ChartEntry.JsonLayout"); got["eventDataAttribute"] != "JsonLayout" {
		t.Errorf("eventDataAttribute = %q, want the bare JsonLayout", got["eventDataAttribute"])
	}
}
