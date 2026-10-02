// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#557: a script opening with `create user role Administrator (…)` —
// the role a blank app already ships — passed `check --references` ("Check
// passed!") and was then refused by exec ("user role already exists"), after
// the statements before it had been written. CreateUserRoleStmt was in neither
// stmtCreateKind nor setFor, so the guard that compares those two switches
// could not see it. The same held for every other create whose handler
// refuses an existing element and that neither switch named.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

func setup557Ctx(t *testing.T) *ExecContext {
	t.Helper()
	ctx, mod := setupProjectConflictCtx(t)
	mb := ctx.Backend.(*mock.MockBackend)
	mb.GetProjectSecurityFunc = func() (*security.ProjectSecurity, error) {
		return &security.ProjectSecurity{UserRoles: []*security.UserRole{{Name: "Administrator"}}}, nil
	}
	mb.ListQueuesFunc = func() ([]*types.Queue, error) {
		return []*types.Queue{{ContainerID: mod.ID, Name: "Jobs"}}, nil
	}
	mb.ListRegularExpressionsFunc = func() ([]*model.RegularExpression, error) {
		return []*model.RegularExpression{{ContainerID: mod.ID, Name: "Digits"}}, nil
	}
	return ctx
}

func TestCheckProjectConflicts_UserRole557(t *testing.T) {
	assertHasConflict(t, setup557Ctx(t), `create user role Administrator (M.User);`, "user role already exists in project: Administrator")
}

// The queue handler finds the stored queue case-insensitively, so check has to.
func TestCheckProjectConflicts_OtherRefusingCreates557(t *testing.T) {
	assertHasConflict(t, setup557Ctx(t), `create queue M.Jobs ( Parallelism: 2 );`, "queue already exists in project: M.Jobs")
	assertHasConflict(t, setup557Ctx(t), `create queue M.jobs ( Parallelism: 2 );`, "M.jobs")
	assertHasConflict(t, setup557Ctx(t), `create regular expression M.Digits ( Expression: '[0-9]+' );`, "regular expression already exists in project: M.Digits")
}

// CONTROL: the re-runnable spellings, a dropped role and a new role pass, as
// they do at exec.
func TestCheckProjectConflicts_UserRole557_NotReported(t *testing.T) {
	for name, src := range map[string]string{
		"or modify":     `create or modify user role Administrator (M.User);`,
		"if not exists": `create user role if not exists Administrator (M.User);`,
		"dropped first": "drop user role Administrator;\ncreate user role Administrator (M.User);",
		"new role":      `create user role Reviewer (M.User);`,
		"queue, new":    `create queue M.Other ( Parallelism: 2 );`,
	} {
		t.Run(name, func(t *testing.T) { assertNoConflicts(t, setup557Ctx(t), src) })
	}
}
