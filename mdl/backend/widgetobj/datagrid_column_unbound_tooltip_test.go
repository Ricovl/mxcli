// SPDX-License-Identifier: Apache-2.0

// ChipCoV8 #11, second half: a column with an empty caption — no attribute, no
// content widgets, no ShowContentAs — is still an ATTRIBUTE column (that is
// showContentAs' default), and `mx update-widgets` writes its `tooltip` as an
// empty Forms$ClientTemplate. Classified as itemKindDefault it got null, and
// `mx check --no-update-widgets` reported CE0463 on the grid.
package widgetobj

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

const dgColumnTooltipID = "00000000000000000000000000000013"

func dgColumnEntryWithTooltip() pages.PropertyTypeIDEntry {
	e := dgColumnEntry()
	e.NestedKeyOrder = append(e.NestedKeyOrder, "tooltip")
	e.NestedPropertyIDs["tooltip"] = pages.PropertyTypeIDEntry{PropertyTypeID: dgColumnTooltipID, ValueType: "TextTemplate"}
	return e
}

func tooltipTemplate(t *testing.T, spec backend.ObjectListItemSpec) any {
	t.Helper()
	got := buildObjectListItemBSON("com.mendix.widget.web.datagrid.Datagrid", "columns", dgColumnEntryWithTooltip(), spec)
	return templateOf(t, got, dgColumnTooltipID)
}

func TestDataGridColumn_UnboundAttributeColumnGetsEmptyTooltip(t *testing.T) {
	// No showContentAs in the spec: the widget default ("attribute") applies.
	if tt := tooltipTemplate(t, backend.ObjectListItemSpec{}); tt == nil {
		t.Error("tooltip of an attribute-less column with default showContentAs is null; update-widgets writes an empty ClientTemplate (CE0463)")
	}
	// Explicit ShowContentAs: attribute, still no attribute bound.
	explicit := backend.ObjectListItemSpec{Properties: []backend.ObjectListItemProperty{
		{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "attribute"},
	}}
	if tt := tooltipTemplate(t, explicit); tt == nil {
		t.Error("tooltip of an explicit showContentAs: attribute column with no attribute is null; want an empty ClientTemplate")
	}
	// Control: a custom-content column keeps tooltip null (Studio Pro's convention).
	custom := backend.ObjectListItemSpec{Properties: []backend.ObjectListItemProperty{
		{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "customContent"},
	}}
	if tt := tooltipTemplate(t, custom); tt != nil {
		t.Errorf("custom-content column tooltip = %v, want null", tt)
	}
}
