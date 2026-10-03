// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A page title is a Texts$Text with one translation per language. The builder
// took the first entry of that map, and Go's map order is random, so
// pages.Title — and every lint rule reading page.title — changed between two
// catalog builds of an unchanged project (mendixlabs/mxcli#1262). The title is
// the project's default language, with pickTranslation's fallbacks.
func TestPageTitleIsDefaultLanguage_Issue1262(t *testing.T) {
	const modID = model.ID("mod-t")

	// Many languages so that a map-order pick fails essentially every build,
	// not one time in two.
	multi := map[string]string{"nl_NL": "Klant historie"}
	for i := 0; i < 20; i++ {
		multi[fmt.Sprintf("x%02d_XX", i)] = fmt.Sprintf("other %d", i)
	}
	multi["en_US"] = "Customer history"

	newPage := func(id, name string, title map[string]string) *pages.Page {
		pg := &pages.Page{Name: name}
		pg.ID = model.ID(id)
		pg.ContainerID = modID
		if title != nil {
			pg.Title = &model.Text{Translations: title}
		}
		return pg
	}
	pageList := []*pages.Page{
		newPage("p-multi", "Multi", multi),
		newPage("p-empty-default", "EmptyDefault", map[string]string{"nl_NL": "", "en_US": "Fallback", "de_DE": "Rückfall"}),
		newPage("p-none", "NoTitle", nil),
		newPage("p-blank", "Blank", map[string]string{}),
	}
	want := map[string]string{
		"T.Multi":        "Klant historie",
		"T.EmptyDefault": "Fallback",
		"T.NoTitle":      "",
		"T.Blank":        "",
	}

	for build := 0; build < 30; build++ {
		cat, err := New()
		if err != nil {
			t.Fatal(err)
		}
		tx, err := cat.CatalogDB().Begin()
		if err != nil {
			t.Fatal(err)
		}
		b := &Builder{
			catalog: cat,
			reader: &mock.MockBackend{
				ListPagesFunc: func() ([]*pages.Page, error) { return pageList, nil },
				GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
					return &model.ProjectSettings{Language: &model.LanguageSettings{DefaultLanguageCode: "nl_NL"}}, nil
				},
			},
			snapshot: &Snapshot{ID: "snap"},
			hierarchy: &hierarchy{
				moduleIDs:       map[model.ID]bool{modID: true},
				moduleNames:     map[model.ID]string{modID: "T"},
				containerParent: map[model.ID]model.ID{},
				folderNames:     map[model.ID]string{},
			},
			tx: tx,
		}
		if err := b.buildPages(); err != nil {
			t.Fatalf("buildPages: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		got := queryStrings(t, cat, `SELECT QualifiedName, Title FROM pages_data`)
		cat.Close()
		for qn, w := range want {
			if got[qn] != w {
				t.Fatalf("build %d: %s title = %q, want %q (the project's default language, "+
					"else en_US, else the lowest-sorted non-empty one)", build, qn, got[qn], w)
			}
		}
	}
}
