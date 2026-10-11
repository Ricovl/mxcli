// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// attributeTypeSignature is what a view column over the attribute depends on:
// the kind, a String's length, an enumeration's reference and a DateTime's
// LocalizeDate. Two attributes with the same signature give a view column the
// same type; a change in it is what can put a view out of sync (CE6770).
func attributeTypeSignature(t domainmodel.AttributeType) string {
	switch v := t.(type) {
	case nil:
		return ""
	case *domainmodel.StringAttributeType:
		return fmt.Sprintf("String(%d)", v.Length)
	case *domainmodel.DateTimeAttributeType:
		return fmt.Sprintf("DateTime(localize=%v)", v.LocalizeDate)
	case *domainmodel.EnumerationAttributeType:
		return "Enumeration(" + v.EnumerationRef + ")"
	default:
		return t.GetTypeName()
	}
}

// attributeSignatures maps each attribute name to its type signature.
func attributeSignatures(e *domainmodel.Entity) map[string]string {
	if e == nil {
		return nil
	}
	out := make(map[string]string, len(e.Attributes))
	for _, a := range e.Attributes {
		if a != nil {
			out[a.Name] = attributeTypeSignature(a.Type)
		}
	}
	return out
}

// changedAttributes names the attributes of after whose type signature differs
// from the one recorded in before. An attribute new to the entity is not a
// change: no view can read it yet.
func changedAttributes(before map[string]string, after *domainmodel.Entity) map[string]bool {
	changed := map[string]bool{}
	for _, a := range after.Attributes {
		if a == nil {
			continue
		}
		if old, ok := before[a.Name]; ok && old != attributeTypeSignature(a.Type) {
			changed[a.Name] = true
		}
	}
	return changed
}

// resyncViewsOverSource brings the view entities that read a changed source
// attribute back in line with it, after the source entity has been written.
//
// A view's DateTime LocalizeDate is derived from the source attribute only
// when the view statement runs (viewDateTimeLocalize, #1297), so flipping the
// source's Localize afterwards — `alter entity … modify attribute … DateTime
// not localized`, or the same through `create or modify entity` — left every
// view over it out of sync: check and exec were silent and mx check reported
// CE6770 "View Entity is out of sync with the OQL Query" once per view
// (ako/mxcli-ledger FINDINGS #169). Mendix requires the view attribute to
// match the query whatever the view statement said, so the flag is re-derived
// here and a view is written only when a stored value differs — a repeat run
// writes nothing.
//
// Every other column property (String length, kind, enumeration) is declared
// in the view text and cannot be re-derived without rewriting what the user
// wrote, so a source change that breaks one is reported as a warning naming
// the view and column; the view has to be re-declared.
func resyncViewsOverSource(ctx *ExecContext, sourceQN string, changed map[string]bool) {
	if len(changed) == 0 {
		return
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return
	}
	for _, dm := range dms {
		for _, view := range dm.Entities {
			if !isViewEntity(view) || view.OqlQuery == "" {
				continue
			}
			readCols := viewColumnsReading(view, sourceQN, changed)
			if len(readCols) == 0 {
				continue
			}
			viewQN := moduleNameOfDomainModel(ctx, dm) + "." + view.Name
			flipped := resyncViewLocalizeDate(ctx, dm.ID, viewQN, view, readCols)
			warnStaleViewColumns(ctx, viewQN, view, readCols)
			// A view over this view reads the columns just changed. Views
			// cannot form a cycle, and a column is only reported when its
			// stored value moved, so this terminates.
			resyncViewsOverSource(ctx, viewQN, flipped)
		}
	}
}

// moduleNameOfDomainModel resolves a domain model's module name the way
// findEntity does — from its container, which also covers System.
func moduleNameOfDomainModel(ctx *ExecContext, dm *domainmodel.DomainModel) string {
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return ""
	}
	return h.GetModuleName(dm.ContainerID)
}

// viewAttributesAsAST describes a stored view entity's attributes the way a
// view statement would declare them.
func viewAttributesAsAST(view *domainmodel.Entity) []ast.ViewAttribute {
	attrs := make([]ast.ViewAttribute, 0, len(view.Attributes))
	for _, a := range view.Attributes {
		attrs = append(attrs, ast.ViewAttribute{Name: a.Name, Type: convertDomainModelTypeToAST(a.Type)})
	}
	return attrs
}

// viewColumnsReading returns, by column position, the view attributes whose
// select expression references one of the changed attributes of sourceQN —
// through any alias the FROM clause binds to it.
func viewColumnsReading(view *domainmodel.Entity, sourceQN string, changed map[string]bool) map[int]bool {
	oql := stripOQLComments(view.OqlQuery)
	clause := extractSelectClause(oql)
	if clause == "" {
		return nil
	}
	aliasMap := extractAliasMap(oql)
	reads := map[int]bool{}
	for i, expr := range attributeSelectColumns(oql, parseSelectColumns(clause)) {
		if i >= len(view.Attributes) {
			break
		}
		if m := oqlAliasSuffixRe.FindStringSubmatch(expr); m != nil {
			expr = strings.TrimSuffix(expr, m[0])
		}
		for _, ref := range oqlSourceAttrRefRe.FindAllStringSubmatch(oqlStringLiteralRe.ReplaceAllString(expr, "''"), -1) {
			if entity, ok := aliasMap[unquoteOQLIdent(ref[1])]; ok && entity == sourceQN && changed[unquoteOQLIdent(ref[2])] {
				reads[i] = true
				break
			}
		}
	}
	return reads
}

// resyncViewLocalizeDate re-derives the LocalizeDate of the view's DateTime
// columns that read a changed attribute, and writes the view only when one
// differs from what is stored. It returns the columns it changed.
func resyncViewLocalizeDate(ctx *ExecContext, dmID model.ID, viewQN string, view *domainmodel.Entity, readCols map[int]bool) map[string]bool {
	derived := viewDateTimeLocalize(ctx, view.OqlQuery, viewAttributesAsAST(view))
	var flipped []string
	changed := map[string]bool{}
	for i, a := range view.Attributes {
		if !readCols[i] {
			continue
		}
		dt, ok := a.Type.(*domainmodel.DateTimeAttributeType)
		if !ok {
			continue
		}
		want, ok := derived[a.Name]
		if !ok || dt.LocalizeDate == want {
			continue
		}
		dt.LocalizeDate = want
		state := "localized"
		if !want {
			state = "not localized"
		}
		flipped = append(flipped, a.Name+": "+state)
		changed[a.Name] = true
	}
	if len(flipped) == 0 {
		return nil
	}
	if err := ctx.Backend.UpdateEntity(dmID, view); err != nil {
		fmt.Fprintf(ctx.Output, "⚠ could not resync view %s: %v\n", viewQN, err)
		return nil
	}
	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)
	ctx.ReportMutation("Resynced", "view %s (%s)", viewQN, strings.Join(flipped, ", "))
	return changed
}

// warnStaleViewColumns reports the columns over a changed attribute whose
// declared type the source no longer gives them. They cannot be repaired
// without rewriting the view text, so the view must be re-declared.
func warnStaleViewColumns(ctx *ExecContext, viewQN string, view *domainmodel.Entity, readCols map[int]bool) {
	attrs := viewAttributesAsAST(view)
	columns, _ := inferOQLTypes(ctx, view.OqlQuery, attrs)
	var stale []string
	idx := make([]int, 0, len(readCols))
	for i := range readCols {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		if i >= len(columns) || i >= len(attrs) {
			continue
		}
		col, attr := columns[i], attrs[i]
		if col.InferredType.Kind == ast.TypeUnknown {
			continue
		}
		mismatch := passthroughStringLengthMismatch(attr.Type, col.InferredType, col.SourceAttr) ||
			!typesCompatible(attr.Type, col.InferredType) ||
			enumerationRefMismatch(attr.Type, col.InferredType)
		if mismatch {
			stale = append(stale, fmt.Sprintf("%s declared %s, the query now returns %s",
				attr.Name, formatDataTypeForError(attr.Type), formatDataTypeForError(viewColumnType(col.InferredType))))
		}
	}
	if len(stale) == 0 {
		return
	}
	fmt.Fprintf(ctx.Output,
		"⚠ view entity %s is out of sync with its source: %s — mxbuild reports CE6770 "+
			"\"View Entity is out of sync with the OQL Query\". Re-declare the view "+
			"(create or modify view entity %s …) with the new type.\n",
		viewQN, strings.Join(stale, "; "), viewQN)
}

// enumerationRefMismatch reports an enumeration column declared over a
// different enumeration than the one the query returns.
func enumerationRefMismatch(declared, inferred ast.DataType) bool {
	if declared.Kind != ast.TypeEnumeration || inferred.Kind != ast.TypeEnumeration {
		return false
	}
	if declared.EnumRef == nil || inferred.EnumRef == nil {
		return false
	}
	return declared.EnumRef.String() != inferred.EnumRef.String()
}
