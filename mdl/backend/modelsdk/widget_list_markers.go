// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "github.com/mendixlabs/mxcli/modelsdk/codec"

// widgetsListOwners are the $Types that hold a `Widgets` PartList in stored
// Mendix 11 documents. Each is the owner's storage name, as the encoder sees it
// (Forms$FormCallArgument, not gen's Forms$LayoutCallArgument).
var widgetsListOwners = []string{
	"CustomWidgets$WidgetValue",
	"Forms$BuildingBlock",
	"Forms$DataView",
	"Forms$DivContainer",
	"Forms$FormCallArgument",
	"Forms$GroupBox",
	"Forms$LayoutGridColumn",
	"Forms$ListView",
	"Forms$ListViewTemplate",
	"Forms$NativeLayoutContent",
	"Forms$NavigationListItem",
	"Forms$ScrollContainerRegion",
	"Forms$Snippet",
	"Forms$TabPage",
	"Forms$TemplateGridContents",
	"Forms$WebLayoutContent",
}

// Every `Widgets` list is typed-array marker 2, keyed on the property rather
// than on the first child. Measured across ako/TestApp and PedApp: 22,000+
// Widgets lists on every owner above but TemplateGridContents, empty and populated, with no
// exception — including 14 whose first child is a Forms$TabControl, whose own
// TabPages list is 3 (32 of 32) and stays on the child-type default.
//
// The child-type registrations (RegisterListMarker per widget type) gave the
// right answer only for a first child someone had registered: a column opening
// with a tab control was written as 3 (ako/mxcli-ledger FINDINGS #170). They
// also never reach two other paths, which the property registration does: an
// emptied list (the encoder's empty default is 3) and a selectively rebuilt one
// (a stored list with one dirty child, written with 3 unless the property is
// registered).
//
// Forms$TemplateGridContents has no reference document; it was measured with
// mx convert (Mendix 11.15.0), which re-serializes every unit. Injected with
// its Widgets written as [3] — empty, and with a TabControl first child — it
// came back [2] both times, and a bare grid gained an empty Widgets [2]. The
// same run's control, a DivContainer written as [3], also came back [2].
//
// The other gen Widgets owners cannot occur in a Mendix 11 project, so no
// marker is registered for them: convert refuses Forms$TableCell,
// Forms$VerticalFlow, NativePages$NativeLayoutCallArgument and
// NativePages$NativePage with "the type cache does not contain a type". A
// Mendix 11 table cell is Forms$DbTableCell (Widgets [2] on convert, empty or
// with a TabControl first), which gen does not model: the decoder keeps it as
// opaque raw bytes, so the encoder never writes its list. Forms$Layout and
// Forms$LayoutCall hold no Widgets list themselves — their widgets sit in
// WebLayoutContent / NativeLayoutContent and FormCallArgument, all above.
//
// The list is also mandatory: no owner above is ever stored without its Widgets
// key (TestApp + PedApp: TabPage 89/89 with 20 empty, LayoutGridColumn 732/732
// with 13 empty, DivContainer 838/838 with 8 empty, WidgetValue 20,775/20,775,
// every other owner likewise). A freshly built TabPage, DivContainer, GroupBox,
// LayoutGridColumn, ScrollContainerRegion or NavigationListItem with no children
// wrote no key at all; RegisterMandatoryPropertyList writes it as [2].
func init() {
	for _, owner := range widgetsListOwners {
		codec.RegisterMandatoryPropertyList(owner, "Widgets", 2)
	}
}
