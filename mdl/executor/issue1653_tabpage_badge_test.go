// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"crypto/rand"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#1644 (ako/mxcli#1653): a tab page's Badge — a text template
// with parameters, Forms$TabPage.Badge — had no MDL spelling. `check` warned
// MDL-WIDGET07 "will be silently dropped", describe printed nothing for it, and
// re-executing describe's output stored "Badge": null, so a badge set in Studio
// Pro was removed by any rewrite of its page.

const issue1653Page = `create or modify page MyFirstModule.Panel_Tabs (
  Title: 'Tabs',
  Layout: Atlas_Core.Atlas_Default,
  Params: ( $Panel: MyFirstModule.Panel )
) {
  dataview dvPanel (DataSource: $Panel) {
    dynamictext txtTotal (Content: '{1}', ContentParams: ({1} = Total))
    tabcontainer tcMain {
      tabpage tpOne (Caption: 'One', Badge: '{1} open', BadgeParams: ({1} = Total)) { dynamictext txtOne (Content: 'one') }
      tabpage tpTwo (Caption: 'Two') { dynamictext txtTwo (Content: 'two') }
    }
  }
};`

func issue1653Executor(t *testing.T) (*Executor, interface {
	Reset()
	String() string
}, string) {
	t.Helper()
	exec, out := pageFixtureExecutor(t)
	run(t, exec, `create non-persistent entity MyFirstModule.Panel (Total: Integer);`)
	return exec, out, ""
}

// storedBadge is the stored Badge of the tab page named tab.
func storedBadge(t *testing.T, exec *Executor, id, tab string) any {
	t.Helper()
	return bsonnav.DGet(namedDoc(t, storedUnitDoc(t, exec, id), tab), "Badge")
}

func badgeTemplate(t *testing.T, badge any) (text string, attrs []string) {
	t.Helper()
	ct, ok := badge.(bson.D)
	if !ok || bsonnav.DGetString(ct, "$Type") != "Forms$ClientTemplate" {
		t.Fatalf("Badge is %v, want a Forms$ClientTemplate", badge)
	}
	for _, tr := range bsonDocs(bsonValue(bsonDocField(ct, "Template"), "Items")) {
		if bsonString(tr, "LanguageCode") == "en_US" {
			text = bsonString(tr, "Text")
		}
	}
	for _, p := range bsonDocs(bsonValue(ct, "Parameters")) {
		attrs = append(attrs, bsonnav.DGetString(bsonDocField(p, "AttributeRef"), "Attribute"))
	}
	return text, attrs
}

func TestIssue1653_TabPageBadgeIsWritten(t *testing.T) {
	exec, out, _ := issue1653Executor(t)
	run(t, exec, issue1653Page)
	id := unitIDByName(t, exec, "page", "Panel_Tabs")

	text, attrs := badgeTemplate(t, storedBadge(t, exec, id, "tpOne"))
	if text != "{1} open" || strings.Join(attrs, ",") != "MyFirstModule.Panel.Total" {
		t.Errorf("tpOne's Badge is %q bound to %v, want '{1} open' bound to MyFirstModule.Panel.Total", text, attrs)
	}
	// A tab page without one stores none, as Studio Pro does.
	if b := storedBadge(t, exec, id, "tpTwo"); b != nil {
		t.Errorf("tpTwo stores a Badge it was not given: %v", b)
	}

	// describe prints it, and its output runs again without writing.
	out.Reset()
	run(t, exec, "describe page MyFirstModule.Panel_Tabs")
	described := out.String()
	if !strings.Contains(described, "tabpage tpOne (Caption: 'One', Badge: '{1} open', BadgeParams: ({1} = Total))") {
		t.Fatalf("describe does not print the badge:\n%s", described)
	}
	if strings.Contains(described, "tabpage tpTwo (Caption: 'Two', Badge") {
		t.Errorf("describe prints a badge on tpTwo:\n%s", described)
	}
	out.Reset()
	run(t, exec, described)
	if got := out.String(); !strings.Contains(got, "Unchanged page") {
		t.Errorf("re-running describe wrote the page:\n%s", got)
	}
}

// The report's own scenario: a badge as Studio Pro stores it — a copy of a
// dynamic text's Content template in TabPage.Badge — survives describe → exec.
func TestIssue1653_DescribeRoundTripKeepsAStudioProBadge(t *testing.T) {
	exec, out, _ := issue1653Executor(t)
	run(t, exec, strings.Replace(issue1653Page, ", Badge: '{1} open', BadgeParams: ({1} = Total)", "", 1))
	id := unitIDByName(t, exec, "page", "Panel_Tabs")

	doc := storedUnitDoc(t, exec, id)
	badge := cloneBsonD(bsonDocField(namedDoc(t, doc, "txtTotal"), "Content"))
	freshIDs(badge)
	if !bsonnav.DSet(namedDoc(t, doc, "tpOne"), "Badge", badge) {
		t.Fatal("the stored tab page has no Badge key to set")
	}
	writeUnitDoc(t, exec, id, doc)
	if text, _ := badgeTemplate(t, storedBadge(t, exec, id, "tpOne")); text != "{1}" {
		t.Fatalf("seed did not take: %q", text)
	}

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Panel_Tabs")
	described := strings.Replace(out.String(), "Title: 'Tabs'", "Title: 'Tabs (changed)'", 1) // force the write
	run(t, exec, described)
	if !storedHas(t, exec, id, "Tabs (changed)") {
		t.Fatal("the rewrite did not land, so it shows nothing about the badge")
	}
	text, attrs := badgeTemplate(t, storedBadge(t, exec, id, "tpOne"))
	if text != "{1}" || strings.Join(attrs, ",") != "MyFirstModule.Panel.Total" {
		t.Errorf("after describe → exec tpOne's Badge is %q bound to %v", text, attrs)
	}
}

// freshIDs gives every element of a copied document an id of its own, as the
// report's script does: two elements sharing one is a unit Studio Pro refuses.
func freshIDs(v any) {
	switch x := v.(type) {
	case bson.D:
		for i, e := range x {
			if e.Key == "$ID" {
				x[i].Value = bson.Binary{Subtype: 0, Data: freshID()}
				continue
			}
			freshIDs(e.Value)
		}
	case bson.A:
		for _, e := range x {
			freshIDs(e)
		}
	}
}

func freshID() []byte {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func issue1653Violations(t *testing.T, src string) []string {
	t.Helper()
	var widgets []*ast.WidgetV3
	for _, s := range parseMDL(t, src).Statements {
		if p, ok := s.(*ast.CreatePageStmtV3); ok {
			widgets = p.Widgets
		}
	}
	var out []string
	for _, v := range validateWidgetTree(widgets, LoadWidgetRegistry(""), "page M.P") {
		out = append(out, v.RuleID+" "+v.Message)
	}
	return out
}

func TestIssue1653_CheckAcceptsTheBadgeOnATabPageOnly(t *testing.T) {
	for _, v := range issue1653Violations(t, issue1653Page) {
		if strings.Contains(v, "Badge") {
			t.Errorf("check reports the tab page badge: %s", v)
		}
	}
	// A badge is a tab page's: anywhere else it is still dropped, and said so.
	got := issue1653Violations(t, `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  dynamictext txt (Content: 'a', Badge: 'b')
};`)
	if len(got) == 0 || !strings.Contains(strings.Join(got, "\n"), nativeWidgetPropDropped.Code) {
		t.Errorf("Badge on a dynamic text is not reported: %v", got)
	}
}

// A badge in a shape the tab page cannot store is refused, by check and by exec,
// rather than written as something else.
func TestIssue1653_MalformedBadgeIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, props, want string }{
		{"parameters without a template", "Caption: 'One', BadgeParams: ({1} = Total)", "has no Badge template"},
		{"an expression, not a template", "Caption: 'One', Badge: $Panel/Total", "written in quotes"},
	} {
		src := strings.Replace(issue1653Page, "Caption: 'One', Badge: '{1} open', BadgeParams: ({1} = Total)", tc.props, 1)
		got := strings.Join(issue1653Violations(t, src), "\n")
		if !strings.Contains(got, "MDL-WIDGET51") || !strings.Contains(got, tc.want) {
			t.Errorf("%s: check reports %q", tc.name, got)
		}

		exec, _, _ := issue1653Executor(t)
		prog := parseMDL(t, src)
		err := exec.Execute(prog.Statements[0])
		if err == nil || !strings.Contains(err.Error(), "MDL-WIDGET51") {
			t.Errorf("%s: exec wrote the page (%v)", tc.name, err)
		}
	}
}
