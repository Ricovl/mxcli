// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/generated/metamodel"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The enum literals documented for the builtins added in mendixlabs/mxcli#1265
// and #1269, pinned to the values the reader actually stores -- the same guard
// as lint_rule_doc_vocabulary_test.go, scoped to one "### struct" section,
// because these field names (type, owner, http_method, …) recur in other
// sections with other vocabularies. rest_operation's http_method is "GET"; a
// published operation's is "Get".

// sectionRowValues returns the quoted values in the row for field under the
// "### section" heading. It fails rather than returning nothing, which would
// make every assertion vacuous.
func sectionRowValues(t *testing.T, doc, section, field string) []string {
	t.Helper()
	start := strings.Index(doc, "\n### "+section+"\n")
	if start < 0 {
		t.Fatalf("no ### %s section in the write-lint-rules skill", section)
	}
	body := doc[start+1:]
	if end := strings.Index(body[4:], "\n#"); end >= 0 {
		body = body[:end+4]
	}
	row := regexp.MustCompile(`(?m)^\| ` + "`" + regexp.QuoteMeta(field) + "`" + ` \|.*$`).FindString(body)
	if row == "" {
		t.Fatalf("### %s has no row for %q", section, field)
	}
	var out []string
	for _, m := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(row, -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("### %s row %q documents no values: %s", section, field, row)
	}
	return out
}

func stringSet[T ~string](vals ...T) map[string]bool {
	m := map[string]bool{}
	for _, v := range vals {
		m[string(v)] = true
	}
	return m
}

// navigationActionTypes is every literal the navigation reader assigns to a
// menu item's ActionType, read from its source so a new one cannot be missed.
func navigationActionTypes(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "mdl", "backend", "modelsdk", "navigation_read.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`item\.ActionType = "([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	// CONTROL: the scan must find the known ones, or it is reading the wrong thing.
	if !out["PageAction"] || !out["MicroflowAction"] || len(out) < 5 {
		t.Fatalf("scan of %s found %v -- broken scan, not broken docs", path, out)
	}
	// Any other client action is reported by its stored type; the docs give
	// one measured on TestApp.
	out["Forms$CallNanoflowClientAction"] = true
	return out
}

func TestSkillDocumentsRealCatalogTableVocabulary(t *testing.T) {
	doc := lintRuleSkillDoc(t)
	deleteBehaviours := stringSet(
		domainmodel.DeleteBehaviorTypeDeleteMeAndReferences,
		domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
		domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences)

	for _, tc := range []struct {
		section, field string
		real           map[string]bool
	}{
		{"association", "type", stringSet(domainmodel.AssociationTypeReference, domainmodel.AssociationTypeReferenceSet)},
		{"association", "owner", stringSet(domainmodel.AssociationOwnerDefault, domainmodel.AssociationOwnerBoth)},
		{"association", "storage_format", stringSet(domainmodel.StorageFormatColumn, domainmodel.StorageFormatTable)},
		{"association", "to_delete_behavior", deleteBehaviours},
		{"association", "from_delete_behavior", deleteBehaviours},
		{"entity_event_handler", "moment", stringSet(domainmodel.EventMomentBefore, domainmodel.EventMomentAfter)},
		{"entity_event_handler", "event", stringSet(domainmodel.EventTypeCreate, domainmodel.EventTypeCommit,
			domainmodel.EventTypeDelete, domainmodel.EventTypeRollback)},
		{"layout", "layout_type", stringSet(pages.LayoutTypeResponsive, pages.LayoutTypeDefault, pages.LayoutTypeTablet,
			pages.LayoutTypePhone, pages.LayoutTypeModalPopup, pages.LayoutTypePopup, pages.LayoutTypeLegacy)},
		{"published_rest_operation", "http_method", stringSet(metamodel.RestHTTPMethodGet, metamodel.RestHTTPMethodPost,
			metamodel.RestHTTPMethodPut, metamodel.RestHTTPMethodPatch, metamodel.RestHTTPMethodDelete,
			metamodel.RestHTTPMethodHead, metamodel.RestHTTPMethodOptions)},
		{"navigation_menu_item", "action_type", navigationActionTypes(t)},
	} {
		// The "Rollback" warning in the event row names the spelling that
		// matches nothing; it is documented on purpose.
		var vals []string
		for _, v := range sectionRowValues(t, doc, tc.section, tc.field) {
			if tc.field == "event" && v == "Rollback" {
				continue
			}
			vals = append(vals, v)
		}
		assertDocumentedValuesExist(t, tc.section+"."+tc.field, vals, tc.real, "the reader")
	}
}
