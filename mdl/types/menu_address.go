// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strconv"
	"strings"
)

// MenuItemAddress is how ALTER NAVIGATION / ALTER MENU name a menu item. An
// item has no name in the model (Studio Pro stores none), so it is addressed
// by the caption describe prints for it, as a path written outer to inner:
//
//	'Users'                    the item captioned Users, anywhere in the tree
//	'Admin' > 'Users'          Users whose parent is captioned Admin
//	'Admin'@2 > 'Users'        Users inside the 2nd item captioned Admin
//	'Admin' > 'Users'@2        the 2nd Users among the sub-items of Admin
//
// A path matches as a suffix of an item's caption path, one direct parent/child
// step per `>`. @n belongs to the segment it follows. The rules are
// backend.ResolveMenuItemAddress's.
type MenuItemAddress struct {
	Segments []MenuItemAddressSegment
}

// MenuItemAddressSegment is one `'Caption'[@n]` of an address.
type MenuItemAddressSegment struct {
	// Caption is the caption as describe prints it, unquoted.
	Caption string
	// Ordinal is @n, 1-based; 0 when absent.
	Ordinal int
}

// String spells the address as MDL writes it, which is also how every message
// names a menu item, so a message can be copied into a statement.
func (a MenuItemAddress) String() string {
	parts := make([]string, len(a.Segments))
	for i, s := range a.Segments {
		parts[i] = QuoteMenuCaption(s.Caption)
		if s.Ordinal > 0 {
			parts[i] += "@" + strconv.Itoa(s.Ordinal)
		}
	}
	return strings.Join(parts, " > ")
}

// QuoteMenuCaption writes a caption as an MDL string literal, apostrophes
// doubled (mendixlabs/mxcli#1343).
func QuoteMenuCaption(caption string) string {
	return "'" + strings.ReplaceAll(caption, "'", "''") + "'"
}
