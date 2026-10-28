// SPDX-License-Identifier: Apache-2.0

package microflows

// CanvasBox is an element's box in absolute canvas coordinates: a loop body's
// objects are drawn relative to the loop box's top-left corner, so a box of one
// of them has that corner added.
type CanvasBox struct{ Left, Top, Right, Bottom int }

// FacingSides returns the sides at which a line from a note to the element it
// documents attaches: the side of the note that faces the element, and the
// side of the element that faces the note (mendixlabs/mxcli#1598, case 2).
//
// Studio Pro's own drawings are the rule: of 523 distinct note lines in
// Studio Pro-authored apps, the element's side is top or bottom whenever the
// two boxes are vertically clear of each other, and left or right only when
// they overlap vertically — 98% agreement. So vertical separation wins over
// horizontal, and boxes that overlap fall back to the note-above drawing the
// writer has always used.
//
// One formula for every writer: the flow builder orients the lines of the
// notes it creates, and the splice orients a line it draws from a note
// already in the flow (ako/mxcli#1695).
func FacingSides(note, target CanvasBox) ConnectionSides {
	const top, right, bottom, left = 0, 1, 2, 3
	switch {
	case note.Bottom < target.Top:
		return ConnectionSides{Origin: bottom, Destination: top}
	case note.Top > target.Bottom:
		return ConnectionSides{Origin: top, Destination: bottom}
	case note.Right < target.Left:
		return ConnectionSides{Origin: right, Destination: left}
	case note.Left > target.Right:
		return ConnectionSides{Origin: left, Destination: right}
	}
	return ConnectionSides{Origin: bottom, Destination: top}
}
