// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1476, the read half: an Image widget bound to the object in
// context is stored with an EntityRef on `imageObject` and no collection entry.
// DESCRIBE must print `Image: $currentObject` for it, or describe -> exec
// writes a widget that names no image ("No image selected.").

// boundImagePropWidget is imagePropWidget with the value bound to an entity.
func boundImagePropWidget(entity string) map[string]any {
	w := imagePropWidget("imageObject", "")
	prop := w["Object"].(map[string]any)["Properties"].([]any)[0].(map[string]any)
	prop["Value"].(map[string]any)["EntityRef"] = map[string]any{
		"$Type":  "DomainModels$DirectEntityRef",
		"Entity": entity,
	}
	return w
}

func TestExtractImageObjectBinding_ReadsTheEntityRef(t *testing.T) {
	ctx, _ := newMockCtx(t)
	if !extractCustomWidgetPropertyImageBound(ctx, boundImagePropWidget("Gallery.Photo"), "imageObject") {
		t.Error("a bound imageObject read as unbound")
	}
}

// CONTROL: a collection entry is not a binding.
func TestExtractImageObjectBinding_CollectionEntryIsNotBound(t *testing.T) {
	ctx, _ := newMockCtx(t)
	if extractCustomWidgetPropertyImageBound(ctx, imagePropWidget("imageObject", "M.C.I"), "imageObject") {
		t.Error("a collection entry read as bound")
	}
}

func TestDescribeImageWidget_EmitsTheBinding(t *testing.T) {
	w := rawWidget{Name: "img"}
	w.ImageFromObject = true
	w.DefaultImage = "Atlas_Web_Content.Content.user"

	got := strings.Join(describeImageWidgetProps(nil, w), ", ")
	if !strings.Contains(got, "Image: $currentObject") {
		t.Errorf("describe did not emit the binding: %s", got)
	}
	if !strings.Contains(got, "DefaultImage: 'Atlas_Web_Content.Content.user'") {
		t.Errorf("describe did not emit the fallback image: %s", got)
	}
}
