// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// docFixture builds the pieces of a project that carry documentation, runs the
// builders that fill the catalog tables plus buildStrings in full mode, and
// returns the catalog. One word appears only in documentation fields.
func docFixture(t *testing.T, word string) *Catalog {
	t.Helper()
	const modID = model.ID("mod-sales")

	entity := &domainmodel.Entity{
		BaseElement:   model.BaseElement{ID: "ent-order"},
		Name:          "Order",
		Documentation: note("Entity notes: ", word),
		Persistable:   true,
		Attributes: []*domainmodel.Attribute{{
			BaseElement:   model.BaseElement{ID: "attr-total"},
			Name:          "Total",
			Documentation: note("Attribute notes: ", word),
			Type:          &domainmodel.StringAttributeType{},
		}},
	}
	entity.ContainerID = "dm-sales"
	dm := &domainmodel.DomainModel{
		BaseElement:   model.BaseElement{ID: "dm-sales"},
		ContainerID:   modID,
		Documentation: note("Domain model notes: ", word),
		Entities:      []*domainmodel.Entity{entity},
	}
	constant := &model.Constant{
		BaseElement:   model.BaseElement{ID: "const-limit"},
		ContainerID:   modID,
		Name:          "Limit",
		Documentation: note("Constant notes: ", word),
		Type:          model.ConstantDataType{Kind: "String"},
	}
	page := &pages.Page{
		BaseElement:   model.BaseElement{ID: "page-home"},
		ContainerID:   modID,
		Name:          "Home",
		Documentation: note("Page notes: ", word),
	}
	// A microflow's documentation is already indexed by the typed extraction;
	// the catalog-table pass must not index it a second time.
	mf := &microflows.Microflow{
		BaseElement:   model.BaseElement{ID: "mf-create"},
		ContainerID:   modID,
		Name:          "CreateOrder",
		Documentation: note("Microflow notes: ", word),
	}

	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog:  cat,
		fullMode: true,
		reader: &mock.MockBackend{
			ListModulesFunc: func() ([]*model.Module, error) {
				return []*model.Module{{
					BaseElement:   model.BaseElement{ID: modID},
					Name:          "Sales",
					Documentation: note("Module notes: ", word),
				}}, nil
			},
			ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
			ListConstantsFunc:    func() ([]*model.Constant, error) { return []*model.Constant{constant}, nil },
			ListPagesFunc:        func() ([]*pages.Page, error) { return []*pages.Page{page}, nil },
			ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
			ListNanoflowsFunc:    func() ([]*microflows.Nanoflow, error) { return nil, nil },
			ListRulesFunc:        func() ([]*microflows.Rule, error) { return nil, nil },
		},
		snapshot: &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{modID: true},
			moduleNames:     map[model.ID]string{modID: "Sales"},
			containerParent: map[model.ID]model.ID{"dm-sales": modID},
			folderNames:     map[model.ID]string{},
		},
		tx: wrapTx(tx),
	}
	// In the order Build runs them: strings reads the tables the others fill.
	for _, step := range []struct {
		name  string
		build func() error
	}{
		{"modules", b.buildModules},
		{"entities", b.buildEntities},
		{"constants", b.buildConstants},
		{"microflows", b.buildMicroflows},
		{"pages", b.buildPages},
		{"strings", b.buildStrings},
	} {
		if err := step.build(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return cat
}

// A word that appears only in the documentation of an entity, an attribute, a
// constant or a page used to be invisible to SEARCH: those fields are plain
// strings in catalog tables, not Texts$Text, so neither the translatable walk
// nor the typed extractions reached them. Each hit must also say which element
// and which kind of field it came from, and which module it belongs to: a
// module's own rows name the module itself, not the empty ModuleName the
// objects view carries for a MODULE, so a filter on ModuleName (excluding
// Marketplace modules, say) can see them.
func TestBuildStrings_IndexesElementDocumentation(t *testing.T) {
	cat := docFixture(t, "zebracorn")

	res, err := cat.Query(`SELECT QualifiedName, ObjectType, StringContext, ElementId, ModuleName
		FROM strings WHERE strings MATCH 'zebracorn'`)
	if err != nil {
		t.Fatal(err)
	}
	type hit struct{ objectType, context, elementID, module string }
	// A module has two rows (its own documentation and its domain model's), so
	// the key is the element and the field.
	got := map[string]hit{}
	for _, row := range res.Rows {
		qn, _ := row[0].(string)
		ot, _ := row[1].(string)
		cx, _ := row[2].(string)
		id, _ := row[3].(string)
		mod, _ := row[4].(string)
		key := qn + " / " + cx
		if _, dup := got[key]; dup {
			t.Errorf("%s indexed more than once", key)
		}
		got[key] = hit{ot, cx, id, mod}
	}

	want := map[string]hit{
		"Sales.Order / documentation":        {"ENTITY", "documentation", "ent-order", "Sales"},
		"Sales.Order.Total / documentation":  {"ATTRIBUTE", "documentation", "attr-total", "Sales"},
		"Sales.Limit / documentation":        {"CONSTANT", "documentation", "const-limit", "Sales"},
		"Sales.Home / documentation":         {"PAGE", "documentation", "page-home", "Sales"},
		"Sales.CreateOrder / documentation":  {"MICROFLOW", "documentation", "mf-create", "Sales"},
		"Sales / documentation":              {"MODULE", "documentation", "mod-sales", "Sales"},
		"Sales / domain_model_documentation": {"MODULE", "domain_model_documentation", "mod-sales", "Sales"},
	}
	for key, w := range want {
		g, ok := got[key]
		if !ok {
			t.Errorf("%s: documentation never reached the strings index", key)
			continue
		}
		if g != w {
			t.Errorf("%s: got %+v, want %+v", key, g, w)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("unexpected hit %s: %+v", key, got[key])
		}
	}
}

// An element with no documentation must not produce an empty row: an empty
// StringValue is what the insert closure already refuses, and the objects view
// carries ” for every kind that has no documentation field.
func TestBuildStrings_UndocumentedElementsAddNoRows(t *testing.T) {
	cat := docFixture(t, "")
	res, err := cat.Query(`SELECT QualifiedName, StringValue FROM strings WHERE StringContext IN ('documentation', 'domain_model_documentation')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range res.Rows {
		v, _ := row[1].(string)
		if v == "" {
			t.Errorf("empty documentation row for %v", row[0])
		}
	}
}

// note returns prefix+word, or "" for an element that has no documentation.
func note(prefix, word string) string {
	if word == "" {
		return ""
	}
	return prefix + word
}

// A log message is a Microflows$StringTemplate whose Text is a plain string, not
// a Texts$Text, so the translatable walk never sees it: only the log node name
// reached the index, and a word that appears only in the message was invisible
// to SEARCH.
func TestExtractLogNodeNames_IndexesTheLogMessage(t *testing.T) {
	act := &microflows.ActionActivity{
		Action: &microflows.LogMessageAction{
			LogNodeName:     "'Orders'",
			MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "Order failed: zebracorn"}},
		},
	}
	act.ID = "act-log"
	oc := &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{act}}

	type row struct{ objType, value, context, elementID, module string }
	var got []row
	extractLogNodeNames(oc, "Sales.CreateOrder", "MICROFLOW", "Sales",
		func(qn, objType, value, ctx, lang, elementID, module string) {
			if qn != "Sales.CreateOrder" || lang != "" {
				t.Errorf("qn = %q, lang = %q", qn, lang)
			}
			got = append(got, row{objType, value, ctx, elementID, module})
		})

	want := []row{
		{"MICROFLOW", "'Orders'", "log_node", "act-log", "Sales"},
		{"MICROFLOW", "Order failed: zebracorn", "log_message", "act-log", "Sales"},
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
