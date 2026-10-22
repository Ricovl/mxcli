// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"

	"github.com/mendixlabs/mxcli/sdk/security"
)

// buildRoleMappings populates the role_mappings table from project security.
// This maps user roles to their assigned module roles.
func (b *Builder) buildRoleMappings() error {
	ps, err := b.reader.GetProjectSecurity()
	if err != nil {
		// No project security — skip silently
		return nil
	}
	if ps == nil {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO role_mappings (UserRoleName, ModuleRoleName, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID := b.catalog.projectID
	snapshotID := b.snapshot.ID
	count := 0

	for _, ur := range ps.UserRoles {
		for _, mr := range ur.ModuleRoles {
			// Module role is a qualified name like "MyModule.Admin"
			moduleName := ""
			if parts := strings.SplitN(mr, ".", 2); len(parts) == 2 {
				moduleName = parts[0]
			}
			stmt.Exec(ur.Name, mr, moduleName, projectID, snapshotID)
			count++
		}
	}

	b.report("RoleMappings", count)

	return b.buildUserRoles(ps)
}

// buildUserRoles writes one user_roles row per project user role, with the
// documentation a "every user role is documented" rule needs (ako/mxcli#1442).
func (b *Builder) buildUserRoles(ps *security.ProjectSecurity) error {
	stmt, err := b.tx.Prepare(`
		INSERT INTO user_roles (Name, Description, IsAnonymous, ModuleRoleCount, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ur := range ps.UserRoles {
		isAnonymous := 0
		if ur.Name == ps.GuestUserRole {
			isAnonymous = 1
		}
		if _, err := stmt.Exec(ur.Name, ur.Description, isAnonymous, len(ur.ModuleRoles),
			b.catalog.projectID, b.snapshot.ID); err != nil {
			return err
		}
	}

	b.report("User Roles", len(ps.UserRoles))
	return nil
}
