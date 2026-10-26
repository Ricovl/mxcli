// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1704: describe → exec rewrote JTSBootLogboek.Home_Native — "only the
// order of its two placeholders and of some English/Dutch texts changed".
// describe prints a multi-placeholder page's `placeholder X { … }` blocks in
// stored order; the builder put Main first whatever the blocks said, so a page
// stored [Header, Main] came back [Main, Header].
package executor

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

func argumentOrder(t *testing.T, blocks []string, bare bool) string {
	t.Helper()
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	var phs []*ast.PagePlaceholderV3
	for _, b := range blocks {
		phs = append(phs, &ast.PagePlaceholderV3{Name: b})
	}
	var bareWidgets []*ast.WidgetV3
	if bare {
		bareWidgets = []*ast.WidgetV3{} // an empty body still binds Main
	}
	args, err := pb.buildLayoutCallArguments("L.Layout", bareWidgets, phs)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range args {
		names = append(names, strings.TrimPrefix(string(a.ParameterID), "L.Layout."))
	}
	return strings.Join(names, ",")
}

func TestLayoutCallArguments_FollowTheBlockOrder(t *testing.T) {
	for _, tc := range []struct {
		blocks []string
		want   string
	}{
		// The reported shape: Main is not the first block.
		{[]string{"Header", "Main"}, "Header,Main"},
		{[]string{"Main", "Header"}, "Main,Header"},
		// Controls: without a Main block, Main is first and always emitted.
		{[]string{"Header"}, "Main,Header"},
		{nil, "Main"},
	} {
		if got := argumentOrder(t, tc.blocks, false); got != tc.want {
			t.Errorf("blocks %v: arguments %s, want %s", tc.blocks, got, tc.want)
		}
	}
}

// The reported round trip on a PedApp page whose stored arguments are [Header,
// Main] and whose texts are stored Dutch first, as Studio Pro stores them.
func TestDescribeExec_KeepsArgumentAndLanguageOrder(t *testing.T) {
	exec, out := openPedAppFixture(t)
	src := `create page MyFirstModule.Home_Native (Title: 'Home', Layout: Atlas_Core.NativePhone_Default) {
  placeholder Main { dynamictext textMain (Content: 'Welcome') }
  placeholder Header { dynamictext textHeader (Content: 'Header') }
};`
	if err := afRun(t, exec, src); err != nil {
		t.Fatalf("exec: %v", err)
	}
	id, raw := pageUnitByName(t, exec, "Home_Native")
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	// Studio Pro's shape: Header's argument first, and a Dutch translation
	// stored ahead of the English on every text.
	nl := 0
	walkStoredDocs(doc, func(d bson.D) {
		for i := range d {
			switch {
			case d[i].Key == "Arguments" && afGet(d, "$Type") == "Forms$LayoutCall":
				args := d[i].Value.(bson.A) // [marker, Main, Header]
				d[i].Value = bson.A{args[0], args[2], args[1]}
			case d[i].Key == "Items" && afGet(d, "$Type") == "Texts$Text":
				items := d[i].Value.(bson.A)
				if len(items) != 2 {
					continue
				}
				en := items[1].(bson.D)
				nlElem := bson.D{}
				for _, e := range en {
					switch e.Key {
					case "$ID":
						id := e.Value.(bson.Binary)
						data := append([]byte(nil), id.Data...)
						data[len(data)-1] ^= 0xFF // a distinct element id
						nlElem = append(nlElem, bson.E{Key: "$ID", Value: bson.Binary{Subtype: id.Subtype, Data: data}})
					case "LanguageCode":
						nlElem = append(nlElem, bson.E{Key: e.Key, Value: "nl_NL"})
					case "Text":
						nlElem = append(nlElem, bson.E{Key: e.Key, Value: "NL " + e.Value.(string)})
					default:
						nlElem = append(nlElem, e)
					}
				}
				d[i].Value = bson.A{items[0], nlElem, en}
				nl++
			}
		}
	})
	if nl == 0 {
		t.Fatal("no text to translate on the page")
	}
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.newExecContext(context.Background()).Backend.UpdateRawUnit(string(id), b); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := afRun(t, exec, "describe page MyFirstModule.Home_Native;"); err != nil {
		t.Fatal(err)
	}
	described := out.String()
	// Header is a keyword, so describe quotes it.
	if h, m := strings.Index(described, `placeholder "Header"`), strings.Index(described, "placeholder Main"); h < 0 || m < 0 || h > m {
		t.Fatalf("describe did not print the stored placeholder order:\n%s", described)
	}
	out.Reset()
	if err := afRun(t, exec, described); err != nil {
		t.Fatalf("re-exec of the description: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("re-executing the description was not a no-op:\n%s", out.String())
	}
}
