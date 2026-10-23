// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// An else-if chain: `A ? (…) : B ? (…hides…) : C && hide(…)`. A hide in the B
// branch fires when B holds and A does not; one in the C branch when C holds
// and neither A nor B does. Combo box writes its three option sources this way:
//
//	"context"===t.source ? (…) : "database"===t.source ? (…) : "static"===t.source && hide(…)
//
// Every hide after the first `:` was unread — its guard's boundary is the outer
// ternary's `:`, which parseGuard treats as "a fragment of something larger" —
// so nothing the database or static source hides was known to be hidden. A
// database-mode combo box then kept the template's `optionsSourceType` and
// `selectAllButtonCaption`, both hidden there, and mx check reported CE0463
// (mendixlabs/mxcli#1583).

const elseIfChainJS = `function getProperties(t,e){` +
	`return"context"===e.source?(x.hidePropertiesIn(t,e,["a"])):` +
	`"database"===e.source?(x.hidePropertiesIn(t,e,["b","c"]),` +
	`"attribute"===e.ct?x.hidePropertiesIn(t,e,["d"]):x.hidePropertiesIn(t,e,["f"])):` +
	`"static"===e.source&&x.hidePropertiesIn(t,e,["g"]),t}`

// ruleHidesUnder reports whether any rule for prop fires under values, and
// whether a rule for it exists at all.
func ruleHidesUnder(rules []types.WidgetVisibilityRule, prop string, values map[string]string) (hidden, found bool) {
	for _, r := range rules {
		if r.PropertyKey != prop || r.ListPropertyKey != "" || r.HiddenWhen == nil {
			continue
		}
		found = true
		fires, determinable := r.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
			v, ok := values[c.PropertyKey]
			return v, ok
		})
		if determinable && fires {
			hidden = true
		}
	}
	return hidden, found
}

func TestElseIfChainBranchesAreLifted(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(elseIfChainJS)

	cases := []struct {
		prop   string
		values map[string]string
		want   bool
	}{
		// The first branch, which already worked — the control.
		{"a", map[string]string{"source": "context"}, true},
		{"a", map[string]string{"source": "database"}, false},
		// The else-if's group: its first hide and its comma sibling.
		{"b", map[string]string{"source": "database"}, true},
		{"b", map[string]string{"source": "context"}, false},
		{"b", map[string]string{"source": "static"}, false},
		{"c", map[string]string{"source": "database"}, true},
		{"c", map[string]string{"source": "static"}, false},
		// A ternary nested inside the else-if's group carries both terms.
		{"d", map[string]string{"source": "database", "ct": "attribute"}, true},
		{"d", map[string]string{"source": "database", "ct": "expression"}, false},
		{"d", map[string]string{"source": "context", "ct": "attribute"}, false},
		{"f", map[string]string{"source": "database", "ct": "expression"}, true},
		{"f", map[string]string{"source": "database", "ct": "attribute"}, false},
		{"f", map[string]string{"source": "static", "ct": "expression"}, false},
		// The last branch, `: C && hide(…)`, guarded with no group of its own.
		{"g", map[string]string{"source": "static"}, true},
		{"g", map[string]string{"source": "database"}, false},
		{"g", map[string]string{"source": "context"}, false},
	}
	for _, c := range cases {
		got, found := ruleHidesUnder(rules, c.prop, c.values)
		if !found {
			t.Errorf("%s: no rule lifted", c.prop)
			continue
		}
		if got != c.want {
			t.Errorf("%s under %v: hidden=%v, want %v", c.prop, c.values, got, c.want)
		}
	}
}

// The terms of an else-if are only exact when every earlier condition is read.
// One the extractor cannot represent leaves the branch unlifted rather than
// lifted on its own condition — which would hide `b` wherever source is
// "database", including where the unread first branch runs instead.
func TestElseIfChainWithAnUnreadableBranchIsNotLifted(t *testing.T) {
	js := `function getProperties(t,e){` +
		`return e.source.startsWith("c")?(x.hidePropertiesIn(t,e,["a"])):` +
		`"database"===e.source?(x.hidePropertiesIn(t,e,["b"])):0,t}`
	rules, _ := extractVisibilityRulesFromJS(js)
	if _, found := ruleHidesUnder(rules, "b", nil); found {
		t.Errorf("lifted a rule for b although the chain's first condition is unreadable: %+v", rules)
	}
}

// The real widget: Combo box 2.5.0, the version a blank 11.12.5 app ships.
// optionsSourceType and selectAllButtonCaption are what mx update-widgets
// resets on a database-mode combo box mxcli wrote — the two differences
// between it and a widget that builds.
func TestComboBoxDatabaseBranchIsLifted(t *testing.T) {
	js, err := mpk.ReadEditorConfig(
		"../../testdata/expr-checker/widgets/com.mendix.widget.web.Combobox.mpk",
		"com.mendix.widget.web.combobox.Combobox")
	if err != nil || js == "" {
		t.Fatalf("read Combo box editorConfig: %v", err)
	}
	rules, _ := extractVisibilityRulesFromJS(js)

	database := map[string]string{
		"source": "database", "optionsSourceType": "enumeration", "selectAllButton": "true",
		"optionsSourceDatabaseCaptionType": "attribute", "optionsSourceDatabaseItemSelection": "Single",
	}
	for _, prop := range []string{"optionsSourceType", "selectAllButtonCaption", "onChangeEvent",
		"optionsSourceDatabaseCaptionExpression", "selectedItemsSorting"} {
		if hidden, found := ruleHidesUnder(rules, prop, database); !found || !hidden {
			t.Errorf("%s: not hidden in database mode (rule found: %v)", prop, found)
		}
	}
	// Shown in database mode: the rules must not over-fire.
	for _, prop := range []string{"optionsSourceDatabaseCaptionAttribute", "optionsSourceDatabaseDataSource",
		"databaseAttributeString"} {
		if hidden, _ := ruleHidesUnder(rules, prop, database); hidden {
			t.Errorf("%s: hidden in database mode, where the editor shows it", prop)
		}
	}
	// And the control: in context/association mode with the select-all button
	// on, both properties are shown.
	association := map[string]string{
		"source": "context", "optionsSourceType": "association", "selectAllButton": "true",
		"optionsSourceAssociationCustomContentType": "no", "showFooter": "false",
		"optionsSourceAssociationCaptionType": "attribute",
	}
	for _, prop := range []string{"optionsSourceType", "selectAllButtonCaption"} {
		if hidden, _ := ruleHidesUnder(rules, prop, association); hidden {
			t.Errorf("%s: hidden in association mode, where the editor shows it", prop)
		}
	}
}
