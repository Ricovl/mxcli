// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/widgetobj"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	mversion "github.com/mendixlabs/mxcli/modelsdk/version"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// LoadWidgetTemplate loads a pluggable widget template from the modelsdk/widgets
// registry and returns the shared widgetobj.Builder, wired with the codec child
// serializer. The registry speaks bson v2; the builder + pages.CustomWidget speak
// v1, so the template is bridged here.
func (b *Backend) LoadWidgetTemplate(widgetID string, projectPath string) (backend.WidgetObjectBuilder, error) {
	typeBSON, objBSON, propIDs, objectTypeID, err := templateFullBSONIDs(widgetID, projectPath, b.ProjectVersion())
	if err != nil {
		return nil, err
	}
	if typeBSON == nil || objBSON == nil {
		return nil, nil
	}
	return widgetobj.New(widgetID, v2ToV1BSON(typeBSON), v2ToV1BSON(objBSON),
		propIDs, objectTypeID, codecChildSerializer{}), nil
}

// templateFullBSONIDs loads a pluggable widget's template (augmented from the
// project's .mpk) with its Type written in the shape the project's Mendix
// version declares — see widgetTypeForVersion. Every template-built widget goes
// through it: LoadWidgetTemplate and BuildFilterWidget alike.
func templateFullBSONIDs(widgetID, projectPath string, pv *types.ProjectVersion) (bson.D, bson.D, map[string]mwidgets.PropertyTypeIDEntry, string, error) {
	typeBSON, objBSON, propIDs, objectTypeID, _, err := mwidgets.GetTemplateFullBSON(widgetID, mmpr.GenerateID, projectPath)
	if err != nil || typeBSON == nil {
		return typeBSON, objBSON, propIDs, objectTypeID, err
	}
	return widgetTypeForVersion(typeBSON, pv), objBSON, propIDs, objectTypeID, nil
}

// widgetTypeForVersion drops from a pluggable widget Type (the
// CustomWidgets$CustomWidgetType schema and everything under it) every key the
// project's Mendix version does not declare yet, whatever it holds.
//
// The embedded templates are extracted from Mendix 11.6, so every
// CustomWidgets$WidgetValueType in them carries SetLabel (10.5), IsLinked and
// IsMetaData (10.14), DefaultType (10.15), ActionVariables (10.21) and
// AllowUpload (11.8). DefaultType is "None" on every property, so the property
// floor (canon.StripUndeclaredProperties) refused every page with a pluggable
// widget on a Mendix 9.24 project; IsLinked / IsMetaData / SetLabel are true on
// the combo box and the data grid filters and would be refused next.
//
// Dropping a value is right here and wrong at the floor: the Type is the widget
// package's schema, copied from a newer Studio Pro, not anything the statement
// said — Studio Pro 9.24 stores none of these keys (measured on a Studio
// Pro-authored 9.24.30 app), and `mx convert` drops them. The Object (the
// widget's values) is not touched; a value it holds that the version cannot
// store is still refused by the floor.
//
// The version data is the floor's own (canon.PropertyDeclared), so the two
// cannot disagree. An unreadable version keeps the template as it is, the
// newest shape, as the floor strips nothing then either.
//
// Not codec.Encoder.OmitKeys: the Type is decoded from raw bytes and its
// elements are clean, and the encoder passes a clean element's fields through
// verbatim without consulting OmitKeys.
func widgetTypeForVersion(t bson.D, pv *types.ProjectVersion) bson.D {
	if pv == nil || pv.MajorVersion == 0 {
		return t
	}
	v := mversion.Version{Major: pv.MajorVersion, Minor: pv.MinorVersion, Patch: pv.PatchVersion}
	return dropUndeclaredWidgetKeys(t, v)
}

func dropUndeclaredWidgetKeys(d bson.D, v mversion.Version) bson.D {
	typeName := ""
	for _, e := range d {
		if e.Key == "$Type" {
			typeName, _ = e.Value.(string)
		}
	}
	out := make(bson.D, 0, len(d))
	for _, e := range d {
		if strings.HasPrefix(typeName, "CustomWidgets$") && !canon.PropertyDeclared(typeName, e.Key, v) {
			continue
		}
		e.Value = dropUndeclaredWidgetValue(e.Value, v)
		out = append(out, e)
	}
	return out
}

func dropUndeclaredWidgetValue(val any, v mversion.Version) any {
	switch x := val.(type) {
	case bson.D:
		return dropUndeclaredWidgetKeys(x, v)
	case bson.A:
		out := make(bson.A, len(x))
		for i := range x {
			out[i] = dropUndeclaredWidgetValue(x[i], v)
		}
		return out
	}
	return val
}

// SerializeWidgetToOpaque converts a domain widget to its raw BSON form (for
// embedding as a pluggable widget's child content) via the codec converters.
func (b *Backend) SerializeWidgetToOpaque(w pages.Widget) any {
	return codecChildSerializer{}.SerializeWidget(w)
}

// SerializeDataSourceToOpaque converts a domain data source to raw BSON.
func (b *Backend) SerializeDataSourceToOpaque(ds pages.DataSource) any {
	return codecChildSerializer{}.SerializeCustomWidgetDataSource(ds)
}

// BuildCreateAttributeObject builds an attribute object for filter widgets (pure,
// shared with the legacy engine).
func (b *Backend) BuildCreateAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (any, error) {
	return widgetobj.CreateAttributeObject(attributePath, objectTypeID, propertyTypeID, valueTypeID)
}

// BuildFilterWidget builds a DataGrid2 filter widget (text / number / date /
// dropdown) as a pages.CustomWidget whose Type/Object come from the
// modelsdk/widgets registry. Mirrors the MPR backend's BuildFilterWidget; the rest
// of the CustomWidget envelope (Appearance, editability, …) is added when the
// widget is serialized via customWidgetToGen.
func (b *Backend) BuildFilterWidget(spec backend.FilterWidgetSpec, projectPath string) (pages.Widget, error) {
	typeBSON, objBSON, propertyTypeIDs, _, err := templateFullBSONIDs(spec.WidgetID, projectPath, b.ProjectVersion())
	if err != nil {
		return nil, fmt.Errorf("BuildFilterWidget: load template %s: %w", spec.WidgetID, err)
	}
	if typeBSON == nil || objBSON == nil {
		return nil, fmt.Errorf("BuildFilterWidget: no template for widget %s", spec.WidgetID)
	}
	objV1 := v2ToV1BSON(objBSON)
	// Null any TextTemplate the widget's editorConfig hides in the filter's default
	// configuration (#574) — e.g. a DropdownFilter's screen-reader captions when
	// `adjustable` is off — else the populated default trips CE0463.
	if len(spec.VisibilityRules) > 0 {
		objV1 = widgetobj.ApplyVisibilityRules(objV1, propertyTypeIDs, spec.VisibilityRules)
	}
	return &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: spec.FilterName,
		},
		Editable:  "Always",
		RawObject: objV1,
		RawType:   v2ToV1BSON(typeBSON),
	}, nil
}

// Child-serialization failures cannot be returned: widgetobj.ChildSerializer
// yields BSON only, and making it error-returning is the interface change the
// TODO in mdl/backend/widgetobj/builder.go defers. Until then a failure is
// recorded here and drained by the page/snippet write entry points, so an
// unsupported construct fails the statement instead of being logged and dropped
// while the write still reports success (ADR-0004: refuse, don't drop).
//
// Widget building is synchronous and engines run sequentially — the same
// reasoning that makes widgetobj's package-level serializer hook safe — but the
// mutex keeps this honest if that ever changes.
var (
	childSerializeMu   sync.Mutex
	childSerializeErrs []error
)

// recordChildSerializeErr remembers a failure for the next drain. It is also
// logged, since the log line is the only signal for callers that do not drain.
func recordChildSerializeErr(err error) {
	log.Printf("modelsdk: %v", err)
	childSerializeMu.Lock()
	defer childSerializeMu.Unlock()
	childSerializeErrs = append(childSerializeErrs, err)
}

// takeChildSerializeErr returns the accumulated child-serialization failures as
// one error and clears them. A nil return means the widget tree serialized whole.
func takeChildSerializeErr() error {
	childSerializeMu.Lock()
	defer childSerializeMu.Unlock()
	if len(childSerializeErrs) == 0 {
		return nil
	}
	err := errors.Join(childSerializeErrs...)
	childSerializeErrs = nil
	return err
}

// codecChildSerializer implements widgetobj.ChildSerializer by routing child
// content through the modelsdk codec converters, then bridging v2→v1 BSON.
type codecChildSerializer struct{}

func (codecChildSerializer) SerializeWidget(w pages.Widget) bson.D {
	el, err := widgetToGen(w)
	if err != nil {
		recordChildSerializeErr(fmt.Errorf("serialize child widget %T: %w", w, err))
		return nil
	}
	return genToV1BSON(el)
}

func (codecChildSerializer) SerializeClientAction(a pages.ClientAction) bson.D {
	el, err := clientActionToGen(a)
	if err != nil {
		recordChildSerializeErr(fmt.Errorf("serialize client action %T: %w", a, err))
		return nil
	}
	return genToV1BSON(el)
}

func (codecChildSerializer) SerializeCustomWidgetDataSource(ds pages.DataSource) bson.D {
	el, err := customWidgetDataSourceToGen(ds)
	if err != nil {
		recordChildSerializeErr(fmt.Errorf("serialize custom widget data source %T: %w", ds, err))
		return nil
	}
	if el == nil {
		return nil
	}
	return genToV1BSON(el)
}

// SerializeClientTemplate builds an empty Forms$ClientTemplate through the
// codec, for a pluggable text-template property ALTER has to fill.
func (codecChildSerializer) SerializeClientTemplate() bson.D {
	return genToV1BSON(clientTemplateToGen(nil))
}

// genToV1BSON encodes a gen element with the codec (bson v2 bytes) and decodes it
// back into a v1 bson.D for the shared (v1) builder.
func genToV1BSON(el element.Element) bson.D {
	out, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		recordChildSerializeErr(fmt.Errorf("encode child element %T: %w", el, err))
		return nil
	}
	var d bson.D
	if err := bson.Unmarshal([]byte(out), &d); err != nil {
		log.Printf("modelsdk: v1 decode child element: %v", err)
		return nil
	}
	return d
}

// v2ToV1BSON bridges a modelsdk/widgets template document (bson v2) to v1.
func v2ToV1BSON(d bson.D) bson.D {
	b, err := bson.Marshal(d)
	if err != nil {
		return nil
	}
	var out bson.D
	if err := bson.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}
