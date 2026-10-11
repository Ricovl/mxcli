// SPDX-License-Identifier: Apache-2.0

package pages

import "strings"

// A text box's and text area's submit behaviour, and a layout grid's width,
// shared by the CREATE PAGE builder, the check-time rule and the ALTER PAGE
// mutator so the three cannot accept different values (ako/mxcli-ledger
// FINDINGS #170). The members are the metamodel's: pages SubmitBehaviourType
// and ContainerWidth.

// Defaults Studio Pro stores on a new widget. describe prints a value only when
// it differs from these, so an ordinary widget describes as before.
const (
	DefaultSubmitBehaviour    = "OnEndEditing"
	DefaultSubmitOnInputDelay = 300
	DefaultLayoutGridWidth    = "FullWidth"
)

// CanonicalSubmitBehaviour returns the stored spelling of a SubmitBehaviour
// value (OnEndEditing, WhileEditing), matched case-insensitively.
func CanonicalSubmitBehaviour(s string) (string, bool) {
	return canonicalMember(s, "OnEndEditing", "WhileEditing")
}

// CanonicalLayoutGridWidth returns the stored spelling of a layout grid's Width
// (FullWidth, FixedWidth), matched case-insensitively.
func CanonicalLayoutGridWidth(s string) (string, bool) {
	return canonicalMember(s, "FullWidth", "FixedWidth")
}

// HasSubmitBehaviour reports whether a widget of the given storage type carries
// SubmitBehaviour and SubmitOnInputDelay.
func HasSubmitBehaviour(storageType string) bool {
	switch storageType {
	case "Forms$TextBox", "Pages$TextBox", "Forms$TextArea", "Pages$TextArea":
		return true
	}
	return false
}

func canonicalMember(s string, members ...string) (string, bool) {
	for _, m := range members {
		if strings.EqualFold(s, m) {
			return m, true
		}
	}
	return "", false
}
