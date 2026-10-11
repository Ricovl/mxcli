// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// PageEntityReadAccessRule (SEC010) reports a data widget whose entity the
// roles that may open its page cannot read.
//
// At Production security the runtime applies entity access to every retrieve
// a page makes, so a list over an entity a role has no read rule for comes out
// EMPTY for that role — and from a microflow data source, the right length with
// every field blank (manage-security skill, measured on 11.14.0). Nothing
// reports it: `check --references`, mxbuild and the runtime log are all clean
// (ChipCoV9: a combo box over System.User filtered by role, empty for every
// non-admin role).
//
// The question is asked per USER role, because that is what a person logs in
// as: a user role that holds one of the page's module roles opens the page,
// and reads the entity if ANY of its module roles has a read rule on it, on
// one of its generalizations (rules are inherited), or on a specialization
// (a list over System.User shows the Administration.Account objects the role
// may read — that is how an Administrator's user list fills).
//
// Severity follows what the role sees:
//   - no read rule at all            → warning, the widget renders empty
//   - only XPath-constrained rules   → info, it renders the rows the
//     constraint allows (often right: "my own requests")
//   - System.User, row-scoped        → warning, special message: System.User's
//     own rule is [id = '[%CurrentUser%]'], so a user picker lists exactly one
//     person and, filtered by role, often nobody.
//
// Only at Production. Off checks nothing, and Prototype is stored as
// CheckFormsAndMicroflows: the runtime enforces page and microflow access but
// not entity access, so the data is not withheld there (MPR007 makes the same
// level split, see homePageConsequence).
//
// System entities carry no access rules in the .mpr; their read access comes
// from meta.SystemReadAccess, which is measured from a deployed model.mdp.
type PageEntityReadAccessRule struct{}

// NewPageEntityReadAccessRule creates the SEC010 rule.
func NewPageEntityReadAccessRule() *PageEntityReadAccessRule { return &PageEntityReadAccessRule{} }

func (r *PageEntityReadAccessRule) ID() string                       { return "SEC010" }
func (r *PageEntityReadAccessRule) Name() string                     { return "PageEntityNotReadable" }
func (r *PageEntityReadAccessRule) Category() string                 { return "security" }
func (r *PageEntityReadAccessRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *PageEntityReadAccessRule) Description() string {
	return "Checks that the roles allowed on a page may read the entity each of its data widgets lists (at Production security it renders empty for them, silently)"
}

// RequiredCatalogMode: widgets and permissions are written by REFRESH CATALOG FULL.
func (r *PageEntityReadAccessRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

// readLevel is how much of an entity a set of module roles may read.
type readLevel int

const (
	readNone   readLevel = iota // no read rule
	readScoped                  // only XPath-constrained read rules
	readFull                    // at least one unconstrained read rule
)

// dataWidgetLabels names the widget types whose entity is a list data source,
// as Studio Pro calls them. A data view is left out: its object usually comes
// in through a page parameter, and mxbuild already reports the attributes in
// it a role cannot read (CE2729).
var dataWidgetLabels = map[string]string{
	"Forms$ListView":                  "List view",
	"Forms$DataGrid":                  "Data grid",
	"Forms$TemplateGrid":              "Template grid",
	"Forms$ReferenceSelector":         "Reference selector",
	"Forms$ReferenceSetSelector":      "Reference set selector",
	"Forms$InputReferenceSetSelector": "Input reference set selector",
}

// pluggableLabels names the common pluggable data widgets. Any other pluggable
// widget with an entity is reported as "Widget".
var pluggableLabels = map[string]string{
	"com.mendix.widget.web.datagrid.Datagrid": "Data grid",
	"com.mendix.widget.web.gallery.Gallery":   "Gallery",
	"com.mendix.widget.web.combobox.Combobox": "Combo box",
	"com.mendix.widget.web.treenode.TreeNode": "Tree node",
}

func dataWidgetLabel(widgetType string) (string, bool) {
	if l, ok := dataWidgetLabels[widgetType]; ok {
		return l, true
	}
	if l, ok := pluggableLabels[widgetType]; ok {
		return l, true
	}
	// A pluggable widget is catalogued under its widget ID, which has no '$'.
	if widgetType != "" && !strings.Contains(widgetType, "$") && strings.Contains(widgetType, ".") {
		return "Widget", true
	}
	return "", false
}

// entityAccessModel is the read rules and inheritance needed to answer
// "may these module roles read entity E".
type entityAccessModel struct {
	reads    map[string]map[string]readLevel // entity → module role → best level
	gen      map[string]string               // entity → generalization
	children map[string][]string             // entity → direct specializations
	known    map[string]bool                 // entities whose access is known
}

func (m *entityAccessModel) grant(entity, role string, lvl readLevel) {
	if m.reads[entity] == nil {
		m.reads[entity] = map[string]readLevel{}
	}
	if lvl > m.reads[entity][role] {
		m.reads[entity][role] = lvl
	}
}

func buildEntityAccessModel(ctx *linter.LintContext) *entityAccessModel {
	m := &entityAccessModel{
		reads:    map[string]map[string]readLevel{},
		gen:      map[string]string{},
		children: map[string][]string{},
		known:    map[string]bool{},
	}
	// Every entity, platform and marketplace modules included: a list over
	// System.User is filled by Administration.Account's rules, so an excluded
	// or marketplace module still decides the answer.
	if rows, err := ctx.Query(`SELECT QualifiedName, COALESCE(Generalization, '') FROM entities`); err == nil {
		for rows.Next() {
			var qn, gen string
			if rows.Scan(&qn, &gen) != nil {
				continue
			}
			if !strings.HasPrefix(qn, "System.") {
				m.known[qn] = true
			}
			if gen != "" {
				m.gen[qn] = gen
			}
		}
		rows.Close()
	}
	for _, e := range meta.SystemEntities {
		qn := "System." + e.Name
		if e.Generalization != "" {
			m.gen[qn] = e.Generalization
		}
		grants, ok := meta.SystemReadAccess(qn)
		if !ok {
			continue
		}
		m.known[qn] = true
		for _, g := range grants {
			lvl := readFull
			if g.RowScoped {
				lvl = readScoped
			}
			m.grant(qn, g.ModuleRole, lvl)
		}
	}
	for child, parent := range m.gen {
		m.children[parent] = append(m.children[parent], child)
	}
	for p := range ctx.Permissions() {
		if p.ElementType != "ENTITY" || p.AccessType != "READ" || p.MemberName != "" {
			continue
		}
		lvl := readFull
		if p.IsConstrained {
			lvl = readScoped
		}
		m.known[p.ElementName] = true
		m.grant(p.ElementName, p.ModuleRoleName, lvl)
	}
	return m
}

// level returns the best read level any of roles has on entity, counting
// rules on its generalizations and on its specializations.
func (m *entityAccessModel) level(entity string, roles []string) readLevel {
	best := readNone
	consider := func(e string) {
		for _, r := range roles {
			if l := m.reads[e][r]; l > best {
				best = l
			}
		}
	}
	seen := map[string]bool{}
	for e := entity; e != "" && !seen[e]; e = m.gen[e] {
		seen[e] = true
		consider(e)
	}
	var down func(e string)
	down = func(e string) {
		for _, c := range m.children[e] {
			if seen[c] {
				continue
			}
			seen[c] = true
			consider(c)
			down(c)
		}
	}
	down(entity)
	return best
}

// Check reports each page data widget whose entity some user role that may
// open the page cannot read.
func (r *PageEntityReadAccessRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}
	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil || ps.SecurityLevel != security.SecurityLevelProduction || len(ps.UserRoles) == 0 {
		return nil
	}

	pageRoles := map[string][]string{}
	for p := range ctx.Permissions() {
		if p.ElementType == "PAGE" && p.AccessType == "VIEW" {
			pageRoles[p.ElementName] = append(pageRoles[p.ElementName], p.ModuleRoleName)
		}
	}
	if len(pageRoles) == 0 {
		return nil
	}

	access := buildEntityAccessModel(ctx)

	var violations []linter.Violation
	for w := range ctx.Widgets() {
		if w.EntityRef == "" || !strings.EqualFold(w.ContainerType, "page") {
			continue
		}
		label, ok := dataWidgetLabel(w.WidgetType)
		if !ok || !access.known[w.EntityRef] {
			continue
		}
		roles := pageRoles[w.ContainerQualifiedName]
		if len(roles) == 0 {
			continue // no role may open it: MPR007 / CE0557 territory
		}
		var blind, scoped, openers []string
		for _, ur := range ps.UserRoles {
			if !sharesRole(ur.ModuleRoles, roles) {
				continue
			}
			openers = append(openers, ur.Name)
			switch access.level(w.EntityRef, ur.ModuleRoles) {
			case readNone:
				blind = append(blind, ur.Name)
			case readScoped:
				scoped = append(scoped, ur.Name)
			}
		}
		if v, ok := r.violation(w, label, roles, openers, blind, scoped); ok {
			violations = append(violations, v)
		}
	}
	return violations
}

func (r *PageEntityReadAccessRule) violation(w linter.Widget, label string, pageRoles, openers, blind, scoped []string) (linter.Violation, bool) {
	sort.Strings(pageRoles)
	widget := fmt.Sprintf("%s '%s' on page %s lists %s", label, w.Name, w.ContainerQualifiedName, w.EntityRef)
	v := linter.Violation{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Location: linter.Location{
			Module:       moduleFromQualified(w.ContainerQualifiedName),
			DocumentType: "page",
			DocumentName: docNameFromQualified(w.ContainerQualifiedName),
		},
	}
	switch {
	case len(blind) > 0:
		who := fmt.Sprintf("which none of the page's roles (%s) may read", strings.Join(pageRoles, ", "))
		if len(blind) < len(openers) {
			who = fmt.Sprintf("which user role%s %s can open the page but may not read",
				plural(blind), quoteList(blind))
		}
		v.Message = fmt.Sprintf("%s, %s — it renders empty for them (from a microflow data source: every field blank)", widget, who)
		v.Suggestion = fmt.Sprintf("GRANT <Module>.<Role> ON %s (READ *) for a module role those user roles hold, or bind the widget to an entity they may read", w.EntityRef)
		if strings.HasPrefix(w.EntityRef, "System.") {
			// A module cannot widen access to a System entity (manage-security skill).
			v.Suggestion = "A module cannot grant read on a System entity: open the page only to roles holding System.Administrator, or copy the data onto an entity your module owns"
		}
	case len(scoped) > 0 && w.EntityRef == "System.User":
		v.Message = fmt.Sprintf("%s, which lists only the current user for user role%s %s — denormalise the name onto an entity of your own (e.g. Administration.Account) or use a microflow data source that returns that entity",
			widget, plural(scoped), quoteList(scoped))
		v.Suggestion = "System.User's own access rule is [id = '[%CurrentUser%]'] for every role; a microflow returning System.User objects delivers the rows with every field blank, so return your own entity"
	case len(scoped) > 0:
		v.Severity = linter.SeverityInfo
		v.Message = fmt.Sprintf("%s, which user role%s %s may read only through an XPath-constrained rule — it renders just the rows the constraint allows",
			widget, plural(scoped), quoteList(scoped))
		v.Suggestion = "Intended if the list is meant to be row-scoped; otherwise grant an unconstrained READ to a module role those user roles hold"
	default:
		return linter.Violation{}, false
	}
	return v, true
}

func plural(xs []string) string {
	if len(xs) == 1 {
		return ""
	}
	return "s"
}

func quoteList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = "'" + x + "'"
	}
	return strings.Join(q, ", ")
}
