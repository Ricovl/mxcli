// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// checkDocumentAccessRolesSameModule rejects granting a document (page,
// microflow, nanoflow, or a published OData/REST service) access to a module role
// from a different module than the document. Mendix stores document access as
// references to the document's OWN module roles only — Studio Pro's role picker
// only offers the containing module's roles — so a cross-module reference builds
// with CE0148 ("reselect roles") even though it is otherwise well-formed. Without
// this guard the grant writes an invalid reference that passes mxcli but fails the
// Mendix build with an opaque error. The MOVE path enforces the same rule by
// remapping to same-named target-module roles (see remapDocumentAccessRoles); on
// an explicit GRANT we reject instead, so a wrong role or wrong document is not
// silently substituted.
//
// docKind/docName are used only for the message; docModule is the document's module.
func checkDocumentAccessRolesSameModule(docKind, docModule, docName string, roles []ast.QualifiedName) error {
	for _, role := range roles {
		if role.Module != docModule {
			return mdlerrors.NewValidationf(
				"cannot grant %s %s.%s access to %s.%s: a %s can only reference module roles from its own module %q "+
					"(Mendix rejects a cross-module document role with CE0148 \"reselect roles\"). "+
					"Grant a %s module role instead — e.g. %s.%s if it exists — and map the user role to it.",
				docKind, docModule, docName, role.Module, role.Name,
				docKind, docModule,
				docModule, docModule, role.Name)
		}
	}
	return nil
}

// validateCrossModuleGrant applies the CE0148 guard above to a GRANT statement
// without touching the project, so `check --references` reports it instead of
// letting exec fail partway through a script (#836). mxcli does not run a script
// in a single transaction, so a grant that only fails at exec time leaves every
// preceding statement applied — exactly what a pre-flight check exists to avoid.
//
// The document's module is taken from the statement's own qualified name rather
// than from the resolved document: the comparison is the same, and this keeps
// the check usable before the document exists (it may be created earlier in the
// same script).
//
// Statements other than the five document-access grants return nil.
func validateCrossModuleGrant(stmt ast.Statement) error {
	switch s := stmt.(type) {
	case *ast.GrantMicroflowAccessStmt:
		return checkDocumentAccessRolesSameModule("microflow", s.Microflow.Module, s.Microflow.Name, s.Roles)
	case *ast.GrantNanoflowAccessStmt:
		return checkDocumentAccessRolesSameModule("nanoflow", s.Nanoflow.Module, s.Nanoflow.Name, s.Roles)
	case *ast.GrantPageAccessStmt:
		return checkDocumentAccessRolesSameModule("page", s.Page.Module, s.Page.Name, s.Roles)
	case *ast.GrantODataServiceAccessStmt:
		return checkDocumentAccessRolesSameModule("OData service", s.Service.Module, s.Service.Name, s.Roles)
	case *ast.GrantPublishedRestServiceAccessStmt:
		return checkDocumentAccessRolesSameModule("published REST service", s.Service.Module, s.Service.Name, s.Roles)
	}
	return nil
}

// autoDocumentRoleDescription marks the <Module>.User role mxcli used to
// create, unasked, for a new document in a module with no module roles. It no
// longer does (mendixlabs/mxcli#1474): a security role is the author's choice.
// Projects written by an earlier mxcli still hold the role, so the description
// stays recognised — a later `create module role M.User` adopts it rather than
// reporting a duplicate.
const autoDocumentRoleDescription = "Auto-created default role for mxcli document access"

// remapDocumentAccessRoles carries a moved document's allowed roles to the
// same-named roles of its target module (a document can only reference its own
// module's roles — CE0148). A role the target does not have is dropped, never
// created.
func remapDocumentAccessRoles(ctx *ExecContext, targetModule *model.Module, currentRoles []model.ID) []model.ID {
	if targetModule == nil {
		return nil
	}

	ms, err := ctx.Backend.GetModuleSecurity(targetModule.ID)
	if err != nil || ms == nil {
		return nil
	}
	if len(ms.ModuleRoles) == 0 {
		return nil
	}

	targetRoleNames := make(map[string]bool, len(ms.ModuleRoles))
	for _, role := range ms.ModuleRoles {
		targetRoleNames[role.Name] = true
	}

	var remapped []model.ID
	seen := make(map[string]bool)
	for _, qualifiedRole := range currentRoles {
		roleName := string(qualifiedRole)
		if idx := strings.LastIndex(roleName, "."); idx >= 0 {
			roleName = roleName[idx+1:]
		}
		if !targetRoleNames[roleName] {
			continue
		}
		targetQualifiedRole := targetModule.Name + "." + roleName
		if seen[targetQualifiedRole] {
			continue
		}
		seen[targetQualifiedRole] = true
		remapped = append(remapped, model.ID(targetQualifiedRole))
	}

	return remapped
}

func documentRoleStrings(roles []model.ID) []string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		values = append(values, string(role))
	}
	return values
}

func cloneRoleIDs(roles []model.ID) []model.ID {
	if len(roles) == 0 {
		return nil
	}
	cloned := make([]model.ID, len(roles))
	copy(cloned, roles)
	return cloned
}

// pruneInvalidUserRoles removes user roles that no longer have any non-System
// module role assignments. Mendix rejects those roles with CE0157.
func pruneInvalidUserRoles(ctx *ExecContext, ps *security.ProjectSecurity) error {
	if latest, err := ctx.Backend.GetProjectSecurity(); err == nil {
		ps = latest
	} else if ps == nil {
		return err
	}

	for _, userRole := range ps.UserRoles {
		hasNonSystemRole := false
		for _, moduleRole := range userRole.ModuleRoles {
			if !strings.HasPrefix(moduleRole, "System.") {
				hasNonSystemRole = true
				break
			}
		}
		if hasNonSystemRole {
			continue
		}
		if err := ctx.Backend.RemoveUserRole(ps.ID, userRole.Name); err != nil {
			return err
		}
		if !ctx.Quiet {
			fmt.Fprintf(ctx.Output, "Dropped invalid user role: %s\n", userRole.Name)
		}
	}

	return nil
}
