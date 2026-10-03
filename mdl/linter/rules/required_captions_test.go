// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/translations"

	_ "modernc.org/sqlite"
)

func objectsDB(t *testing.T) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE objects (Id TEXT, QualifiedName TEXT, Name TEXT, ModuleName TEXT, ObjectType TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO objects VALUES
		('u1', 'Administration.Account_Overview', 'Account_Overview', 'Administration', 'PAGE'),
		('u2', 'Shop.Header', 'Header', 'Shop', 'SNIPPET')`); err != nil {
		t.Fatal(err)
	}
	return catalog.WrapSqlDB(db)
}

// QUAL006 (ako/mxcli#944): a tab page caption with no text in the default
// language is the CE4899 the de_DE build reported and QUAL005 never did. Each
// one is an error named by its document, with the alter that fixes it.
func TestRequiredCaptionDefaultLanguage_NamesEachCaption(t *testing.T) {
	db := objectsDB(t)
	defer db.Close()
	ctx := linter.NewLintContextFromDB(db)
	r := NewRequiredCaptionDefaultLanguageRule()
	missing := []translations.MissingCaption{
		{UnitID: "u1", UnitType: "Forms$Page", UnitName: "Account_Overview", Kind: "tab page caption", OwnerName: "tabPage2", Sample: "Local Users"},
		{UnitID: "u2", UnitType: "Forms$Snippet", UnitName: "Header", Kind: "tab page caption", OwnerName: "tpMain"},
	}
	vs := r.violations(ctx, "de_DE", missing)
	if len(vs) != 2 {
		t.Fatalf("want 2, got %+v", vs)
	}
	v := vs[0]
	if v.RuleID != "QUAL006" || v.Severity != linter.SeverityError {
		t.Errorf("rule/severity = %s/%v", v.RuleID, v.Severity)
	}
	if !strings.Contains(v.Message, `Administration.Account_Overview: tab page caption tabPage2 ("Local Users") has no text in the default language de_DE`) {
		t.Errorf("message = %s", v.Message)
	}
	if v.Location.Module != "Administration" || v.Location.DocumentName != "Account_Overview" || v.Location.DocumentType != "page" {
		t.Errorf("location = %+v", v.Location)
	}
	if !strings.Contains(vs[1].Suggestion, "alter snippet Shop.Header { set (Caption: '…') on tpMain; }") {
		t.Errorf("snippet suggestion = %s", vs[1].Suggestion)
	}

	// The module filter applies.
	ctx.SetExcludedModules([]string{"Shop"})
	if vs := r.violations(ctx, "de_DE", missing); len(vs) != 1 {
		t.Errorf("Shop excluded; want 1, got %d", len(vs))
	}
}

// Without a project reader (no units, no default language) the rule is silent
// rather than guessing a language.
func TestRequiredCaptionDefaultLanguage_NoReaderIsSilent(t *testing.T) {
	db := objectsDB(t)
	defer db.Close()
	if vs := NewRequiredCaptionDefaultLanguageRule().Check(linter.NewLintContextFromDB(db)); len(vs) != 0 {
		t.Errorf("got %+v", vs)
	}
}
