// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// An image-typed pluggable property (the Image widget's `imageObject` and
// `defaultImageDynamic`) holds EITHER an image collection entry or the object
// in context, an entity specializing System.Image (mendixlabs/mxcli#1476).
//
//	image img (Image: 'Atlas_Web_Content.Content.user')   -- a collection entry
//	image img (Image: $currentObject)                     -- the uploaded image
//
// The unquoted variable cannot be mistaken for a three-part entry name, and it
// is the name MDL already gives the enclosing object everywhere else. Without
// it the only ways to show an uploaded image in a React app were a URL computed
// into an extra String attribute, or `dynamicimage`, which is CE0582 there.

// imageObjectVariable is the one variable an image property binds to: the
// enclosing data container's object. Any other is refused with this name.
const imageObjectVariable = "$currentObject"

// systemImageEntity is the entity the bound object must specialize. mxbuild
// refuses anything else with CE7247 "Widget expects entity of type System.Image."
const systemImageEntity = "System.Image"

// imageObjectReference returns the text of an image property written as a
// variable rather than a quoted entry name. The parser hands `$currentObject`
// over as a variable-led data source (its source text kept) or as an
// expression, depending on what follows it; both read the same here.
func imageObjectReference(w *ast.WidgetV3, key string, raw any) (string, bool) {
	switch v := raw.(type) {
	case *ast.WidgetExpressionV3:
		return strings.TrimSpace(v.Text), true
	case *ast.DataSourceV3:
		if src, ok := w.ExpressionSource(key); ok {
			return strings.TrimSpace(src), true
		}
		return strings.TrimSpace(formatDataSourceV3(v)), true
	}
	return "", false
}

// isImageObjectVariable reports whether ref is the spelling that binds the
// enclosing object.
func isImageObjectVariable(ref string) bool {
	return strings.EqualFold(ref, imageObjectVariable)
}

// resolveImageObject returns the entity an image property written as ref binds
// to: the enclosing data container's, which must specialize System.Image.
func (e *PluggableWidgetEngine) resolveImageObject(w *ast.WidgetV3, propertyKey, source, ref string) (string, error) {
	where := fmt.Sprintf("widget `%s` property `%s`", widgetLabelForError(w), source)
	if !isImageObjectVariable(ref) {
		return "", mdlerrors.NewValidationf(
			"%s: an image can be bound only to the object in context, written %s — not %s. "+
				"Place the image inside a data view over that object and write `%s: %s`",
			where, imageObjectVariable, ref, source, imageObjectVariable)
	}
	entity := e.bindingScopeFor(propertyKey).Entity
	if entity == "" {
		return "", mdlerrors.NewValidationf(
			"%s: %s binds the enclosing data view's object, and this widget is not inside one — "+
				"place it in a data view over an entity that specializes %s, or name a collection "+
				"entry: `%s: 'Module.Collection.ImageName'`",
			where, imageObjectVariable, systemImageEntity, source)
	}
	// Refused only when the entity is known and is not an image: one this
	// builder cannot see (created by a later statement, a library module) is
	// left for mxbuild to judge rather than refused on a guess.
	if e.pageBuilder.knowsEntity(entity) && !e.pageBuilder.isSpecializationOf(entity, systemImageEntity) {
		return "", mdlerrors.NewValidationf(
			"%s: %s is %s, which does not specialize %s — mxbuild rejects it with CE7247 "+
				"\"Widget expects entity of type System.Image.\" Bind the image inside a data view "+
				"over an image entity (`create persistent entity M.Photo extends System.Image (…)`)",
			where, imageObjectVariable, entity, systemImageEntity)
	}
	return entity, nil
}

// knowsEntity reports whether entityQN is in the domain models this builder
// has loaded.
func (pb *pageBuilder) knowsEntity(entityQN string) bool {
	module, name, ok := strings.Cut(entityQN, ".")
	if !ok {
		return false
	}
	_, err := pb.resolveEntity(ast.QualifiedName{Module: module, Name: name})
	return err == nil
}
