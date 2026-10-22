// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// The backend reads a menu item's caption in model.AuthoringLanguage (the
// page rule, model.SelectTranslation), so every executor path that reads or
// writes navigation must resolve the project's default language BEFORE it
// reads. Otherwise a fresh process reads in en_US whatever the project's
// language is, and describe disagrees with describe page about the same texts.
func TestNavigationReadsResolveTheDefaultLanguageFirst(t *testing.T) {
	type call struct {
		name string
		run  func(ctx *ExecContext) error
	}
	calls := []call{
		{"describe navigation", func(ctx *ExecContext) error {
			return describeNavigation(ctx, ast.QualifiedName{Name: "Responsive"})
		}},
		{"list navigation menu", func(ctx *ExecContext) error { return listNavigationMenu(ctx, nil) }},
		{"describe menu", func(ctx *ExecContext) error {
			return describeMenu(ctx, ast.QualifiedName{Module: "M", Name: "Menu"})
		}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			model.SetAuthoringLanguage("")
			t.Cleanup(func() { model.SetAuthoringLanguage("") })
			var seen string
			mb := &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
					return &model.ProjectSettings{Language: &model.LanguageSettings{DefaultLanguageCode: "nl_NL"}}, nil
				},
				GetNavigationFunc: func() (*types.NavigationDocument, error) {
					seen = model.AuthoringLanguage()
					return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{Name: "Responsive", Kind: "Responsive"}}}, nil
				},
				GetMenuDocumentByQualifiedNameFunc: func(string, string) (*types.MenuDocument, error) {
					seen = model.AuthoringLanguage()
					return &types.MenuDocument{Name: "Menu"}, nil
				},
			}
			ctx := &ExecContext{Backend: mb, Cache: &executorCache{}, Output: &bytes.Buffer{}}
			if err := c.run(ctx); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if seen != "nl_NL" {
				t.Errorf("%s read navigation in %q, want the project's default nl_NL", c.name, seen)
			}
		})
	}
}
