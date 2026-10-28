// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/xpathrefs"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// A user task's targeting constraint picks the users (or workflow groups) the
// task goes to, and nothing checked it: the expression-scope pass skips it as
// "not an expression", and the reference pass looked only at a targeting
// microflow's name. An agent's `[System.User/Name = 'MxAdmin']` therefore passed
// `check --references` and exec, and Studio Pro reported CE0161 on every task
// that carried it (ako/mxcli#1711).
//
// Measured with mx check on mxbuild 11.15.0, blank app:
//
//	users  [System.User/Name = 'MxAdmin']                     CE0161
//	users  [NoSuchAttr = 'x']                                 CE0161
//	groups [System.WorkflowGroup/Name = 'Reviewers']          CE0161
//	users  [Name = 'MxAdmin'], [Active = true()]              clean
//	users  [System.UserRoles = '[%UserRole_User%]']           clean (and selects the role's users at runtime, 11.13.0)
//	users  [System.UserRoles/System.UserRole/Name = 'User']   clean
//	groups [Name = 'Reviewers']                               clean
//
// Re-measuring has one trap: with a CE1613 anywhere in the app (an unknown
// `[%UserRole_X%]` token, a dangling call), mx check reported no CE0161 at all.
//
// The constraint is evaluated on the workflow user entity — System.User unless
// App Settings > Workflows names a specialization — or on System.WorkflowGroup
// for group targeting. The System module has no domain model in the project, so
// its members come from modelsdk/meta; a specialization's own members come from
// the project, and what the model cannot answer stays silent.

// validateWorkflowTargetingXPath checks the targeting constraint of every user
// task among activities, nested ones included.
func validateWorkflowTargetingXPath(ctx *ExecContext, activities []ast.WorkflowActivityNode, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() || len(activities) == 0 {
		return nil
	}
	var errs []string
	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		if n, ok := act.(*ast.WorkflowUserTaskNode); ok {
			errs = append(errs, targetingXPathProblems(ctx, sc, "user task "+workflowUserTaskLabel(n), n.Targeting.Kind, n.Targeting.XPath)...)
		}
	})
	return errs
}

// validateAlterWorkflowTargetingXPath is the ALTER WORKFLOW entry point: the
// tasks an operation adds, and `set (Targeting: xpath [ … ]) on X`.
func validateAlterWorkflowTargetingXPath(ctx *ExecContext, s *ast.AlterWorkflowStmt, added []ast.WorkflowActivityNode, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	errs := validateWorkflowTargetingXPath(ctx, added, sc)
	for _, op := range s.Operations {
		if o, ok := op.(*ast.SetActivityPropertyOp); ok && o.Property == "targeting_xpath" {
			errs = append(errs, targetingXPathProblems(ctx, sc, "activity '"+o.ActivityRef+"'", "xpath", o.Value)...)
		}
	}
	return errs
}

// targetingXPathProblems reports what in one targeting constraint mxbuild rejects.
func targetingXPathProblems(ctx *ExecContext, sc *scriptContext, label, kind, xpath string) []string {
	var base string
	switch kind {
	case "xpath":
		base = "System.User"
	case "group_xpath":
		base = "System.WorkflowGroup"
	default:
		return nil
	}
	if strings.TrimSpace(xpath) == "" {
		return nil
	}
	// The constraint as buildUserTask stores it: a name qualified with the
	// evaluated entity is the bare member.
	stored := resolveXPathMemberNames(xpath, base)
	on := base
	if kind == "xpath" {
		on = workflowUserEntity(ctx)
	}
	m, resolve := targetingXPathResolver(ctx, sc)

	var errs []string
	for _, bad := range unresolvableXPathStepsAndLeadingEntity(ctx, m, stored, on, resolve) {
		switch bad.kind {
		case "entity":
			errs = append(errs, fmt.Sprintf("%s targeting: %s", label, leadingEntityStepMessage(ctx, stored, bad.name, on)))
		case "attribute":
			errs = append(errs, fmt.Sprintf(
				"%s targeting: the constraint names %q, which is neither an attribute nor an association of %s — "+
					"mxbuild rejects the constraint (CE0161 \"Error(s) in XPath constraint\")",
				label, bad.name, on))
		default:
			errs = append(errs, fmt.Sprintf(
				"%s targeting: the constraint names the %s %q, which %s does not have — "+
					"mxbuild reports CE1613 \"The selected %s '%s' no longer exists\"",
				label, bad.kind, bad.name, on, bad.kind, bad.name))
		}
	}
	return errs
}

// workflowUserEntity is the entity a users-targeting constraint is evaluated on:
// App Settings > Workflows > User entity, System.User when none is set.
func workflowUserEntity(ctx *ExecContext) string {
	if ps, err := ctx.Backend.GetProjectSettings(); err == nil && ps != nil && ps.Workflows != nil && ps.Workflows.UserEntity != "" {
		return ps.Workflows.UserEntity
	}
	return "System.User"
}

// targetingXPathResolver is scriptXPathResolver with the System module added,
// which a targeting constraint always starts from.
func targetingXPathResolver(ctx *ExecContext, sc *scriptContext) (xpathrefs.Model, func(entityQN, member string) memberResolution) {
	inner, projectResolve := scriptXPathResolver(ctx, sc)
	resolve := func(entityQN, member string) memberResolution {
		if strings.HasPrefix(entityQN, "System.") {
			return systemMemberResolution(entityQN, member)
		}
		// A specialization's own members; at System.User the project walk has
		// no domain model to read and answers memberUnknown, never a guess.
		return projectResolve(entityQN, member)
	}
	return &systemXPathModel{inner: inner, ctx: ctx}, resolve
}

// systemXPathModel answers xpathrefs.Model for the System module from
// modelsdk/meta and defers everything else to the project-and-script model.
type systemXPathModel struct {
	inner xpathrefs.Model
	ctx   *ExecContext
}

func (m *systemXPathModel) IsEntity(qn string) bool {
	if _, ok := modelerSystemEntity(qn); ok {
		return true
	}
	return m.inner.IsEntity(qn)
}

func (m *systemXPathModel) AssociationTarget(qn, from string) (string, bool) {
	name, ok := strings.CutPrefix(qn, "System.")
	if !ok {
		return m.inner.AssociationTarget(qn, from)
	}
	for _, a := range meta.ModelerSystemAssociations() {
		if a.Name != name {
			continue
		}
		// Either end may be traversed, from the entity or anything it
		// specializes.
		for _, e := range m.chain(from) {
			switch e {
			case "System." + a.Parent:
				return "System." + a.Child, true
			case "System." + a.Child:
				return "System." + a.Parent, true
			}
		}
		return "", false
	}
	return m.inner.AssociationTarget(qn, from)
}

// chain is from and its generalizations, through the project's entities into
// the System module's.
func (m *systemXPathModel) chain(from string) []string {
	chain := []string{from}
	if !strings.HasPrefix(from, "System.") {
		if c, _ := generalizationChain(m.ctx, from); len(c) > 0 {
			chain = c
		}
	}
	last := chain[len(chain)-1]
	for seen := map[string]bool{}; !seen[last]; {
		seen[last] = true
		def, ok := modelerSystemEntity(last)
		if !ok || def.Generalization == "" {
			break
		}
		last = def.Generalization
		chain = append(chain, last)
	}
	return chain
}

// modelerSystemEntity is the System entity qn names, among those Studio Pro shows.
func modelerSystemEntity(qn string) (meta.SystemEntityDef, bool) {
	name, ok := strings.CutPrefix(qn, "System.")
	if !ok {
		return meta.SystemEntityDef{}, false
	}
	for _, e := range meta.ModelerSystemEntities() {
		if e.Name == name {
			return e, true
		}
	}
	return meta.SystemEntityDef{}, false
}

// systemMemberResolution resolves a bare member of a System entity: one of its
// attributes or its generalizations', or — the bare spelling being
// MDL-XPATH01's to report, not this check's — one of its associations.
func systemMemberResolution(entityQN, member string) memberResolution {
	def, ok := modelerSystemEntity(entityQN)
	if !ok {
		return memberUnknown
	}
	ends := map[string]bool{}
	for seen := map[string]bool{}; !seen[def.Name]; {
		seen[def.Name] = true
		ends[def.Name] = true
		for _, a := range def.Attributes {
			if a.Name == member {
				return memberFound
			}
		}
		if def.Generalization == "" {
			break
		}
		if def, ok = modelerSystemEntity(def.Generalization); !ok {
			return memberUnknown
		}
	}
	for _, a := range meta.ModelerSystemAssociations() {
		if a.Name == member && (ends[a.Parent] || ends[a.Child]) {
			return memberFound
		}
	}
	return memberMissing
}
