// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"strconv"
	"strings"
)

// ColumnSelector is the explicit address of a DataGrid 2 column (R12,
// ako/mxcli#749):
//
//	dg column(Name)          the column whose `Attribute:` is Name
//	dg column(Owner/Name)    … an attribute over an association, as describe writes it
//	dg column('Total')       the column whose `Caption:` is 'Total'
//	dg column(Name)@2        the second of several such columns
//
// Mendix stores no name on a DataGrid 2 column, so a column is addressed by what
// describe prints in it: its attribute, or its caption. Two columns over the
// same attribute are both `column(Name)`, and an address that matches more than
// one is refused unless @n picks one — the same rule every generic ALTER target
// follows (PickAlterTargetMatch).
//
// It travels through the PageMutator interface as the columnRef string, spelled
// by String. That spelling cannot be mistaken for the derived column name the
// older `grid.Column` form passes: no derived name contains "(".
type ColumnSelector struct {
	// Attribute is the column's `Attribute:` value as describe writes it: the
	// short attribute name, or `Assoc/…/Attr` over associations.
	Attribute string
	// Caption is the column's `Caption:` text. Set instead of Attribute.
	Caption string
	// Ordinal is @n, 1-based; 0 when absent.
	Ordinal int
}

const columnSelectorPrefix = "column("

// String spells the selector as MDL writes it after the grid name.
func (s ColumnSelector) String() string {
	var b strings.Builder
	b.WriteString(columnSelectorPrefix)
	if s.Caption != "" {
		b.WriteString("'" + strings.ReplaceAll(s.Caption, "'", "''") + "'")
	} else {
		b.WriteString(s.Attribute)
	}
	b.WriteString(")")
	if s.Ordinal > 0 {
		b.WriteString("@" + strconv.Itoa(s.Ordinal))
	}
	return b.String()
}

// IsColumnSelector reports whether a columnRef is an explicit selector rather
// than a derived column name.
func IsColumnSelector(ref string) bool {
	_, ok := ParseColumnSelector(ref)
	return ok
}

// ParseColumnSelector reads a columnRef spelled by ColumnSelector.String. ok is
// false for anything else, including every derived column name.
func ParseColumnSelector(ref string) (ColumnSelector, bool) {
	rest, found := strings.CutPrefix(ref, columnSelectorPrefix)
	if !found {
		return ColumnSelector{}, false
	}
	var s ColumnSelector
	if strings.HasPrefix(rest, "'") {
		// A quoted caption: '' is an escaped quote.
		var b strings.Builder
		i := 1
		for {
			if i >= len(rest) {
				return ColumnSelector{}, false
			}
			if rest[i] == '\'' {
				if i+1 < len(rest) && rest[i+1] == '\'' {
					b.WriteByte('\'')
					i += 2
					continue
				}
				break
			}
			b.WriteByte(rest[i])
			i++
		}
		s.Caption = b.String()
		rest = rest[i+1:]
		if s.Caption == "" || !strings.HasPrefix(rest, ")") {
			return ColumnSelector{}, false
		}
		rest = rest[1:]
	} else {
		end := strings.IndexByte(rest, ')')
		if end <= 0 {
			return ColumnSelector{}, false
		}
		s.Attribute = rest[:end]
		rest = rest[end+1:]
	}
	if rest == "" {
		return s, true
	}
	n, found := strings.CutPrefix(rest, "@")
	if !found {
		return ColumnSelector{}, false
	}
	v, err := strconv.Atoi(n)
	if err != nil || v < 1 {
		return ColumnSelector{}, false
	}
	s.Ordinal = v
	return s, true
}

// joinColumnAddress spells a grid and its member: `grid column(…)` for an
// explicit selector, `grid.Member` for a derived name or a region.
func joinColumnAddress(widget, member string) string {
	if IsColumnSelector(member) {
		return widget + " " + member
	}
	return widget + "." + member
}
