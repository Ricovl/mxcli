// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A data view's footer is a REGION, not a widget (ako/mxcli#528,
// mendixlabs/mxcli#293). Studio Pro keeps its widgets in the data view's own
// FooterWidgets list and stores no name for it, so `footer footerButtons { … }`
// had nowhere to keep `footerButtons`, and describe's `footer1` was invented.
// Neither resolved, and a footer could not be edited at all.
//
// It is addressed the way a scroll container's region is — positionally, by the
// owner and the slot: `dvMain.footer`. INSERT INTO appends to it, REPLACE swaps
// its whole content, DROP empties it; BEFORE/AFTER a region mean nothing and are
// refused, as they are for a scroll-container region.

const dataViewFooterSlot = "footer"

// isDataViewFooterRef reports whether a dotted member names the footer slot.
func isDataViewFooterRef(member string) bool {
	return strings.EqualFold(member, dataViewFooterSlot)
}

// findDataViewFooter resolves `<owner>.footer`. ok is false when the owner is
// not a data view, so the caller falls through to the other dotted forms (a
// DataGrid 2 column called "footer" stays addressable).
func (m *Mutator) findDataViewFooter(owner, member string) (dv *bsonWidgetResult, ok bool) {
	if !isDataViewFooterRef(member) {
		return nil, false
	}
	result := m.widgetFinder(m.rawData, owner)
	if result == nil {
		return nil, false
	}
	if t := bsonnav.DGetString(result.widget, "$Type"); t != "Forms$DataView" && t != "Pages$DataView" {
		return nil, false
	}
	return result, true
}

// setDataViewFooter stores the footer's new content, keeping the list marker
// (2, as CREATE writes it — widget_write.go MandatoryListMarkers), and writes
// the data view back into its parent slot: bson.D is a slice, and a grown field
// list that is not written back is a silent no-op reported as success.
func setDataViewFooter(dv *bsonWidgetResult, widgets []any) {
	out := bson.A{int32(2)}
	if raw := bsonnav.ToBsonA(bsonnav.DGet(dv.widget, "FooterWidgets")); len(raw) > 0 && isListMarker(raw[0]) {
		out = bson.A{raw[0]}
	}
	out = append(out, widgets...)
	doc := dv.widget
	if !bsonnav.DSet(doc, "FooterWidgets", out) {
		doc = append(doc, bson.E{Key: "FooterWidgets", Value: out})
	}
	dv.widget = doc
	dv.parentArr[dv.index] = doc
	bsonnav.DSetArray(dv.parentDoc, dv.parentKey, dv.parentArr)
}

// dataViewFooterWidgets is the footer's current content, without the marker.
func dataViewFooterWidgets(dv bson.D) []any {
	return bsonnav.DGetArrayElements(bsonnav.DGet(dv, "FooterWidgets"))
}

// insertIntoDataViewFooter handles `insert into <dataview>.footer { … }`.
func (m *Mutator) insertIntoDataViewFooter(owner, member string, position backend.InsertPosition, widgets []pages.Widget) (bool, error) {
	dv, ok := m.findDataViewFooter(owner, member)
	if !ok {
		return false, nil
	}
	if !strings.EqualFold(string(position), "into") {
		return true, fmt.Errorf("a data view footer can only be an INSERT INTO target; "+
			"to place a widget relative to another, name that widget: `insert %s <widgetName> { … }`",
			strings.ToLower(string(position)))
	}
	newBson, err := m.serializeWidgets(widgets)
	if err != nil {
		return true, fmt.Errorf("serialize widgets: %w", err)
	}
	existing := dataViewFooterWidgets(dv.widget)
	if len(existing) == 0 && len(newBson) > 0 {
		// Filling an empty footer turns it on, as a `footer { … }` block does in
		// CREATE; otherwise the widgets are stored where nothing renders them.
		bsonnav.DSet(dv.widget, "ShowFooter", true)
	}
	setDataViewFooter(dv, append(append([]any{}, existing...), newBson...))
	return true, nil
}

// replaceDataViewFooter handles `replace <dataview>.footer with { … }`: the
// footer's whole content becomes the new widgets.
func (m *Mutator) replaceDataViewFooter(owner, member string, widgets []pages.Widget) (bool, error) {
	dv, ok := m.findDataViewFooter(owner, member)
	if !ok {
		return false, nil
	}
	newBson, err := m.serializeWidgets(widgets)
	if err != nil {
		return true, fmt.Errorf("serialize widgets: %w", err)
	}
	if len(dataViewFooterWidgets(dv.widget)) == 0 && len(newBson) > 0 {
		bsonnav.DSet(dv.widget, "ShowFooter", true)
	}
	setDataViewFooter(dv, newBson)
	return true, nil
}

// dropDataViewFooter handles `drop <dataview>.footer`: the footer is emptied.
// The data view's ShowFooter is its own property and is left as stored.
func (m *Mutator) dropDataViewFooter(owner, member string) bool {
	dv, ok := m.findDataViewFooter(owner, member)
	if !ok {
		return false
	}
	setDataViewFooter(dv, nil)
	return true
}

// dataViewFooterHint names the footer addresses a page has, for a not-found
// message: the name a script wrote on a data view footer, and describe's old
// `footer1`, were never stored, so a miss on one should say what does resolve.
func (m *Mutator) dataViewFooterHint() string {
	var owners []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			if t := bsonnav.DGetString(x, "$Type"); (t == "Forms$DataView" || t == "Pages$DataView") &&
				len(dataViewFooterWidgets(x)) > 0 {
				if n := bsonnav.DGetString(x, "Name"); n != "" {
					owners = append(owners, n+"."+dataViewFooterSlot)
				}
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(m.rawData)
	if len(owners) == 0 {
		return ""
	}
	return ". A data view footer stores no name — address it by its data view: " + strings.Join(owners, ", ")
}

// ContainedWidgetNames lists the names stored INSIDE what a reference addresses
// — a widget's descendants, or a data view footer's widgets and theirs. A
// REPLACE removes them with its target, so the replacement may reuse them; the
// duplicate-name scope used to count them as taken (mendixlabs/mxcli#293).
func (m *Mutator) ContainedWidgetNames(widgetRef, columnRef string) []string {
	var roots []any
	switch {
	case columnRef != "":
		dv, ok := m.findDataViewFooter(widgetRef, columnRef)
		if !ok {
			return nil
		}
		roots = dataViewFooterWidgets(dv.widget)
	default:
		result := m.widgetFinder(m.rawData, widgetRef)
		if result == nil {
			return nil
		}
		// The widget's own fields, not the widget: its own name is the target.
		for _, e := range result.widget {
			roots = append(roots, e.Value)
		}
	}
	var names []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			if strings.HasPrefix(bsonnav.DGetString(x, "$Type"), "Forms$") || strings.HasPrefix(bsonnav.DGetString(x, "$Type"), "CustomWidgets$CustomWidget") {
				if n := bsonnav.DGetString(x, "Name"); n != "" {
					names = append(names, n)
				}
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return names
}
