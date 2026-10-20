// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1411: a layout built on another layout. The shapes below are
// ConversationalUI.Layout_MasterBase as Studio Pro stores it (Conversational UI
// 7.3.1, Mendix 11.12.4):
//
//	Content: Forms$WebLayoutContent
//	  LayoutCall: Forms$LayoutCall            {$ID $Type Arguments Form}
//	    Arguments:
//	      - Forms$FormCallArgument            {$ID $Type Parameter Widgets}
//	          Parameter: "Atlas_Core.Atlas_Default.Main"
//	          Widgets: [Forms$Placeholder Main]
//	    Form: "Atlas_Core.Atlas_Default"
//	  LayoutType: "Responsive"
//	  Widgets: []
//
// The key is LayoutCall. gen bound WebLayoutContent's call to "FormCall" — the
// key a Forms$Page uses — so a call set through it was written where no Studio
// Pro document keeps one, and a stored one never decoded.
func nestedLayout() *pages.Layout {
	return &pages.Layout{
		Name:       "Layout_MasterBase",
		LayoutType: pages.LayoutTypeResponsive,
		LayoutCall: &pages.LayoutCall{
			LayoutName: "Atlas_Core.Atlas_Default",
			Arguments: []*pages.LayoutCallArgument{{
				ParameterID: "Atlas_Core.Atlas_Default.Main",
				Widgets:     []pages.Widget{&pages.LayoutPlaceholder{BaseWidget: pages.BaseWidget{Name: "Main"}}},
			}},
		},
	}
}

func keySet(m map[string]any) string {
	k := keysOf(m)
	sort.Strings(k)
	return strings.Join(k, ",")
}

func TestLayoutToGen_WritesTheLayoutCallWhereStudioProKeepsIt(t *testing.T) {
	g, err := layoutToGen(nestedLayout())
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	content, ok := doc["Content"].(map[string]any)
	if !ok {
		t.Fatalf("Content missing; keys = %v", keysOf(doc))
	}
	if _, wrong := content["FormCall"]; wrong {
		t.Error("wrote FormCall on the content wrapper; Studio Pro keeps the call under LayoutCall")
	}
	call, ok := content["LayoutCall"].(map[string]any)
	if !ok {
		t.Fatalf("no LayoutCall on the content wrapper; keys = %v", keysOf(content))
	}
	if got, want := keySet(call), "$ID,$Type,Arguments,Form"; got != want {
		t.Errorf("layout call keys = %s, want %s", got, want)
	}
	if call["$Type"] != "Forms$LayoutCall" || call["Form"] != "Atlas_Core.Atlas_Default" {
		t.Errorf("layout call = %v", call)
	}
	args, _ := call["Arguments"].(bson.A)
	if len(args) != 2 { // marker + one argument
		t.Fatalf("arguments = %v", call["Arguments"])
	}
	arg := args[1].(map[string]any)
	if got, want := keySet(arg), "$ID,$Type,Parameter,Widgets"; got != want {
		t.Errorf("argument keys = %s, want %s", got, want)
	}
	if arg["$Type"] != "Forms$FormCallArgument" || arg["Parameter"] != "Atlas_Core.Atlas_Default.Main" {
		t.Errorf("argument = %v", arg)
	}
	widgets, _ := content["Widgets"].(bson.A)
	if len(widgets) != 1 { // marker only
		t.Errorf("a nested layout's content widgets must be empty, got %v", content["Widgets"])
	}

	// And it reads back through gen: the decode side of the same key.
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	back, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	wc, ok := back.(*genPg.Layout).Content().(*genPg.WebLayoutContent)
	if !ok || wc.LayoutCall() == nil {
		t.Fatal("the stored layout call does not decode")
	}
}

// The placeholders a nested layout declares sit inside its arguments; they are
// what the CE0848 rule counts. One with none is still refused.
func TestLayoutToGen_NestedLayoutPlaceholdersAreCounted(t *testing.T) {
	if _, err := layoutToGen(nestedLayout()); err != nil {
		t.Fatalf("Main declared inside an argument was not counted: %v", err)
	}
	l := nestedLayout()
	l.LayoutCall.Arguments[0].Widgets = nil
	if _, err := layoutToGen(l); err == nil || !strings.Contains(err.Error(), "CE0848") {
		t.Errorf("want CE0848 refusal, got %v", err)
	}
}

// Forms$NativeLayoutContent has no LayoutCall property (generated/metamodel's
// PagesNativeLayoutContent declares none), so a native layout cannot wrap one.
func TestLayoutToGen_RefusesANativeNestedLayout(t *testing.T) {
	l := nestedLayout()
	l.Native, l.LayoutType = true, pages.LayoutTypeDefault
	if _, err := layoutToGen(l); err == nil {
		t.Fatal("native layout with a layout call accepted")
	}
}
