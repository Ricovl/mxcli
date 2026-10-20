// SPDX-License-Identifier: Apache-2.0

package bsonnav

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
)

// SetTextsTextTranslation writes text into a Texts$Text as the translation for
// model.AuthoringLanguage() — the project's default language, the one DESCRIBE
// shows — updating that entry in place, or appending one when the text has no
// translation in that language yet. Every other language's translation is kept.
//
// This is the one rule for every text an ALTER sets: ALTER PAGE's texts and
// a menu item's caption in ALTER NAVIGATION / ALTER MENU. Before it there were
// four: a page Title and a column caption overwrote EVERY translation with the
// same string (English text written into nl_NL); a button caption or content
// overwrote whichever translation was listed first, whatever its language; and
// a Caption/Label stored as a Texts$Text looked for a `Translations` key no
// Mendix document has, then tried to set a stray `Text` field that DSet could
// not add — reporting success with nothing written. The case that surfaced it:
// a project switched to de_DE, a tab caption with only en_US → mxbuild CE4899
// "Empty caption. [German, Germany]", and `set Caption` had to fill de_DE
// without discarding the en_US text.
//
// Returns false when doc carries no Items list (not a Texts$Text).
func SetTextsTextTranslation(doc bson.D, text string) bool {
	raw := ToBsonA(DGet(doc, "Items"))
	if raw == nil && !HasKey(doc, "Items") {
		return false
	}
	lang := model.AuthoringLanguage()
	for _, item := range DGetArrayElements(raw) {
		itemDoc, ok := item.(bson.D)
		if !ok || DGetString(itemDoc, "LanguageCode") != lang {
			continue
		}
		if DSet(itemDoc, "Text", text) {
			return true
		}
	}
	newItem := bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "LanguageCode", Value: lang},
		{Key: "Text", Value: text},
	}
	var newArr bson.A
	if len(raw) > 0 && IsListMarker(raw[0]) {
		newArr = append(newArr, raw...)
	} else {
		// Texts$Text.Items is a marker-3 list in every document Studio Pro writes.
		newArr = append(append(newArr, int32(3)), raw...)
	}
	newArr = append(newArr, newItem)
	return DSet(doc, "Items", newArr)
}

// HasKey reports whether doc has the key at all, null value included.
func HasKey(doc bson.D, key string) bool {
	for _, e := range doc {
		if e.Key == key {
			return true
		}
	}
	return false
}

// IsListMarker reports whether v is the int32 type marker at index 0 of a
// Mendix list.
func IsListMarker(v any) bool {
	switch v.(type) {
	case int32, int:
		return true
	}
	return false
}

// TextsTextTranslations returns a Texts$Text document's translations in
// document order, for model.SelectTranslation. nil for anything else.
func TextsTextTranslations(doc bson.D) []model.TranslationEntry {
	var out []model.TranslationEntry
	for _, item := range DGetArrayElements(DGet(doc, "Items")) {
		itemDoc, ok := item.(bson.D)
		if !ok || DGetString(itemDoc, "$Type") != "Texts$Translation" {
			continue
		}
		out = append(out, model.TranslationEntry{
			LanguageCode: DGetString(itemDoc, "LanguageCode"),
			Text:         DGetString(itemDoc, "Text"),
		})
	}
	return out
}
