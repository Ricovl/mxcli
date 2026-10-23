// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1583 (ako/mxcli#1488): a Combo box in database mode
//
//	combobox cbCode (source: 'database', optionsSourceDatabaseDataSource: microflow Mod.GET_Parents,
//	  optionsSourceDatabaseItemSelection: 'Single', optionsSourceDatabaseCaptionAttribute: Name,
//	  optionsSourceDatabaseValueAttribute: Name, databaseAttributeString: Code)
//
// "check reports 3 issues: 0 errors, 3 warnings" (two MDL-WIDGET06 "recognized
// but not yet persisted"), "exec reports Created page Mod.Child_Edit", and mx
// check reports "[CE0463] The definition of this widget has changed" at Combo
// box 'cbCode'.
//
// The definition had no database mode: the datasource had no write path, the
// item selection went through the explicit-key pass as a PrimitiveValue, and
// the caption and value attributes — linked to the unwritten datasource — bound
// against the enclosing data view's entity instead.

// comboBoxDatabaseEngine builds inside a data view over Mod.Child (Code, Name),
// with Mod.GET_Parents returning a list of Mod.Parent (Name) created earlier in
// the same script, and the REAL combo box template.
func comboBoxDatabaseEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	return comboBoxDatabaseEngineIn(t, "")
}

// comboBoxDatabaseEngineIn is comboBoxDatabaseEngine in a project, whose
// installed widget packages supply the visibility rules the build applies.
func comboBoxDatabaseEngineIn(t *testing.T, mprPath string) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-mod")}, Name: "Mod"}
	b := &mock.MockBackend{
		PathFunc:               func() string { return mprPath },
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-mod")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-parent")}, Name: "Parent",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-child")}, Name: "Child",
						Attributes: []*domainmodel.Attribute{{Name: "Code"}, {Name: "Name"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		entityContext:    "Mod.Child",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
		execCache: &executorCache{createdMicroflows: map[string]*createdMicroflowInfo{
			"Mod.GET_Parents": {ID: model.ID("mf-get-parents"), Name: "GET_Parents", ModuleName: "Mod",
				ReturnEntityName: "Mod.Parent"},
		}},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

func realComboBoxDef(t *testing.T) *WidgetDefinition {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	def, ok := reg.Get("COMBOBOX")
	if !ok {
		t.Fatal("no COMBOBOX definition")
	}
	return def
}

func buildComboBox(t *testing.T, src string) *pages.CustomWidget {
	t.Helper()
	widget, err := comboBoxDatabaseEngine(t).Build(realComboBoxDef(t), parseSnippetWidget(t, src))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return widget
}

func storedSelection(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	s, _ := bsonField(customWidgetValue(t, cw, key), "Selection").(string)
	return s
}

// storedMicroflowSource returns the microflow a datasource property is bound
// to, "" when it holds no microflow source.
func storedMicroflowSource(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	ds, _ := bsonField(customWidgetValue(t, cw, key), "DataSource").(bson.D)
	if typ, _ := bsonField(ds, "$Type").(string); typ != "Forms$MicroflowSource" {
		return ""
	}
	settings, _ := bsonField(ds, "MicroflowSettings").(bson.D)
	mf, _ := bsonField(settings, "Microflow").(string)
	return mf
}

const reportedDatabaseComboBox = `combobox cbCode (source: 'database', optionsSourceDatabaseDataSource: microflow Mod.GET_Parents, ` +
	`optionsSourceDatabaseItemSelection: 'Single', optionsSourceDatabaseCaptionAttribute: Name, ` +
	`optionsSourceDatabaseValueAttribute: Name, databaseAttributeString: Code)`

func assertDatabaseComboBox(t *testing.T, cw *pages.CustomWidget, selection string) {
	t.Helper()
	if got := storedPrimitive(t, cw, "source"); got != "database" {
		t.Errorf("source = %q, want database", got)
	}
	if got := storedMicroflowSource(t, cw, "optionsSourceDatabaseDataSource"); got != "Mod.GET_Parents" {
		t.Errorf("optionsSourceDatabaseDataSource microflow = %q, want Mod.GET_Parents — the datasource was dropped", got)
	}
	if got := storedSelection(t, cw, "optionsSourceDatabaseItemSelection"); got != selection {
		t.Errorf("optionsSourceDatabaseItemSelection Selection = %q, want %q", got, selection)
	}
	// A selection lives in the value's Selection field. Writing it as a
	// PrimitiveValue too is a value Studio Pro never stores there.
	if got := storedPrimitive(t, cw, "optionsSourceDatabaseItemSelection"); got != "" {
		t.Errorf("optionsSourceDatabaseItemSelection PrimitiveValue = %q, want empty", got)
	}
	// Linked to optionsSourceDatabaseDataSource (widget.xml dataSource=…):
	// attributes of the option entity, not of the data view's Mod.Child.
	for _, key := range []string{"optionsSourceDatabaseCaptionAttribute"} {
		if got := storedAttribute(t, cw, key); got != "Mod.Parent.Name" {
			t.Errorf("%s = %q, want Mod.Parent.Name", key, got)
		}
	}
	// Linked to no datasource: the attribute of the enclosing object.
	if got := storedAttribute(t, cw, "databaseAttributeString"); got != "Mod.Child.Code" {
		t.Errorf("databaseAttributeString = %q, want Mod.Child.Code", got)
	}
}

func TestComboBoxDatabase1488_ReportedScriptIsWritten(t *testing.T) {
	cw := buildComboBox(t, reportedDatabaseComboBox)
	assertDatabaseComboBox(t, cw, "Single")
	if got := storedAttribute(t, cw, "optionsSourceDatabaseValueAttribute"); got != "Mod.Parent.Name" {
		t.Errorf("optionsSourceDatabaseValueAttribute = %q, want Mod.Parent.Name", got)
	}
}

// In a project the build applies the installed package's visibility rules, and
// a database-mode combo box hides properties the template holds at another
// value. These two are what mx update-widgets changed on the widget mxcli wrote
// — the CE0463 once the datasource and selection were in place: the hidden
// `optionsSourceType` must hold its declared default, and the hidden
// `selectAllButtonCaption` text template must be null.
func TestComboBoxDatabase1488_HiddenPropertiesAreAtTheirDefaults(t *testing.T) {
	mpr, err := filepath.Abs("../../testdata/expr-checker/minimal.mpr")
	if err != nil {
		t.Fatal(err)
	}
	e := comboBoxDatabaseEngineIn(t, mpr)
	cw, err := e.Build(realComboBoxDef(t), parseSnippetWidget(t, reportedDatabaseComboBox))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	assertDatabaseComboBox(t, cw, "Single")
	if got := storedPrimitive(t, cw, "optionsSourceType"); got != "association" {
		t.Errorf("optionsSourceType = %q, want its declared default association — hidden in database mode", got)
	}
	if tt := bsonField(customWidgetValue(t, cw, "selectAllButtonCaption"), "TextTemplate"); tt != nil {
		t.Errorf("selectAllButtonCaption TextTemplate = %v, want null — hidden in database mode", tt)
	}
}

// The keyword spelling the association mode already has — `Attribute:` for the
// attribute the widget stores into, `CaptionAttribute:` for the option caption —
// selected by the datasource key alone, with no `source:`.
func TestComboBoxDatabase1488_KeywordSpelling(t *testing.T) {
	cw := buildComboBox(t, `combobox cbCode (optionsSourceDatabaseDataSource: microflow Mod.GET_Parents, `+
		`Attribute: Code, CaptionAttribute: Name)`)
	assertDatabaseComboBox(t, cw, "Single")
	// The enumeration and association slots stay empty: the bare `Attribute:`
	// belongs to the database mode's target attribute here.
	for _, key := range []string{"attributeEnumeration", "attributeAssociation"} {
		if got := storedAttribute(t, cw, key); got != "" {
			t.Errorf("%s = %q, want empty in database mode", key, got)
		}
	}
}

func TestComboBoxDatabase1488_MultiSelection(t *testing.T) {
	cw := buildComboBox(t, `combobox cbCode (optionsSourceDatabaseDataSource: microflow Mod.GET_Parents, `+
		`optionsSourceDatabaseItemSelection: Multi, CaptionAttribute: Name)`)
	if got := storedSelection(t, cw, "optionsSourceDatabaseItemSelection"); got != "Multi" {
		t.Errorf("optionsSourceDatabaseItemSelection Selection = %q, want Multi", got)
	}
	if got := storedPrimitive(t, cw, "optionsSourceDatabaseItemSelection"); got != "" {
		t.Errorf("optionsSourceDatabaseItemSelection PrimitiveValue = %q, want empty", got)
	}
}

// The control: an association-mode combo box is untouched — no source switch,
// its own datasource, the database slots empty.
func TestComboBoxDatabase1488_AssociationModeUnchanged(t *testing.T) {
	cw := buildComboBox(t, `combobox cbCode (DataSource: microflow Mod.GET_Parents)`)
	if got := storedPrimitive(t, cw, "source"); got != "context" {
		t.Errorf("source = %q, want context for an association-mode combo box", got)
	}
	if got := storedMicroflowSource(t, cw, "optionsSourceDatabaseDataSource"); got != "" {
		t.Errorf("optionsSourceDatabaseDataSource = %q, want empty in association mode", got)
	}
	if got := storedMicroflowSource(t, cw, "optionsSourceAssociationDataSource"); got != "Mod.GET_Parents" {
		t.Errorf("optionsSourceAssociationDataSource = %q, want Mod.GET_Parents", got)
	}
}

// storedDatabaseComboBox is a database-mode combo box as it is stored: the
// shipped template with the values exec (and Studio Pro) write for the reported
// configuration — the hidden association-mode `optionsSourceType` back at its
// declared default.
func storedDatabaseComboBox(t *testing.T, selection string) map[string]any {
	t.Helper()
	w := widgetFromTemplate(t, comboBoxWidgetID, "cbCode")
	gridValue(t, w, "source")["PrimitiveValue"] = "database"
	gridValue(t, w, "optionsSourceType")["PrimitiveValue"] = "association"
	gridValue(t, w, "optionsSourceDatabaseDataSource")["DataSource"] =
		dsBSON(dsTypeMicroflow, "MicroflowSettings", map[string]any{"Microflow": "Mod.GET_Parents"})
	gridValue(t, w, "optionsSourceDatabaseItemSelection")["Selection"] = selection
	attr := func(key, qn string) {
		gridValue(t, w, key)["AttributeRef"] = map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": qn}
	}
	attr("optionsSourceDatabaseCaptionAttribute", "Mod.Parent.Name")
	if selection != "Multi" { // Multi hides both
		attr("optionsSourceDatabaseValueAttribute", "Mod.Parent.Name")
		attr("databaseAttributeString", "Mod.Child.Code")
	}
	return w
}

// DESCRIBE spelled only the association and enumeration modes. A database-mode
// combo box came back as `combobox cbCode (DataSource: microflow …)` — the
// generic clause, which selects the ASSOCIATION mode on exec — with its
// target, caption and value attributes named as not reproduced.
func TestDescribeComboBox1488_DatabaseModeRoundTrips(t *testing.T) {
	got := describeParsed(t, storedDatabaseComboBox(t, "Single"))
	for _, want := range []string{
		"optionsSourceDatabaseDataSource: microflow Mod.GET_Parents",
		"Attribute: Code",
		"CaptionAttribute: Name",
		// Bare where the microflow's return entity resolves (a project), and
		// qualified where it cannot (here) — exec reads both.
		"optionsSourceDatabaseValueAttribute: ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"(DataSource:", " DataSource:", "NOT re-executable", "source:", "optionsSourceType"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("describe prints %q:\n%s", unwanted, got)
		}
	}

	// And the description builds the same widget again.
	stmt := got[strings.Index(got, "combobox"):]
	rebuilt := buildComboBox(t, stmt)
	assertDatabaseComboBox(t, rebuilt, "Single")
	if v := storedAttribute(t, rebuilt, "optionsSourceDatabaseValueAttribute"); v != "Mod.Parent.Name" {
		t.Errorf("re-executed optionsSourceDatabaseValueAttribute = %q, want Mod.Parent.Name", v)
	}
}

func TestDescribeComboBox1488_MultiSelectionRoundTrips(t *testing.T) {
	got := describeParsed(t, storedDatabaseComboBox(t, "Multi"))
	if !strings.Contains(got, "optionsSourceDatabaseItemSelection: Multi") {
		t.Fatalf("describe lacks the Multi selection:\n%s", got)
	}
	stmt := got[strings.Index(got, "combobox"):]
	if sel := storedSelection(t, buildComboBox(t, stmt), "optionsSourceDatabaseItemSelection"); sel != "Multi" {
		t.Errorf("re-executed selection = %q, want Multi", sel)
	}
}

// A database-mode combo box saved before its option list was chosen — a state
// Studio Pro allows — describes without one rather than failing.
func TestDescribeComboBox1488_DatabaseModeWithoutDataSource(t *testing.T) {
	w := storedDatabaseComboBox(t, "Single")
	delete(gridValue(t, w, "optionsSourceDatabaseDataSource"), "DataSource")
	got := describeParsed(t, w)
	if !strings.Contains(got, "combobox cbCode") || strings.Contains(got, "optionsSourceDatabaseDataSource:") {
		t.Errorf("describe of a database-mode combo box with no option list:\n%s", got)
	}
}
