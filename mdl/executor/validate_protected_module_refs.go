// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateProtectedModuleRefs reports the two rules a Protected (add-on) module
// imposes, which `check -p --references` did not know and mxbuild enforces
// (mendixlabs/mxcli#1498). A module is Protected when its Projects$ModuleSettings
// says ExportLevel "Protected"; each element in it is then Hidden or Usable
// (stored "API"), and:
//
//   - CE1406: nothing outside the module may reference a Hidden element in it.
//     mxbuild words the source "project level documents" for navigation and
//     "app modules" for a document in an ordinary module.
//   - CE5400: a Usable microflow in the module may not expose a Hidden entity of
//     it through a parameter or its return type.
//
// Measured on blank 11.14.0 and 11.13.0 apps, a control beside each refusal:
// a home microflow, menu item, call microflow and parameter type are refused
// into Hidden and accepted into API, and the module set back to Source accepts
// every one. A PAGE is refused even with ExportLevel "API" stored on it — from
// navigation and from a show page activity alike — so a page in an add-on
// module is reported whatever it stores; an API microflow inside the module
// that shows it is the measured way in.
//
// Covered shapes: navigation home pages, home flows, login / not-found pages and
// menu items; menu documents; microflow and nanoflow parameter and return
// types, call microflow and show page in their bodies. Everything else is left
// alone, and so is anything whose level cannot be established — settings that
// do not read, a target that does not resolve (the not-found checks own that),
// a nanoflow target (mxcli does not read a nanoflow's export level), or a
// reference between two Protected modules (not measured).
func validateProtectedModuleRefs(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() || !hasProtectedModuleCandidates(prog) {
		return nil
	}
	protected := protectedModuleNames(ctx)
	if len(protected) == 0 {
		return nil
	}
	r := &protectedRefResolver{
		ctx:          ctx,
		protected:    protected,
		scriptFlows:  map[string]*string{},
		scriptEntity: map[string]bool{},
		scriptPages:  map[string]bool{},
		entityLevels: map[string]map[string]string{},
	}
	for _, stmt := range prog.Statements {
		r.recordDefinition(stmt)
	}

	var errs []error
	for i, stmt := range prog.Statements {
		for _, p := range r.check(stmt) {
			errs = append(errs, mdlerrors.NewValidationf("statement %d: %s", i+1, p))
		}
	}
	return errs
}

func hasProtectedModuleCandidates(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch stmt.(type) {
		case *ast.AlterNavigationStmt, *ast.CreateMenuStmt, *ast.CreateMicroflowStmt, *ast.CreateNanoflowStmt:
			return true
		}
	}
	return false
}

// protectedModuleNames is the set of modules whose settings say Protected.
// Unreadable settings give the empty set, which turns the check off.
func protectedModuleNames(ctx *ExecContext) map[string]bool {
	byID := moduleProtections(ctx)
	if len(byID) == 0 {
		return nil
	}
	mods, err := getModulesFromCache(ctx)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, m := range mods {
		if byID[string(m.ID)] != "" {
			out[m.Name] = true
		}
	}
	return out
}

// moduleProtection is a module's ProtectedModuleType ("AddOn", "Solution")
// when its settings say Protected, else "". Unreadable settings give "".
func moduleProtection(ctx *ExecContext, moduleID string) string {
	return moduleProtections(ctx)[moduleID]
}

// moduleProtections maps each Protected module's ID to its
// ProtectedModuleType, from one read of the settings.
func moduleProtections(ctx *ExecContext) map[string]string {
	settings, err := ctx.Backend.ListModuleSettings()
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, ms := range settings {
		if ms != nil && ms.ExportLevel == "Protected" {
			kind := ms.ProtectedModuleType
			if kind == "" {
				kind = "AddOn" // the metamodel default
			}
			out[string(ms.ContainerID)] = kind
		}
	}
	return out
}

type protectedRefResolver struct {
	ctx       *ExecContext
	protected map[string]bool

	// What the script defines: a flow's stated export level (nil = not stated),
	// the entities and pages it creates.
	scriptFlows  map[string]*string
	scriptEntity map[string]bool
	scriptPages  map[string]bool

	storedPages  map[string]bool
	entityLevels map[string]map[string]string // module -> entity -> stored level; nil = unreadable
}

func (r *protectedRefResolver) recordDefinition(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.CreateMicroflowStmt:
		r.scriptFlows[s.Name.String()] = s.ExportLevel
	case *ast.CreateEntityStmt:
		r.scriptEntity[s.Name.String()] = true
	case *ast.CreatePageStmtV3:
		r.scriptPages[s.Name.String()] = true
	}
}

func (r *protectedRefResolver) check(stmt ast.Statement) []string {
	switch s := stmt.(type) {
	case *ast.AlterNavigationStmt:
		return r.checkNavigation(s)
	case *ast.CreateMenuStmt:
		if r.protected[s.Name.Module] {
			return nil
		}
		return r.checkMenuItems(s.Items, "app modules")
	case *ast.CreateMicroflowStmt:
		return r.checkFlow(s.Name, s.Parameters, s.ReturnType, s.Body, effectiveFlowLevel(r, s))
	case *ast.CreateNanoflowStmt:
		// A nanoflow's own export level is not read, so only its references out
		// of an ordinary module are checked, never CE5400 on it.
		return r.checkFlow(s.Name, s.Parameters, s.ReturnType, s.Body, "")
	}
	return nil
}

func (r *protectedRefResolver) checkNavigation(s *ast.AlterNavigationStmt) []string {
	const src = "project level documents"
	who := fmt.Sprintf("navigation profile '%s'", s.ProfileName)
	var out []string
	for _, hp := range s.HomePages {
		switch {
		case hp.IsPage:
			out = appendNonEmpty(out, r.pageRef(who, hp.Target, src))
		case !hp.IsNanoflow:
			out = appendNonEmpty(out, r.microflowRef(who, hp.Target, src))
		}
	}
	for _, qn := range []*ast.QualifiedName{s.LoginPage, s.NotFoundPage} {
		if qn != nil {
			out = appendNonEmpty(out, r.pageRef(who, *qn, src))
		}
	}
	return append(out, r.checkMenuItems(s.MenuItems, src)...)
}

func (r *protectedRefResolver) checkMenuItems(items []ast.NavMenuItemDef, src string) []string {
	var out []string
	for _, it := range items {
		who := fmt.Sprintf("menu item '%s'", it.Caption)
		var pages, flows []string
		if it.Page != nil {
			pages = append(pages, it.Page.String())
		}
		if it.Microflow != nil {
			flows = append(flows, it.Microflow.String())
		}
		for a := it.Action; a != nil; a = a.ThenAction {
			switch a.Type {
			case "showPage":
				pages = append(pages, a.Target)
			case "microflow":
				flows = append(flows, a.Target)
			}
		}
		for _, p := range uniqueStrings(pages) {
			out = appendNonEmpty(out, r.pageRef(who, parseQN(p), src))
		}
		for _, f := range uniqueStrings(flows) {
			out = appendNonEmpty(out, r.microflowRef(who, parseQN(f), src))
		}
		out = append(out, r.checkMenuItems(it.Items, src)...)
	}
	return out
}

// checkFlow checks a microflow or nanoflow. From an ordinary module: its
// parameter and return types, and the calls and pages in its body (CE1406).
// In a Protected module and Usable (level "API"): the Hidden entities of that
// module its signature exposes (CE5400).
func (r *protectedRefResolver) checkFlow(name ast.QualifiedName, params []ast.MicroflowParam,
	ret *ast.MicroflowReturnType, body []ast.MicroflowStatement, level string) []string {
	var out []string
	if r.protected[name.Module] {
		if level != types.ExportLevelAPI {
			return nil
		}
		for _, p := range params {
			if e, ok := signatureEntity(p.Type); ok && e.Module == name.Module && r.entityHidden(e) {
				out = append(out, ce5400(e, fmt.Sprintf("parameter '%s'", p.Name)))
			}
		}
		if ret != nil {
			if e, ok := signatureEntity(ret.Type); ok && e.Module == name.Module && r.entityHidden(e) {
				out = append(out, ce5400(e, fmt.Sprintf("microflow '%s'", name.String())))
			}
		}
		return out
	}

	const src = "app modules"
	for _, p := range params {
		if e, ok := signatureEntity(p.Type); ok {
			out = appendNonEmpty(out, r.entityRef(fmt.Sprintf("parameter '%s'", p.Name), e, src))
		}
	}
	if ret != nil {
		if e, ok := signatureEntity(ret.Type); ok {
			out = appendNonEmpty(out, r.entityRef(fmt.Sprintf("the return type of %s", name.String()), e, src))
		}
	}
	refs := &flowRefCollector{}
	refs.collectFromStatements(body)
	for _, p := range uniqueStrings(refs.pages) {
		out = appendNonEmpty(out, r.pageRef("show page activity", parseQN(p), src))
	}
	for _, f := range uniqueStrings(refs.microflows) {
		out = appendNonEmpty(out, r.microflowRef("call microflow activity", parseQN(f), src))
	}
	return out
}

func effectiveFlowLevel(r *protectedRefResolver, s *ast.CreateMicroflowStmt) string {
	if s.ExportLevel != nil {
		return *s.ExportLevel
	}
	// Not stated: a rewrite keeps what is stored, a new microflow is Hidden.
	if lvl, ok := r.storedMicroflowLevel(s.Name); ok {
		return lvl
	}
	return types.ExportLevelHidden
}

const ce1406Tail = "cannot reference hidden elements in add-on modules"

func (r *protectedRefResolver) pageRef(who string, qn ast.QualifiedName, src string) string {
	if !r.protected[qn.Module] || !r.pageExists(qn) {
		return ""
	}
	return fmt.Sprintf("CE1406: %s is not allowed to use page '%s' because %s %s — %s is a protected module, "+
		"and a page in one cannot be made usable (mxbuild refuses it even with export level API stored); "+
		"reference a microflow in %s with `export level api` that shows the page instead",
		who, qn.String(), src, ce1406Tail, qn.Module, qn.Module)
}

func (r *protectedRefResolver) microflowRef(who string, qn ast.QualifiedName, src string) string {
	if !r.protected[qn.Module] {
		return ""
	}
	lvl, ok := r.flowLevel(qn)
	if !ok || lvl == types.ExportLevelAPI {
		return ""
	}
	return fmt.Sprintf("CE1406: %s is not allowed to use microflow '%s' because %s %s — %s is a protected module "+
		"and the microflow's export level is Hidden; give it `export level api` to make it usable",
		who, qn.String(), src, ce1406Tail, qn.Module)
}

func (r *protectedRefResolver) entityRef(who string, qn ast.QualifiedName, src string) string {
	if !r.protected[qn.Module] || !r.entityHidden(qn) {
		return ""
	}
	return fmt.Sprintf("CE1406: %s is not allowed to use entity '%s' because %s %s — %s is a protected module "+
		"and the entity's export level is Hidden (mxcli cannot change an entity's export level; set it to Usable in Studio Pro)",
		who, qn.String(), src, ce1406Tail, qn.Module)
}

func ce5400(entity ast.QualifiedName, exposer string) string {
	return fmt.Sprintf("CE5400: the export level of entity '%s' should be 'Usable' because %s exposes it and has export level 'Usable' "+
		"(%s is a protected module and the entity is Hidden; mxcli cannot change an entity's export level — set it to Usable in Studio Pro, "+
		"or drop `export level api` from the microflow)",
		entity.String(), exposer, entity.Module)
}

// flowLevel is a microflow's effective export level: what the script states,
// else what is stored, else Hidden for one the script creates. ok is false when
// the microflow resolves nowhere.
func (r *protectedRefResolver) flowLevel(qn ast.QualifiedName) (string, bool) {
	if lvl, defined := r.scriptFlows[qn.String()]; defined {
		if lvl != nil {
			return *lvl, true
		}
		if stored, ok := r.storedMicroflowLevel(qn); ok {
			return stored, true
		}
		return types.ExportLevelHidden, true
	}
	return r.storedMicroflowLevel(qn)
}

func (r *protectedRefResolver) storedMicroflowLevel(qn ast.QualifiedName) (string, bool) {
	h, err := getHierarchy(r.ctx)
	if err != nil {
		return "", false
	}
	mfs, err := microflowsNamed(r.ctx, qn.Name)
	if err != nil {
		return "", false
	}
	for _, mf := range mfs {
		if h.GetModuleName(h.FindModuleID(mf.ContainerID)) == qn.Module {
			// "" is written as Hidden (microflowToGen) and read so by mxbuild.
			return mf.ExportLevel, true
		}
	}
	return "", false
}

// entityHidden reports whether an entity is known to be Hidden: a stored one
// whose level is not API, or one the script creates (always written Hidden,
// a rewrite carrying the stored level).
func (r *protectedRefResolver) entityHidden(qn ast.QualifiedName) bool {
	if lvl, ok := r.storedEntityLevel(qn); ok {
		return lvl != types.ExportLevelAPI
	}
	return r.scriptEntity[qn.String()]
}

func (r *protectedRefResolver) storedEntityLevel(qn ast.QualifiedName) (string, bool) {
	levels, read := r.entityLevels[qn.Module]
	if !read {
		levels = nil
		if mod, err := findModule(r.ctx, qn.Module); err == nil {
			if dm, err := r.ctx.Backend.GetDomainModel(mod.ID); err == nil && dm != nil {
				levels = map[string]string{}
				for _, e := range dm.Entities {
					levels[e.Name] = e.ExportLevel
				}
			}
		}
		r.entityLevels[qn.Module] = levels
	}
	lvl, ok := levels[qn.Name]
	return lvl, ok
}

func (r *protectedRefResolver) pageExists(qn ast.QualifiedName) bool {
	if r.scriptPages[qn.String()] {
		return true
	}
	if r.storedPages == nil {
		r.storedPages = buildPageQualifiedNames(r.ctx)
	}
	return r.storedPages[qn.String()]
}

// signatureEntity is the entity a parameter or return type names, if any. A
// bare `Module.Name` parses as an enumeration unless written `enum …`; it is
// tried as an entity too, and resolves only if an entity has that name.
func signatureEntity(dt ast.DataType) (ast.QualifiedName, bool) {
	switch {
	case (dt.Kind == ast.TypeEntity || dt.Kind == ast.TypeListOf) && dt.EntityRef != nil:
		return *dt.EntityRef, true
	case dt.Kind == ast.TypeEnumeration && !dt.ExplicitEnum && dt.EnumRef != nil:
		return *dt.EnumRef, true
	}
	return ast.QualifiedName{}, false
}

func parseQN(s string) ast.QualifiedName {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return ast.QualifiedName{Module: s[:i], Name: s[i+1:]}
		}
	}
	return ast.QualifiedName{Name: s}
}

func appendNonEmpty(out []string, s string) []string {
	if s != "" {
		out = append(out, s)
	}
	return out
}
