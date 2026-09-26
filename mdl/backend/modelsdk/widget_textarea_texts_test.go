// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Studio Pro stores a TextTooLongMessage on every textarea (a Texts$Text, en_US
// "" when unset), and the writer never emitted the key. On a rewrite the
// translation carry had no text to put the stored languages into, so the
// message was deleted (ako/mxcli#705 item 1, FeedbackModule.ShareFeedback's
// textArea2). Written as an empty text, like CounterMessage beside it: the
// carry fills in what the stored document had, and a new textarea gets the
// shape Studio Pro gives it.
func TestTextAreaToGen_WritesTextTooLongMessage(t *testing.T) {
	g, err := widgetToGen(&pages.TextArea{BaseWidget: pages.BaseWidget{Name: "textArea1"}})
	if err != nil {
		t.Fatalf("widgetToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, key := range []string{"TextTooLongMessage", "CounterMessage"} {
		v, err := bson.Raw(raw).LookupErr(key)
		if err != nil {
			t.Errorf("%s not written: %v", key, err)
			continue
		}
		doc, ok := v.DocumentOK()
		if !ok {
			t.Errorf("%s = %v, want a Texts$Text", key, v)
			continue
		}
		if ty := doc.Lookup("$Type").StringValue(); ty != "Texts$Text" {
			t.Errorf("%s.$Type = %q, want Texts$Text", key, ty)
		}
	}
}
