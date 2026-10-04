// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// E001 (string literal in an enumeration slot) is about the VALUE assigned to
// the attribute. A literal compared with a String variable, passed to a
// function, or used as an if-condition operand is not the value, so it must not
// be reported (ako/mxcli#969 item 1: `if $Dir = 'NW' then E.NW else E.N` was
// refused, blocking exec).
func enumSlotCtx() Context {
	return Context{
		SlotPath: "ChangeItem.Value:JTS.Log.Windrichting",
		Slots:    DefaultSlotResolver(),
		Catalog: lookupCatalog{
			kinds: map[string]TypeKind{"JTS.Log|Windrichting": KindEnumeration},
			enums: map[string]string{"JTS.Log|Windrichting": "JTS.WindrichtingEnum"},
			cases: map[string][]string{"JTS.WindrichtingEnum": {"N", "NW"}},
		},
	}
}

func countCode(hs []Hint, code string) int {
	n := 0
	for _, h := range hs {
		if h.Code == code {
			n++
		}
	}
	return n
}

func TestE001_NotInNonValuePositions(t *testing.T) {
	for _, src := range []string{
		`if $Dir = 'NW' then JTS.WindrichtingEnum.NW else JTS.WindrichtingEnum.N`,
		`if find('|NW|NORTHWEST|', '|' + $Dir + '|') >= 0 then JTS.WindrichtingEnum.NW else JTS.WindrichtingEnum.N`,
		`if contains($Dir, 'N') and $Dir != 'NW' then JTS.WindrichtingEnum.N else empty`,
	} {
		_, hs := NewParser().Parse(src, enumSlotCtx())
		if n := countCode(hs, "E001"); n != 0 {
			t.Errorf("%s: %d E001 for literals outside value position: %+v", src, n, hs)
		}
	}
}

func TestE001_StillFiresInValuePositions(t *testing.T) {
	for src, want := range map[string]int{
		`'NW'`:   1,
		`('NW')`: 1,
		`if $Dir = 'x' then 'NW' else JTS.WindrichtingEnum.N`: 1,
		`if $Dir = 'x' then 'NW' else 'N'`:                    2,
		`if $A then JTS.WindrichtingEnum.N else if $B then 'NW' else empty`: 1,
	} {
		_, hs := NewParser().Parse(src, enumSlotCtx())
		if n := countCode(hs, "E001"); n != want {
			t.Errorf("%s: %d E001, want %d: %+v", src, n, want, hs)
		}
	}
}

func TestE002_OnlyInValuePosition(t *testing.T) {
	ctx := Context{SlotPath: "IfStmt.Condition", Slots: DefaultSlotResolver()}
	if sc, ok := slotKind(ctx); !ok || sc.Kind != KindBoolean {
		t.Skip("IfStmt.Condition is not a Boolean slot in this table")
	}
	_, hs := NewParser().Parse(`$S = 'true'`, ctx)
	if hasCode(hs, "E002") {
		t.Errorf("'true' compared with a String is not a Boolean value: %+v", hs)
	}
	_, hs = NewParser().Parse(`'true'`, ctx)
	if !hasCode(hs, "E002") {
		t.Errorf("control: a whole-expression 'true' in a Boolean slot must be E002: %+v", hs)
	}
}
