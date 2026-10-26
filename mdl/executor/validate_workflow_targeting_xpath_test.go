// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A user task's targeting constraint was never checked (ako/mxcli#1711): an
// agent's `[System.User/Name = 'MxAdmin']` passed `check --references` and
// exec, and Studio Pro reported CE0161 on every task carrying it. Every case
// below was measured with mx check on mxbuild 11.15.0 (blank app).

func targetingTask(kind, xpath string) *ast.WorkflowUserTaskNode {
	return &ast.WorkflowUserTaskNode{
		Name: "Review", Caption: "Review",
		Targeting: ast.WorkflowTargetingNode{Kind: kind, XPath: xpath},
	}
}

func TestWorkflowTargetingXPath_ReportsWhatMxbuildRejects(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct {
		name, kind, xpath string
		want              []string
	}{
		// The reported constraint: the entity the constraint is evaluated on,
		// written as a path step.
		{"a leading System.User step", "xpath", "[System.User/Name = 'MxAdmin']",
			[]string{"System.User", "CE0161", "[Name = …]"}},
		{"a member System.User does not have", "xpath", "[NoSuchAttr = 'x']",
			[]string{"NoSuchAttr", "System.User", "CE0161"}},
		{"a leading System.WorkflowGroup step on group targeting", "group_xpath", "[System.WorkflowGroup/Name = 'Reviewers']",
			[]string{"System.WorkflowGroup", "CE0161"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateWorkflowTargetingXPath(ctx,
				[]ast.WorkflowActivityNode{targetingTask(tc.kind, tc.xpath)}, newScriptContext())
			if len(errs) != 1 {
				t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
			}
			for _, w := range tc.want {
				if !strings.Contains(errs[0], w) {
					t.Errorf("error should mention %q: %s", w, errs[0])
				}
			}
		})
	}
}

// CONTROL: each of these built clean on the same app.
func TestWorkflowTargetingXPath_AcceptsWhatMxbuildBuilds(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, kind, xpath string }{
		{"a bare attribute", "xpath", "[Name = 'MxAdmin']"},
		{"a role through System.UserRoles", "xpath", "[System.UserRoles = '[%UserRole_User%]']"},
		{"a path through System.UserRoles to the role's name", "xpath", "[System.UserRoles/System.UserRole/Name = 'User']"},
		{"the qualified attribute the writer resolves", "xpath", "[System.User.Name = 'MxAdmin']"},
		{"a boolean attribute", "xpath", "[Active = true()]"},
		{"an implicit member", "xpath", "[createdDate < '[%BeginOfCurrentDay%]']"},
		{"a group's attribute", "group_xpath", "[Name = 'Reviewers']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := validateWorkflowTargetingXPath(ctx,
				[]ast.WorkflowActivityNode{targetingTask(tc.kind, tc.xpath)}, newScriptContext()); len(errs) != 0 {
				t.Errorf("%s was reported, but mxbuild builds it: %v", tc.xpath, errs)
			}
		})
	}
}

// Microflow targeting and no targeting carry no constraint.
func TestWorkflowTargetingXPath_IgnoresTasksWithoutAConstraint(t *testing.T) {
	ctx := memberFixture(t)
	acts := []ast.WorkflowActivityNode{
		&ast.WorkflowUserTaskNode{Name: "A", Targeting: ast.WorkflowTargetingNode{Kind: "microflow",
			Microflow: ast.QualifiedName{Module: "Shop", Name: "Targets"}}},
		&ast.WorkflowUserTaskNode{Name: "B"},
	}
	if errs := validateWorkflowTargetingXPath(ctx, acts, newScriptContext()); len(errs) != 0 {
		t.Errorf("a task without an XPath constraint was reported: %v", errs)
	}
}

// A task nested in an outcome is checked like one in the main flow.
func TestWorkflowTargetingXPath_ReachesNestedTasks(t *testing.T) {
	ctx := memberFixture(t)
	outer := &ast.WorkflowUserTaskNode{Name: "Outer", Outcomes: []ast.WorkflowUserTaskOutcomeNode{{
		Caption:    "Escalate",
		Activities: []ast.WorkflowActivityNode{targetingTask("xpath", "[System.User/Name = 'Boss']")},
	}}}
	if errs := validateWorkflowTargetingXPath(ctx, []ast.WorkflowActivityNode{outer}, newScriptContext()); len(errs) != 1 {
		t.Fatalf("the nested task's constraint was not checked: %v", errs)
	}
}

// When App Settings > Workflows names a specialization of System.User, the
// constraint is evaluated on it: its own attributes are members too, and an
// attribute the project model cannot rule out stays silent.
func TestWorkflowTargetingXPath_UsesTheConfiguredUserEntity(t *testing.T) {
	ctx := userEntityFixture(t)
	for _, xpath := range []string{"[FullName = 'Jane']", "[Name = 'jane']"} {
		if errs := validateWorkflowTargetingXPath(ctx,
			[]ast.WorkflowActivityNode{targetingTask("xpath", xpath)}, newScriptContext()); len(errs) != 0 {
			t.Errorf("%s names a member of the configured user entity, but was reported: %v", xpath, errs)
		}
	}
	// The leading entity step is wrong whichever entity it names.
	if errs := validateWorkflowTargetingXPath(ctx,
		[]ast.WorkflowActivityNode{targetingTask("xpath", "[Accounts.Account/FullName = 'Jane']")}, newScriptContext()); len(errs) != 1 {
		t.Errorf("a leading Accounts.Account step was not reported: %v", errs)
	}
}

// ALTER WORKFLOW … set (Targeting: xpath …) on X writes the same constraint.
func TestWorkflowTargetingXPath_ChecksAlterSetTargeting(t *testing.T) {
	ctx := memberFixture(t)
	stmt := &ast.AlterWorkflowStmt{
		Name: ast.QualifiedName{Module: "Shop", Name: "Approve"},
		Operations: []ast.AlterWorkflowOp{
			&ast.SetActivityPropertyOp{ActivityRef: "Review", Property: "targeting_xpath", Value: "[System.User/Name = 'MxAdmin']"},
		},
	}
	errs := validateAlterWorkflowTargetingXPath(ctx, stmt, nil, newScriptContext())
	if len(errs) != 1 || !strings.Contains(errs[0], "CE0161") {
		t.Fatalf("set (Targeting: xpath [System.User/Name = …]) was not reported: %v", errs)
	}
}

// userEntityFixture is memberFixture's project with App Settings > Workflows >
// User entity set to Accounts.Account, a specialization of System.User.
func userEntityFixture(t *testing.T) *ExecContext {
	t.Helper()
	ctx := memberFixture(t)
	mb := ctx.Backend.(*mock.MockBackend)

	accounts := &model.Module{Name: "Accounts"}
	accounts.ID = nextID("accounts")
	account := &domainmodel.Entity{Name: "Account", Persistable: true, GeneralizationRef: "System.User"}
	account.ID = nextID("account")
	fullName := &domainmodel.Attribute{Name: "FullName", Type: &domainmodel.StringAttributeType{}}
	fullName.ID = nextID("fullname")
	account.Attributes = []*domainmodel.Attribute{fullName}
	adm := &domainmodel.DomainModel{ContainerID: accounts.ID, Entities: []*domainmodel.Entity{account}}
	adm.ID = nextID("adm")

	listModules, getModule, getDM := mb.ListModulesFunc, mb.GetModuleByNameFunc, mb.GetDomainModelFunc
	mb.ListModulesFunc = func() ([]*model.Module, error) {
		mods, err := listModules()
		return append(mods, accounts), err
	}
	mb.GetModuleByNameFunc = func(name string) (*model.Module, error) {
		if name == "Accounts" {
			return accounts, nil
		}
		if name == "System" {
			return nil, fmt.Errorf("no module %q", name)
		}
		return getModule(name)
	}
	mb.GetDomainModelFunc = func(id model.ID) (*domainmodel.DomainModel, error) {
		if id == accounts.ID {
			return adm, nil
		}
		return getDM(id)
	}
	mb.GetProjectSettingsFunc = func() (*model.ProjectSettings, error) {
		return &model.ProjectSettings{Workflows: &model.WorkflowsSettings{UserEntity: "Accounts.Account"}}, nil
	}
	return ctx
}
