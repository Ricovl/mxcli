// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#1647 (and the write half of #1488): a combo box whose options come
// from a database source — TestApp's WorkflowCommons.User_WorkflowGroup_Select
// comboBox1: source 'database', optionsSourceDatabaseDataSource over
// System.WorkflowGroup, optionsSourceDatabaseItemSelection 'Multi',
// optionsSourceDatabaseCaptionAttribute Name — had no mode in the definition.
// Its data source and item selection were "recognized but not yet persisted"
// (MDL-WIDGET06), describe printed the source as the association mode's
// `DataSource:`, and exec rebuilt it as a context-source, single-selection combo
// box: CE0642 "Property 'Caption'/'Entity' is required", and CE0115 on the
// button passing its selection to a list parameter.

func comboBoxDatabaseEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-dba")}, Name: "DbA"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-dba")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-holder")}, Name: "Holder",
						Attributes: []*domainmodel.Attribute{{Name: "Note"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-grp")}, Name: "Grp",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}, {Name: "Code"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		entityContext:    "DbA.Holder",
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
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

func storedDataSourceEntity(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	ds, _ := bsonField(customWidgetValue(t, cw, key), "DataSource").(bson.D)
	ref, _ := bsonField(ds, "EntityRef").(bson.D)
	s, _ := bsonField(ref, "Entity").(string)
	return s
}

func storedSelection(t *testing.T, cw *pages.CustomWidget, key string) string {
	t.Helper()
	s, _ := bsonField(customWidgetValue(t, cw, key), "Selection").(string)
	return s
}

// The shape describe now prints for TestApp's comboBox1, written as Studio Pro
// stores it.
func TestComboBoxDatabaseModeIsWritten(t *testing.T) {
	cw := buildComboBox(t, `combobox cbGroups (
  optionsSourceDatabaseDataSource: database from DbA.Grp sort by Name asc,
  optionsSourceDatabaseItemSelection: 'Multi',
  optionsSourceDatabaseCaptionAttribute: Name
)`)
	if got := storedPrimitive(t, cw, "source"); got != "database" {
		t.Errorf("source = %q, want database", got)
	}
	if got := storedDataSourceEntity(t, cw, "optionsSourceDatabaseDataSource"); got != "DbA.Grp" {
		t.Errorf("optionsSourceDatabaseDataSource entity = %q, want DbA.Grp", got)
	}
	if got := storedSelection(t, cw, "optionsSourceDatabaseItemSelection"); got != "Multi" {
		t.Errorf("optionsSourceDatabaseItemSelection = %q, want Multi", got)
	}
	if got := storedAttribute(t, cw, "optionsSourceDatabaseCaptionAttribute"); got != "DbA.Grp.Name" {
		t.Errorf("optionsSourceDatabaseCaptionAttribute = %q, want DbA.Grp.Name (the database source's entity)", got)
	}
	// Not also an association-mode combo box.
	if got := storedDataSourceEntity(t, cw, "optionsSourceAssociationDataSource"); got != "" {
		t.Errorf("optionsSourceAssociationDataSource entity = %q, want none", got)
	}
}

// The selection keyword other list widgets take means the same here, and the
// widget's own default (Single) holds when neither is written.
func TestComboBoxDatabaseItemSelection(t *testing.T) {
	cw := buildComboBox(t, `combobox cbGroups (optionsSourceDatabaseDataSource: database from DbA.Grp, Selection: Multi, optionsSourceDatabaseCaptionAttribute: Name)`)
	if got := storedSelection(t, cw, "optionsSourceDatabaseItemSelection"); got != "Multi" {
		t.Errorf("Selection: Multi wrote %q, want Multi", got)
	}
	cw = buildComboBox(t, `combobox cbGroups (optionsSourceDatabaseDataSource: database from DbA.Grp, optionsSourceDatabaseCaptionAttribute: Name)`)
	if got := storedSelection(t, cw, "optionsSourceDatabaseItemSelection"); got != "Single" {
		t.Errorf("no selection wrote %q, want the default Single", got)
	}
}

// Control: `DataSource:` still selects the association mode, unchanged.
func TestComboBoxDataSourceClauseKeepsAssociationMode(t *testing.T) {
	cw := buildComboBox(t, `combobox cbGroups (DataSource: database from DbA.Grp, CaptionAttribute: Name)`)
	if got := storedPrimitive(t, cw, "source"); got == "database" {
		t.Errorf("source = database for a `DataSource:` combo box; the association mode must not change")
	}
	if got := storedPrimitive(t, cw, "optionsSourceType"); got != "association" {
		t.Errorf("optionsSourceType = %q, want association", got)
	}
	if got := storedDataSourceEntity(t, cw, "optionsSourceAssociationDataSource"); got != "DbA.Grp" {
		t.Errorf("optionsSourceAssociationDataSource entity = %q, want DbA.Grp", got)
	}
	if got := storedDataSourceEntity(t, cw, "optionsSourceDatabaseDataSource"); strings.TrimSpace(got) != "" {
		t.Errorf("optionsSourceDatabaseDataSource entity = %q, want none", got)
	}
}
