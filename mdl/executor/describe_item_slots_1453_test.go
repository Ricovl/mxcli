// SPDX-License-Identifier: Apache-2.0

// An object-list item can hold widgets in more than one slot — an Accordion
// group has `headerContent` and `content`. DESCRIBE folded every slot into the
// item's body, so a header widget was printed beside the body widgets, and a
// page regenerated from that description stored it in `content` (measured on a
// blank 11.13.0 app: headerContent -> [txtHead], content -> [txtBody] became
// content -> [txtHead, txtBody]). And `check` refused the block form CREATE
// builds from — `headercontent { … }` inside a group was MDL-WIDGET25 "not a
// widget in this project" — so the honest description could not run either.
// (ako/mxcli#1326, mendixlabs/mxcli#1453; prerequisite of
// PROPOSAL_alter_object_list_item_address.md.)
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// accordionWithBothSlots is a group holding txtHead in headerContent and
// txtBody in content, with the schema declaring headerContent FIRST — so a
// "first widget slot is the body" rule would pick the wrong one.
func accordionWithBothSlots() map[string]any {
	const (
		idGroups = "type-id-groups"
		idHead   = "type-id-headerContent"
		idBody   = "type-id-content"
	)
	text := func(name, s string) map[string]any {
		return map[string]any{"$Type": "Forms$StaticText", "Name": name,
			"Text": map[string]any{"Items": []any{map[string]any{"Text": s}}}}
	}
	return map[string]any{
		"Name": "acc1",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.accordion.Accordion",
			"ObjectType": map[string]any{"PropertyTypes": []any{
				map[string]any{"$ID": idGroups, "PropertyKey": "groups",
					"ValueType": map[string]any{"ObjectType": map[string]any{"PropertyTypes": []any{
						map[string]any{"$ID": idHead, "PropertyKey": "headerContent",
							"ValueType": map[string]any{"Type": "Widgets"}},
						map[string]any{"$ID": idBody, "PropertyKey": "content",
							"ValueType": map[string]any{"Type": "Widgets"}},
					}}}},
			}},
		},
		"Object": map[string]any{"Properties": []any{
			map[string]any{"TypePointer": idGroups, "Value": map[string]any{"Objects": []any{
				map[string]any{"Properties": []any{
					map[string]any{"TypePointer": idHead, "Value": map[string]any{"Widgets": []any{text("txtHead", "Header")}}},
					map[string]any{"TypePointer": idBody, "Value": map[string]any{"Widgets": []any{text("txtBody", "Body")}}},
				}},
			}}},
		}},
	}
}

func TestObjectListItem_BodyAndOtherSlotsKeptApart_1453(t *testing.T) {
	lists := extractObjectLists(nil, accordionWithBothSlots())
	if len(lists) != 1 || len(lists[0].Items) != 1 {
		t.Fatalf("want one group, got %+v", lists)
	}
	item := lists[0].Items[0]
	if len(item.Children) != 1 || item.Children[0].Name != "txtBody" {
		t.Fatalf("body = %+v, want only txtBody — headerContent was folded into the body", item.Children)
	}
	if len(item.Slots) != 1 || item.Slots[0].Keyword != "headercontent" ||
		len(item.Slots[0].Widgets) != 1 || item.Slots[0].Widgets[0].Name != "txtHead" {
		t.Fatalf("slots = %+v, want headercontent: [txtHead]", item.Slots)
	}
}

func TestObjectListItem_EmitsHeaderContentBlock_1453(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, rawWidget{
		Type:        "CustomWidgets$CustomWidget",
		RenderMode:  "accordion",
		Name:        "acc1",
		WidgetID:    "com.mendix.widget.web.accordion.Accordion",
		ObjectLists: extractObjectLists(nil, accordionWithBothSlots()),
	}, 0)
	out := buf.String()
	block := strings.Index(out, "headercontent {")
	head := strings.Index(out, "txtHead")
	body := strings.Index(out, "txtBody")
	if block < 0 || head < block || body < 0 {
		t.Fatalf("want txtHead inside a `headercontent { … }` block of the group:\n%s", out)
	}
	if end := strings.Index(out[block:], "}"); block+end > body {
		t.Fatalf("txtBody must not be inside the headercontent block:\n%s", out)
	}
}

// With no `content` slot the body is the item's first widget slot, as CREATE's
// defaultItemSlotKey routes loose children.
func TestBodyItemSlotKey_1453(t *testing.T) {
	for _, c := range []struct {
		keys []string
		want string
	}{
		{[]string{"headerContent", "content"}, "content"},
		{[]string{"children", "extra"}, "children"},
		{nil, ""},
	} {
		if got := bodyItemSlotKey(c.keys); got != c.want {
			t.Errorf("bodyItemSlotKey(%v) = %q, want %q", c.keys, got, c.want)
		}
	}
}

// The block describe now prints must pass check: an item's own slot keyword is
// a container of that item, not an unknown widget.
func TestCheckAcceptsItemSlotBlock_1453(t *testing.T) {
	prog, errs := visitor.Build(`create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.accordion.Accordion' acc1 {
    group (headerRenderMode: 'custom') {
      headercontent { dynamictext txtHead (content: 'Header') }
      dynamictext txtBody (content: 'Body')
    }
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], accordionRegistry(t)) {
		if v.Severity == linter.SeverityError {
			t.Errorf("an item slot block was refused: %s: %s", v.RuleID, v.Message)
		}
	}
}

// The control for the test above: a keyword that is NOT one of the item's
// slots is still refused, so the fix did not just switch the rule off.
func TestCheckStillRefusesUnknownBlockInItem_1453(t *testing.T) {
	prog, errs := visitor.Build(`create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  pluggablewidget 'com.mendix.widget.web.accordion.Accordion' acc1 {
    group (headerRenderMode: 'custom') {
      headercontnt { dynamictext txtHead (content: 'Header') }
    }
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], accordionRegistry(t)) {
		if v.RuleID == "MDL-WIDGET25" || v.RuleID == "MDL-WIDGET26" {
			return
		}
	}
	t.Fatal("a misspelt slot block inside a group was not refused")
}
