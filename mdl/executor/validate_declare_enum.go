// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// declaredTypeKind is what a bare `Module.Name` declare type turned out to name.
//
// A bare qualified name parses as TypeEnumeration with EnumRef set — the parser
// cannot tell an entity from an enumeration — so MDL043 used to read every one
// as the object declare it refuses, and worded it for an object even when -p
// was given and the project said the name is an enumeration
// (mendixlabs/mxcli#1516). Measured on 11.15.0: with the enumeration in the
// project, `declare $X Tmp.SomeEnum = empty` is written as an enumeration
// variable (describe prints `declare $X enum Tmp.SomeEnum = empty`) and builds
// at 0 errors — so it is not the object declare at all.
type declaredTypeKind int

const (
	declKindUnknown declaredTypeKind = iota
	declKindEnumeration
	declKindEntity
	// declKindAmbiguous: an enumeration and an entity share the name.
	declKindAmbiguous
)

func (r *voidCodeActions) recordScriptKind(qn string, k declaredTypeKind) {
	if r.scriptKinds == nil {
		r.scriptKinds = map[string]declaredTypeKind{}
	}
	addDeclaredKind(r.scriptKinds, qn, k)
}

func addDeclaredKind(m map[string]declaredTypeKind, qn string, k declaredTypeKind) {
	key := strings.ToLower(qn)
	if prev, ok := m[key]; ok && prev != k {
		m[key] = declKindAmbiguous
		return
	}
	m[key] = k
}

// declaredTypeKindOf resolves a declare type's qualified name against the script
// and, when there is one, the project. declKindUnknown when neither knows it —
// the no-project tier always lands there.
func (r *voidCodeActions) declaredTypeKindOf(qn string) declaredTypeKind {
	if r == nil || qn == "" {
		return declKindUnknown
	}
	key := strings.ToLower(qn)
	script := r.scriptKinds[key]
	project := r.projectKindsMap()[key]
	switch {
	case script == declKindUnknown:
		return project
	case project == declKindUnknown || project == script:
		return script
	}
	return declKindAmbiguous
}

func (r *voidCodeActions) projectKindsMap() map[string]declaredTypeKind {
	if r.projectKinds != nil {
		return r.projectKinds
	}
	r.projectKinds = map[string]declaredTypeKind{}
	b := r.project()
	if b == nil {
		return r.projectKinds
	}
	h, err := NewContainerHierarchyFromBackend(b)
	if err != nil || h == nil {
		return r.projectKinds
	}
	if enums, err := b.ListEnumerations(); err == nil {
		for _, e := range enums {
			if e == nil || e.Excluded {
				continue
			}
			if mod := h.GetModuleName(h.FindModuleID(e.ContainerID)); mod != "" {
				addDeclaredKind(r.projectKinds, mod+"."+e.Name, declKindEnumeration)
			}
		}
	}
	if dms, err := b.ListDomainModels(); err == nil {
		for _, dm := range dms {
			mod := h.GetModuleName(h.FindModuleID(dm.ContainerID))
			if mod == "" {
				continue
			}
			for _, ent := range dm.Entities {
				addDeclaredKind(r.projectKinds, mod+"."+ent.Name, declKindEntity)
			}
		}
	}
	return r.projectKinds
}
