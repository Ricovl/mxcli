// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1476 — "No MDL form binds the Image widget to an image
// object; dynamicimage fails CE0582 in React apps". In a data view over
// `MyFirstModule.Photo extends System.Image`:
//
//	image Photo_View_Image ()                            -- "No image selected."
//	image Photo_View_Image (ImageType: dynamicImage)     -- checks clean, not a datasource value
//	dynamicimage Photo_View_Image (DataSource: …)        -- CE0582 in a React app
//
// The Image widget's `imageObject` takes the object in context as well as an
// image collection entry, and MDL could only write the second. The spelling is
// `Image: $currentObject` — unquoted, so it cannot be confused with a
// collection entry's three-part name — and it is stored as the WidgetValue's
// EntityRef (the shape mxbuild accepts; see builder_image_object_test.go).

// imageObjectEngine builds inside a data view over Gallery.Photo (Photo extends
// System.Image) or Gallery.Note (a plain entity), with the REAL Image template.
func imageObjectEngine(t *testing.T, contextEntity string) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-gallery")}, Name: "Gallery"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-gallery")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-photo")}, Name: "Photo",
						GeneralizationRef: "System.Image"},
					{BaseElement: model.BaseElement{ID: model.ID("e-note")}, Name: "Note",
						Attributes: []*domainmodel.Attribute{{Name: "Text"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		entityContext:    contextEntity,
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

func realImageDef(t *testing.T) *WidgetDefinition {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	def, ok := reg.Get("IMAGE")
	if !ok {
		t.Fatal("no IMAGE definition")
	}
	return def
}

// currentObjectImage is the widget as the parser hands it over: `$currentObject`
// is variable-led, so it arrives as a data source with its source text kept.
func currentObjectImage(extra map[string]any) *ast.WidgetV3 {
	w := &ast.WidgetV3{
		Name:        "Photo_View_Image",
		Type:        "image",
		Properties:  map[string]any{"Image": &ast.DataSourceV3{Type: "parameter", Reference: "$currentObject"}},
		ValueSource: map[string]string{"Image": "$currentObject"},
	}
	for k, v := range extra {
		w.Properties[k] = v
	}
	return w
}

func TestImageWidget_CurrentObjectBindsTheContextEntity(t *testing.T) {
	e := imageObjectEngine(t, "Gallery.Photo")
	widget, err := e.Build(realImageDef(t), currentObjectImage(nil))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, widget)
	if !strings.Contains(got, "DomainModels$DirectEntityRef") || !strings.Contains(got, "Gallery.Photo") {
		t.Errorf("imageObject is not bound to Gallery.Photo:\n%s", got)
	}
}

// The parser may also hand `$currentObject` over as an expression; both reach
// the same binding.
func TestImageWidget_CurrentObjectAsExpressionBindsTheContextEntity(t *testing.T) {
	e := imageObjectEngine(t, "Gallery.Photo")
	w := &ast.WidgetV3{Name: "img", Type: "image",
		Properties: map[string]any{"Image": &ast.WidgetExpressionV3{Text: "$currentObject"}}}
	widget, err := e.Build(realImageDef(t), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := renderedWidgetStrings(t, widget); !strings.Contains(got, "Gallery.Photo") {
		t.Errorf("imageObject is not bound to Gallery.Photo:\n%s", got)
	}
}

// CONTROL: the collection-entry form is unchanged and writes no EntityRef.
func TestImageWidget_CollectionEntryStillWritesNoEntityRef(t *testing.T) {
	e := imageObjectEngine(t, "Gallery.Photo")
	w := &ast.WidgetV3{Name: "img", Type: "image",
		Properties: map[string]any{"Image": "Atlas_Web_Content.Content.user"}}
	widget, err := e.Build(realImageDef(t), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, widget)
	if !strings.Contains(got, "Atlas_Web_Content.Content.user") {
		t.Errorf("collection entry not written:\n%s", got)
	}
	if strings.Contains(got, "DomainModels$DirectEntityRef") {
		t.Errorf("a collection-entry image wrote an EntityRef:\n%s", got)
	}
}

// `DefaultImage:` is the fallback shown when the bound object holds no image —
// the widget's `defaultImageDynamic`, which nothing could write before.
func TestImageWidget_DefaultImageIsWritten(t *testing.T) {
	e := imageObjectEngine(t, "Gallery.Photo")
	widget, err := e.Build(realImageDef(t), currentObjectImage(map[string]any{
		"DefaultImage": "Atlas_Web_Content.Content.user",
	}))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := renderedWidgetStrings(t, widget); !strings.Contains(got, "Atlas_Web_Content.Content.user") {
		t.Errorf("DefaultImage not written:\n%s", got)
	}
}

// Outside a data container there is no object to bind: refuse rather than
// write an EntityRef to nothing.
func TestImageWidget_CurrentObjectOutsideADataViewIsRefused(t *testing.T) {
	e := imageObjectEngine(t, "")
	_, err := e.Build(realImageDef(t), currentObjectImage(nil))
	if err == nil || !strings.Contains(err.Error(), "data view") {
		t.Fatalf("err = %v, want a refusal naming the missing data view", err)
	}
}

// mxbuild: CE7247 "Widget expects entity of type System.Image." Refused at
// exec with the same reason, before a build is needed to learn it.
func TestImageWidget_CurrentObjectOfANonImageEntityIsRefused(t *testing.T) {
	e := imageObjectEngine(t, "Gallery.Note")
	_, err := e.Build(realImageDef(t), currentObjectImage(nil))
	if err == nil || !strings.Contains(err.Error(), "System.Image") {
		t.Fatalf("err = %v, want a refusal naming System.Image", err)
	}
}

// Any other variable is refused with the spelling that works: the binding is
// to the enclosing object, which is what `$currentObject` names.
func TestImageWidget_OtherVariableIsRefused(t *testing.T) {
	e := imageObjectEngine(t, "Gallery.Photo")
	w := &ast.WidgetV3{Name: "img", Type: "image",
		Properties:  map[string]any{"Image": &ast.DataSourceV3{Type: "parameter", Reference: "$Photo"}},
		ValueSource: map[string]string{"Image": "$Photo"}}
	_, err := e.Build(realImageDef(t), w)
	if err == nil || !strings.Contains(err.Error(), "$currentObject") {
		t.Fatalf("err = %v, want a refusal pointing at $currentObject", err)
	}
}
