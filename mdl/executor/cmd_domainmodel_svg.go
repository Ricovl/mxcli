// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"html"
	"math"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// cmd_domainmodel_svg.go draws a module's domain model as a finished SVG, for
// clients with no layout engine of their own (the Claude Code companion mod has
// no DOM, so the ELK graph the VS Code extension lays out in a browser is no
// use to it). No layout is computed: entities are drawn where the model stores
// them, which is where Studio Pro shows them.

// Palette, per entity kind (classifyEntity), close to Studio Pro's.
var svgEntityHeader = map[string]string{
	"persistent":    "#2B5FD9",
	"nonpersistent": "#E07B2E",
	"external":      "#8A4FD6",
	"view":          "#2E9E6A",
}

const (
	svgPad        = 24.0
	svgHeaderH    = 26.0
	svgExtendsH   = 14.0
	svgLineH      = 18.0
	svgFont       = `font-family="ui-monospace,SFMono-Regular,Menlo,Consolas,monospace"`
	svgMaxNameLen = 40
)

type svgBox struct {
	x, y, w, h float64
}

func (b svgBox) center() (float64, float64) { return b.x + b.w/2, b.y + b.h/2 }

// edgePoint is where the line from the box's centre towards (tx, ty) leaves the box.
func (b svgBox) edgePoint(tx, ty float64) (float64, float64) {
	cx, cy := b.center()
	dx, dy := tx-cx, ty-cy
	if dx == 0 && dy == 0 {
		return cx, cy
	}
	scale := math.Inf(1)
	if dx != 0 {
		scale = math.Min(scale, (b.w/2)/math.Abs(dx))
	}
	if dy != 0 {
		scale = math.Min(scale, (b.h/2)/math.Abs(dy))
	}
	return cx + dx*scale, cy + dy*scale
}

// edgePointFrom is where the line from (sx, sy) — a point inside the box,
// shifted off its centre for a parallel association — towards (tx, ty) leaves
// the box.
func (b svgBox) edgePointFrom(sx, sy, tx, ty float64) (float64, float64) {
	dx, dy := tx-sx, ty-sy
	if dx == 0 && dy == 0 {
		return sx, sy
	}
	scale := math.Inf(1)
	if dx > 0 {
		scale = math.Min(scale, (b.x+b.w-sx)/dx)
	} else if dx < 0 {
		scale = math.Min(scale, (b.x-sx)/dx)
	}
	if dy > 0 {
		scale = math.Min(scale, (b.y+b.h-sy)/dy)
	} else if dy < 0 {
		scale = math.Min(scale, (b.y-sy)/dy)
	}
	if scale < 0 || math.IsInf(scale, 1) {
		return sx, sy
	}
	return sx + dx*scale, sy + dy*scale
}

func svgNum(f float64) string { return fmt.Sprintf("%.0f", f) }

func svgText(s string) string {
	if len(s) > svgMaxNameLen {
		s = s[:svgMaxNameLen-1] + "…"
	}
	return html.EscapeString(s)
}

// renderDomainModelSVG draws the entities, the associations between them and
// the generalizations of one module. An association or generalization whose
// other end is outside the module has no position to draw to: the association
// is left out, the generalization is written under the entity's name.
func renderDomainModelSVG(moduleName string, entities []*domainmodel.Entity, assocs []*domainmodel.Association) string {
	if len(entities) == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 60" width="320" height="60">` +
			`<text x="16" y="34" ` + svgFont + ` font-size="13" fill="#6B7684">` +
			svgText(moduleName) + ` has no entities</text></svg>`
	}

	boxes := map[model.ID]svgBox{}
	byName := map[string]model.ID{}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, e := range entities {
		size := buildELKEntity(e)
		h := size.Height
		if e.GeneralizationRef != "" {
			h += svgExtendsH
		}
		b := svgBox{x: float64(e.Location.X), y: float64(e.Location.Y), w: size.Width, h: h}
		boxes[e.ID] = b
		byName[moduleName+"."+e.Name] = e.ID
		minX, minY = math.Min(minX, b.x), math.Min(minY, b.y)
		maxX, maxY = math.Max(maxX, b.x+b.w), math.Max(maxY, b.y+b.h)
	}
	minX, minY, maxX, maxY = minX-svgPad, minY-svgPad, maxX+svgPad, maxY+svgPad
	w, h := maxX-minX, maxY-minY

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s" width="%s" height="%s" %s font-size="12">`,
		svgNum(minX), svgNum(minY), svgNum(w), svgNum(h), svgNum(w), svgNum(h), svgFont)
	fmt.Fprintf(&sb, `<rect x="%s" y="%s" width="%s" height="%s" rx="8" fill="#F7F8FA"/>`, svgNum(minX), svgNum(minY), svgNum(w), svgNum(h))

	// Associations between the same two entities are spread side by side, so
	// their lines and labels do not print over one another.
	pairKey := func(a *domainmodel.Association) string {
		if a.ParentID < a.ChildID {
			return string(a.ParentID) + "|" + string(a.ChildID)
		}
		return string(a.ChildID) + "|" + string(a.ParentID)
	}
	pairTotal := map[string]int{}
	for _, a := range assocs {
		pairTotal[pairKey(a)]++
	}
	pairSeen := map[string]int{}

	// Lines first, so the boxes sit on top of them.
	for _, a := range assocs {
		from, okFrom := boxes[a.ParentID] // ParentPointer is the FROM side: it owns the reference
		to, okTo := boxes[a.ChildID]
		if !okFrom || !okTo {
			continue
		}
		key := pairKey(a)
		offset := (float64(pairSeen[key]) - float64(pairTotal[key]-1)/2) * 16
		pairSeen[key]++
		tcx, tcy := to.center()
		fcx, fcy := from.center()
		// Shift both aim points perpendicular to the centre line by the offset.
		if l := math.Hypot(tcx-fcx, tcy-fcy); l > 0 && offset != 0 {
			px, py := -(tcy-fcy)/l*offset, (tcx-fcx)/l*offset
			tcx, tcy, fcx, fcy = tcx+px, tcy+py, fcx+px, fcy+py
		}
		x1, y1 := from.edgePointFrom(fcx, fcy, tcx, tcy)
		x2, y2 := to.edgePointFrom(tcx, tcy, fcx, fcy)
		fromMult, toMult := "*", "1"
		if a.Type == domainmodel.AssociationTypeReferenceSet {
			toMult = "*"
		}
		fmt.Fprintf(&sb, `<g class="association"><title>%s</title>`, svgText(a.Name))
		fmt.Fprintf(&sb, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#6B7684" stroke-width="1.5"/>`, svgNum(x1), svgNum(y1), svgNum(x2), svgNum(y2))
		writeEndLabel(&sb, x1, y1, x2, y2, fromMult)
		writeEndLabel(&sb, x2, y2, x1, y1, toMult)
		// Labels of parallel associations stack, one line each.
		fmt.Fprintf(&sb, `<text x="%s" y="%s" font-size="10" fill="#6B7684" text-anchor="middle">%s</text>`,
			svgNum((x1+x2)/2), svgNum((y1+y2)/2-4+offset/16*12), svgText(a.Name))
		sb.WriteString(`</g>`)
	}
	for _, e := range entities {
		parentID, ok := byName[e.GeneralizationRef]
		if !ok {
			continue
		}
		child, parent := boxes[e.ID], boxes[parentID]
		pcx, pcy := parent.center()
		ccx, ccy := child.center()
		x1, y1 := child.edgePoint(pcx, pcy)
		x2, y2 := parent.edgePoint(ccx, ccy)
		fmt.Fprintf(&sb, `<g class="generalization"><line x1="%s" y1="%s" x2="%s" y2="%s" stroke="#6B7684" stroke-width="1.5"/>`, svgNum(x1), svgNum(y1), svgNum(x2), svgNum(y2))
		writeArrowHead(&sb, x1, y1, x2, y2)
		sb.WriteString(`</g>`)
	}

	for _, e := range entities {
		b := boxes[e.ID]
		kind := classifyEntity(e)
		var tip strings.Builder
		tip.WriteString(e.Name)
		for _, at := range e.Attributes {
			fmt.Fprintf(&tip, "\n%s: %s", at.Name, at.Type.GetTypeName())
		}
		fmt.Fprintf(&sb, `<g class="entity %s" transform="translate(%s %s)"><title>%s</title>`, kind, svgNum(b.x), svgNum(b.y), html.EscapeString(tip.String()))
		headerH := svgHeaderH
		if e.GeneralizationRef != "" {
			headerH += svgExtendsH
		}
		fmt.Fprintf(&sb, `<rect width="%s" height="%s" rx="4" fill="#FFFFFF" stroke="#B8C0CC"/>`, svgNum(b.w), svgNum(b.h))
		fmt.Fprintf(&sb, `<path d="M4 0H%sA4 4 0 0 1 %s 4V%sH0V4A4 4 0 0 1 4 0Z" fill="%s"/>`,
			svgNum(b.w-4), svgNum(b.w), svgNum(headerH), svgEntityHeader[kind])
		fmt.Fprintf(&sb, `<text x="8" y="18" fill="#FFFFFF" font-weight="600">%s</text>`, svgText(e.Name))
		if e.GeneralizationRef != "" {
			fmt.Fprintf(&sb, `<text x="8" y="33" fill="#FFFFFF" font-size="10" font-style="italic">extends %s</text>`, svgText(e.GeneralizationRef))
		}
		for i, at := range e.Attributes {
			fmt.Fprintf(&sb, `<text x="8" y="%s" fill="#1F2933">%s</text>`,
				svgNum(headerH+svgLineH*float64(i)+13), svgText(at.Name+": "+at.Type.GetTypeName()))
		}
		sb.WriteString(`</g>`)
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// writeEndLabel puts a multiplicity just inside the line's end at (x, y), set
// off to one side so it does not sit on the line.
func writeEndLabel(sb *strings.Builder, x, y, ox, oy float64, label string) {
	dx, dy := ox-x, oy-y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l, dy/l
	lx, ly := x+ux*12-uy*8, y+uy*12+ux*8
	fmt.Fprintf(sb, `<text x="%s" y="%s" font-size="11" fill="#1F2933" text-anchor="middle" dominant-baseline="middle">%s</text>`, svgNum(lx), svgNum(ly), label)
}

// writeArrowHead draws a hollow triangle at (x2, y2), pointing along the line.
func writeArrowHead(sb *strings.Builder, x1, y1, x2, y2 float64) {
	dx, dy := x2-x1, y2-y1
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l, dy/l
	bx, by := x2-ux*12, y2-uy*12
	fmt.Fprintf(sb, `<path d="M%s %sL%s %sL%s %sZ" fill="#FFFFFF" stroke="#6B7684" stroke-width="1.5"/>`,
		svgNum(x2), svgNum(y2), svgNum(bx-uy*7), svgNum(by+ux*7), svgNum(bx+uy*7), svgNum(by-ux*7))
}

// domainModelSVG writes the SVG of a module's domain model.
func domainModelSVG(ctx *ExecContext, moduleName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return err
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}
	fmt.Fprint(ctx.Output, renderDomainModelSVG(moduleName, dm.Entities, dm.Associations))
	return nil
}

// DomainModelSVG is the exported Executor method, called from outside the package.
func (e *Executor) DomainModelSVG(moduleName string) error {
	return domainModelSVG(e.newExecContext(context.Background()), moduleName)
}
