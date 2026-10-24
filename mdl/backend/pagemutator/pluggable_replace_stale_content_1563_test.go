// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1563 (mendixlabs/mxcli#1631): `replace cbBooking with { combobox
// cbSale (… DataSource: database from M.Sale …) }` kept cbBooking's custom
// content — txtOptionName, bound to Booking.Name — inside cbSale. describe
// never prints custom content, so the baseline and the replacement agree on it
// and the #1247 merge keeps the stored widgets; with the content type 'yes'
// mx check reported CE1365 "Attribute 'M.Booking.Name' is not an attribute of
// entity 'M.Sale'" at a widget the author never wrote. On current main
// describe prints the type, so the replace turns it 'no' and mx check passes —
// with txtOptionName still stored, hidden, and bound to Booking.

// contentCombo is a combo box whose custom content (widget names) is built for
// its association data source on entity. linked is the content's
// DataSourceProperty, as the stored Type declares it.
func contentCombo(prefix, entity, contentType, emptyText, linked string, content ...string) bson.D {
	type prop struct {
		key string
		vt  bson.D
		val bson.D
	}
	widgets := bson.A{int32(2)}
	for _, n := range content {
		widgets = append(widgets, bson.D{
			{Key: "$ID", Value: bid(prefix + "cw-" + n)},
			{Key: "$Type", Value: "Forms$DynamicText"},
			{Key: "Name", Value: n},
		})
	}
	ds := bson.D{}
	if entity != "" {
		ds = bson.D{
			{Key: "$ID", Value: bid(prefix + "ds")},
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetXPathSource"},
			{Key: "EntityRef", Value: bson.D{
				{Key: "$ID", Value: bid(prefix + "er")},
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: entity},
			}},
		}
	}
	props := []prop{
		{"optionsSourceAssociationDataSource", bson.D{{Key: "Type", Value: "DataSource"}}, bson.D{{Key: "DataSource", Value: ds}}},
		{"optionsSourceAssociationCustomContentType", bson.D{{Key: "Type", Value: "Enumeration"}}, bson.D{{Key: "PrimitiveValue", Value: contentType}}},
		{"optionsSourceAssociationCustomContent",
			bson.D{{Key: "Type", Value: "Widgets"}, {Key: "DataSourceProperty", Value: linked}},
			bson.D{{Key: "PrimitiveValue", Value: ""}, {Key: "Widgets", Value: widgets}}},
		{"emptyOptionText", bson.D{{Key: "Type", Value: "TextTemplate"}}, bson.D{{Key: "PrimitiveValue", Value: emptyText}}},
	}
	var pts, ps bson.A
	pts = append(pts, int32(2))
	ps = append(ps, int32(2))
	for _, p := range props {
		vt := append(bson.D{{Key: "$ID", Value: bid(prefix + "vt-" + p.key)}}, p.vt...)
		pts = append(pts, bson.D{
			{Key: "$ID", Value: bid(prefix + "pt-" + p.key)},
			{Key: "PropertyKey", Value: p.key},
			{Key: "ValueType", Value: vt},
		})
		val := append(bson.D{{Key: "$ID", Value: bid(prefix + "v-" + p.key)}}, p.val...)
		val = append(val, bson.E{Key: "TypePointer", Value: bid(prefix + "vt-" + p.key)})
		ps = append(ps, bson.D{
			{Key: "$ID", Value: bid(prefix + "p-" + p.key)},
			{Key: "TypePointer", Value: bid(prefix + "pt-" + p.key)},
			{Key: "Value", Value: val},
		})
	}
	return bson.D{
		{Key: "$ID", Value: bid(prefix + "w")},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "cbBooking"},
		{Key: "Object", Value: bson.D{{Key: "$ID", Value: bid(prefix + "o")}, {Key: "Properties", Value: ps}}},
		{Key: "Type", Value: bson.D{
			{Key: "$ID", Value: bid(prefix + "t")},
			{Key: "ObjectType", Value: bson.D{{Key: "$ID", Value: bid(prefix + "ot")}, {Key: "PropertyTypes", Value: pts}}},
			{Key: "WidgetId", Value: "com.mendix.widget.web.combobox.Combobox"},
		}},
	}
}

const linkedDS = "optionsSourceAssociationDataSource"

func TestStaleLinkedContent(t *testing.T) {
	stored := contentCombo("S", "M.Booking", "yes", "", linkedDS, "txtOptionName")

	for _, tc := range []struct {
		name                  string
		baseline, replacement bson.D
		want                  bool
	}{
		{
			// The reported shape, as describe prints it on current main (the
			// type, not the widgets) and as it printed it then (neither).
			"another entity, type described",
			contentCombo("B", "M.Booking", "yes", "", linkedDS),
			contentCombo("N", "M.Sale", "no", "", linkedDS),
			true,
		},
		{
			"another entity, type not described",
			contentCombo("B", "M.Booking", "no", "", linkedDS),
			contentCombo("N", "M.Sale", "no", "", linkedDS),
			true,
		},
		// Controls: each still merges, and keeps the content — #1247's case.
		{
			"same entity, another property stated",
			contentCombo("B", "M.Booking", "yes", "", linkedDS),
			contentCombo("N", "M.Booking", "yes", "Pick one", linkedDS),
			false,
		},
		{
			"entity of the replacement unknown",
			contentCombo("B", "M.Booking", "yes", "", linkedDS),
			contentCombo("N", "", "yes", "", linkedDS),
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			merged, ok := mergeUnstatedPluggable(stored, tc.replacement, tc.baseline)
			if !ok {
				t.Fatal("merge did not apply")
			}
			stale := staleLinkedContent(stored, merged)
			if (stale != nil) != tc.want {
				t.Fatalf("staleLinkedContent = %v, want stale=%v", stale, tc.want)
			}
			if stale == nil {
				return
			}
			if stale.Property != "optionsSourceAssociationCustomContent" || stale.DataSource != linkedDS ||
				stale.StoredEntity != "M.Booking" || stale.NewEntity != "M.Sale" ||
				len(stale.Widgets) != 1 || stale.Widgets[0] != "txtOptionName" {
				t.Errorf("stale = %+v", stale)
			}
		})
	}

	// Controls on the stored side: nothing to carry, or content built for no
	// data source (a static source's custom content).
	for name, s := range map[string]bson.D{
		"stored content empty":        contentCombo("S", "M.Booking", "yes", "", linkedDS),
		"content linked to no source": contentCombo("S", "M.Booking", "yes", "", "", "txtOptionName"),
	} {
		t.Run(name, func(t *testing.T) {
			merged, ok := mergeUnstatedPluggable(s, contentCombo("N", "M.Sale", "no", "", linkedDS),
				contentCombo("B", "M.Booking", "yes", "", linkedDS))
			if !ok {
				t.Fatal("merge did not apply")
			}
			if stale := staleLinkedContent(s, merged); stale != nil {
				t.Errorf("reported %v, want nothing", stale)
			}
		})
	}
}

// fakeSerializer hands back the document built for each semantic widget.
type fakeSerializer struct {
	Deps
	docs map[pages.Widget]bson.D
}

func (f fakeSerializer) SerializeWidget(w pages.Widget) bson.D { return f.docs[w] }

// Through the mutator: the replace is refused, nothing is written, and the
// check-time question gives the same answer without writing either.
func TestReplacePluggableRefusesStaleContent(t *testing.T) {
	replacement := &pages.CustomWidget{BaseWidget: pages.BaseWidget{Name: "cbSale"}}
	baseline := &pages.CustomWidget{BaseWidget: pages.BaseWidget{Name: "cbBooking"}}
	newDoc := contentCombo("N", "M.Sale", "no", "", linkedDS)
	bsonnav.DSet(newDoc, "Name", "cbSale")
	deps := fakeSerializer{docs: map[pages.Widget]bson.D{
		replacement: newDoc,
		baseline:    contentCombo("B", "M.Booking", "yes", "", linkedDS),
	}}
	m := New(makeRawPage(contentCombo("S", "M.Booking", "yes", "", linkedDS, "txtOptionName")), model.ID("u"), deps)
	err := m.CheckPluggableReplace("cbBooking", replacement, baseline)
	var stale *backend.StaleWidgetContentError
	if !errors.As(err, &stale) {
		t.Fatalf("CheckPluggableReplace = %v, want a StaleWidgetContentError", err)
	}
	if stale.Widget != "cbBooking" || !strings.Contains(err.Error(), "drop txtOptionName;") {
		t.Errorf("refusal should name the widget and the drop that clears it: %v", err)
	}

	handled, err := m.ReplacePluggableKeepingUnstated("cbBooking", replacement, baseline)
	if !handled || !errors.As(err, &stale) {
		t.Fatalf("ReplacePluggableKeepingUnstated = (%v, %v), want (true, StaleWidgetContentError)", handled, err)
	}
	if findBsonWidget(m.rawData, "cbSale") != nil || findBsonWidget(m.rawData, "cbBooking") == nil {
		t.Error("the refused replace wrote the page")
	}

	// Control: the same replace once the content is dropped goes through.
	clean := New(makeRawPage(contentCombo("S", "M.Booking", "yes", "", linkedDS)), model.ID("u"), deps)
	if err := clean.CheckPluggableReplace("cbBooking", replacement, baseline); err != nil {
		t.Errorf("no content to carry, yet refused: %v", err)
	}
	if handled, err := clean.ReplacePluggableKeepingUnstated("cbBooking", replacement, baseline); !handled || err != nil {
		t.Errorf("ReplacePluggableKeepingUnstated = (%v, %v), want (true, nil)", handled, err)
	}
	if findBsonWidget(clean.rawData, "cbSale") == nil {
		t.Error("the replace with nothing to carry was not written")
	}
}
