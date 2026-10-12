// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// A navigation list item's Forms$Appearance carries Class, Style and
// DynamicClasses like any widget's (measured: every item in PedApp and the
// WidgetDemo showcase stores all three). Describe printed the class as
// `ButtonStyle: nav-orders` — a property an item does not have, which the item
// builder ignored — and the builder wrote an empty appearance whatever the item
// said, so a styled item lost its class on describe → exec. Found while adding
// item addresses for mendixlabs/mxcli#1388.
func TestNavigationListItemAppearance_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	stored := map[string]any{
		"$Type": "Forms$NavigationList",
		"Name":  "navList1",
		"Items": []any{int32(2), map[string]any{
			"$Type":  "Forms$NavigationListItem",
			"Action": map[string]any{"$Type": "Forms$SignOutClientAction"},
			"Appearance": map[string]any{
				"$Type":          "Forms$Appearance",
				"Class":          "nav-orders",
				"Style":          "color: red;",
				"DynamicClasses": "if $currentUser = empty then 'anon' else ''",
			},
			"Widgets": []any{int32(2)},
		}},
	}
	outputWidgetMDLV3(ctx, parseRawWidget(ctx, stored)[0], 1)
	got := buf.String()
	if strings.Contains(got, "ButtonStyle") {
		t.Errorf("describe prints the item's class as ButtonStyle:\n%s", got)
	}
	for _, want := range []string{"Class: 'nav-orders'", "Style: 'color: red;'", "DynamicClasses: if $currentUser = empty then 'anon' else ''"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output lacks %q:\n%s", want, got)
		}
	}

	prog, errs := visitor.Build("create snippet M.S {\n" + got + "}\n")
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, got)
	}
	itemNode := prog.Statements[0].(*ast.CreateSnippetStmtV3).Widgets[0].Children[0]
	pb := &pageBuilder{ctx: ctx, widgetScope: map[string]model.ID{}}
	item, err := pb.buildNavigationListItemV3(itemNode)
	if err != nil {
		t.Fatal(err)
	}
	if item.Class != "nav-orders" || item.Style != "color: red;" || item.DynamicClasses != "if $currentUser = empty then 'anon' else ''" {
		t.Errorf("built item appearance = class %q style %q dynamic %q, want what describe printed",
			item.Class, item.Style, item.DynamicClasses)
	}
}
