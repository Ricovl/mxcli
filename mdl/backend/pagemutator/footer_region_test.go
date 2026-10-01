// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A data view's footer is a region, not a widget: Studio Pro stores its
// widgets in the data view's FooterWidgets list and the footer itself has no
// Name. ako/mxcli#528 / mendixlabs/mxcli#293: neither the name the script
// wrote nor describe's invented `footer1` resolved, so a footer could not be
// edited at all. It is addressed as `<dataview>.footer`.
func footerPage(footer ...bson.D) bson.D {
	fw := bson.A{int32(2)}
	for _, w := range footer {
		fw = append(fw, w)
	}
	return makeRawPage(
		bson.D{
			{Key: "$Type", Value: "Forms$DataView"},
			{Key: "Name", Value: "dvMain"},
			{Key: "ShowFooter", Value: len(footer) > 0},
			{Key: "Widgets", Value: bson.A{int32(2), bson.D{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "Name", Value: "txtReason"}}}},
			{Key: "FooterWidgets", Value: fw},
		},
		bson.D{{Key: "$Type", Value: "Forms$DivContainer"}, {Key: "Name", Value: "ctn"}},
	)
}

func button(name string) bson.D {
	return bson.D{{Key: "$Type", Value: "Forms$ActionButton"}, {Key: "Name", Value: name}}
}

func newFooterMutator(raw bson.D) *Mutator {
	return New(raw, model.ID("page-1"), &stubWidgetDeps{})
}

func footerNames(t *testing.T, m *Mutator) []string {
	t.Helper()
	dv := findBsonWidget(m.rawData, "dvMain")
	if dv == nil {
		t.Fatal("dvMain not found")
	}
	var names []string
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(dv.widget, "FooterWidgets")) {
		if d, ok := el.(bson.D); ok {
			names = append(names, bsonnav.DGetString(d, "Name"))
		}
	}
	return names
}

func textBoxes(names ...string) []pages.Widget {
	var ws []pages.Widget
	for _, n := range names {
		ws = append(ws, &pages.TextBox{BaseWidget: pages.BaseWidget{Name: n}})
	}
	return ws
}

func TestDataViewFooter_Resolves(t *testing.T) {
	m := newFooterMutator(footerPage(button("btnSave")))
	got, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"dvMain", "footer"}})
	if err != nil {
		t.Fatalf("dvMain.footer did not resolve: %v", err)
	}
	if got.Kind != "region" {
		t.Errorf("kind = %q, want region", got.Kind)
	}
	// Control: a member a data view does not have is still refused.
	if _, err := m.ResolveAlterTarget(backend.AlterTarget{Path: []string{"dvMain", "header"}}); err == nil {
		t.Error("dvMain.header resolved; a data view has no header region")
	}
	if !m.ResolvesTarget("dvMain", "footer") {
		t.Error("ResolvesTarget(dvMain, footer) = false; check would refuse what exec does")
	}
}

func TestDataViewFooter_InsertInto(t *testing.T) {
	m := newFooterMutator(footerPage(button("btnSave")))
	if err := m.InsertWidget("dvMain", "footer", backend.InsertPosition("into"), textBoxes("added")); err != nil {
		t.Fatalf("insert into dvMain.footer: %v", err)
	}
	if got := strings.Join(footerNames(t, m), ","); got != "btnSave,added" {
		t.Errorf("FooterWidgets = %s, want btnSave,added", got)
	}
	// BEFORE/AFTER a region means nothing: refuse rather than guess.
	if err := m.InsertWidget("dvMain", "footer", backend.InsertPosition("after"), textBoxes("x")); err == nil {
		t.Error("insert after dvMain.footer succeeded; only INTO addresses a region")
	}
}

// Inserting into an EMPTY footer turns it on, as a `footer { … }` block does in
// CREATE — otherwise the widgets are stored where nothing renders them.
func TestDataViewFooter_InsertIntoEmptyShowsFooter(t *testing.T) {
	m := newFooterMutator(footerPage())
	if err := m.InsertWidget("dvMain", "footer", backend.InsertPosition("into"), textBoxes("added")); err != nil {
		t.Fatalf("insert into empty dvMain.footer: %v", err)
	}
	dv := findBsonWidget(m.rawData, "dvMain").widget
	if bsonnav.DGet(dv, "ShowFooter") != true {
		t.Errorf("ShowFooter = %v after filling an empty footer, want true", bsonnav.DGet(dv, "ShowFooter"))
	}
	if got := bsonnav.ToBsonA(bsonnav.DGet(dv, "FooterWidgets")); len(got) == 0 || got[0] != int32(2) {
		t.Errorf("FooterWidgets lost its list marker: %v", got)
	}
}

func TestDataViewFooter_Replace(t *testing.T) {
	m := newFooterMutator(footerPage(button("btnSave"), button("btnCancel")))
	if err := m.ReplaceWidget("dvMain", "footer", textBoxes("btnS2")); err != nil {
		t.Fatalf("replace dvMain.footer: %v", err)
	}
	if got := strings.Join(footerNames(t, m), ","); got != "btnS2" {
		t.Errorf("FooterWidgets = %s, want btnS2 (the whole footer replaced)", got)
	}
	// The data view's body is untouched.
	if findBsonWidget(m.rawData, "txtReason") == nil {
		t.Error("replacing the footer removed the data view's body")
	}
}

func TestDataViewFooter_Drop(t *testing.T) {
	m := newFooterMutator(footerPage(button("btnSave")))
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "dvMain", Column: "footer"}}); err != nil {
		t.Fatalf("drop dvMain.footer: %v", err)
	}
	if got := footerNames(t, m); len(got) != 0 {
		t.Errorf("FooterWidgets = %v after drop, want empty", got)
	}
	dv := findBsonWidget(m.rawData, "dvMain").widget
	if got := bsonnav.ToBsonA(bsonnav.DGet(dv, "FooterWidgets")); len(got) != 1 || got[0] != int32(2) {
		t.Errorf("FooterWidgets = %v after drop, want the bare list marker", got)
	}
	if findBsonWidget(m.rawData, "dvMain") == nil {
		t.Error("dropping the footer dropped the data view")
	}
}

// A miss on a name that is not stored names the address that works, when the
// page has a data view footer to point at.
func TestDataViewFooter_NotFoundNamesTheRegion(t *testing.T) {
	m := newFooterMutator(footerPage(button("btnSave")))
	err := m.ReplaceWidget("footer1", "", textBoxes("x"))
	if err == nil || !strings.Contains(err.Error(), "dvMain.footer") {
		t.Errorf("replace footer1: error %v should name dvMain.footer", err)
	}
}
