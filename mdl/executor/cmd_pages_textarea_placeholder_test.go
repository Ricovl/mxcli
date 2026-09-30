// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#705 item 1. A textarea's placeholder was neither read by DESCRIBE,
// printed, nor consumed by the builder — only the textbox's was — so describe
// -> exec deleted it, with every translation it carried. On the Blank
// template's FeedbackModule.ShareFeedback that was nine translations of one
// placeholder, most of the 113 -> 102 the audit counted.
package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestTextAreaPlaceholder_RoundTrips(t *testing.T) {
	ctx, _ := newMockCtx(t)
	raw := map[string]any{
		"$Type": "Forms$TextArea",
		"Name":  "textArea2",
		"PlaceholderTemplate": map[string]any{
			"$Type": "Forms$ClientTemplate",
			"Template": map[string]any{
				"$Type": "Texts$Text",
				"Items": []any{
					map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "Please add a detailed description"},
				},
			},
		},
	}

	got := parseRawWidget(ctx, raw)
	if len(got) != 1 || got[0].Placeholder != "Please add a detailed description" {
		t.Fatalf("parsed textarea = %+v, want its placeholder read", got)
	}

	var buf bytes.Buffer
	ctx.Output = &buf
	outputWidgetMDLV3(ctx, got[0], 0)
	assertContainsStr(t, buf.String(), "Placeholder: 'Please add a detailed description'")

	ta, err := newPropsBuilder().buildTextAreaV3(&ast.WidgetV3{Name: "textArea2", Type: "textarea",
		Properties: map[string]any{"Placeholder": "Please add a detailed description"}})
	if err != nil {
		t.Fatalf("buildTextAreaV3: %v", err)
	}
	if ta.Placeholder == nil || len(ta.Placeholder.Translations) != 1 ||
		ta.Placeholder.Translations[authoringLanguage(nil)] != "Please add a detailed description" {
		t.Errorf("built textarea Placeholder = %+v, want the authored text", ta.Placeholder)
	}
}
