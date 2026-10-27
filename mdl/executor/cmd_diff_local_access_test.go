// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1630 (ako/mxcli#1562): after
//
//	grant read *, write * on entity Shop.Customer to Shop.User;
//
// `diff-local` printed only "Summary: 0 new, 1 modified, 0 deleted". The
// domain model renderer printed entities and attributes but not their access
// rules, so a script that changes access rendered identically on both sides.
package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

const (
	acc1630Module = "33333333-3333-3333-3333-333333333333"
	acc1630Unit   = "84776610-13b2-48cb-a265-d5e984d63129"
)

// acc1630DomainModel is a domain model unit as stored: one entity whose single
// access rule gives MyFirstModule.User the default member rights.
func acc1630DomainModel(rights string) map[string]any {
	return map[string]any{
		"$Type": "DomainModels$DomainModel",
		"Entities": []any{int32(3), map[string]any{
			"$Type": "DomainModels$Entity",
			"Name":  "Customer",
			"Attributes": []any{int32(3), map[string]any{
				"$Type":   "DomainModels$Attribute",
				"Name":    "Name",
				"NewType": map[string]any{"$Type": "DomainModels$StringAttributeType", "Length": int32(100)},
			}},
			"AccessRules": []any{int32(3), map[string]any{
				"$Type":                     "DomainModels$AccessRule",
				"AllowedModuleRoles":        []any{int32(1), "MyFirstModule.User"},
				"DefaultMemberAccessRights": rights,
				"XPathConstraint":           "",
				"MemberAccesses": []any{int32(3), map[string]any{
					"$Type":        "DomainModels$MemberAccess",
					"Attribute":    "MyFirstModule.Customer.Name",
					"AccessRights": rights,
				}},
			}},
		}},
	}
}

// acc1630ParseDomainModel stands in for the backend's raw decode. Like the real
// one it reads the roles from the storage key "AllowedModuleRoles" and refuses
// any other document type.
func acc1630ParseDomainModel(raw map[string]any, _, _ model.ID) *domainmodel.DomainModel {
	if raw["$Type"] != "DomainModels$DomainModel" {
		return nil
	}
	dm := &domainmodel.DomainModel{}
	for _, e := range extractBsonArray(raw["Entities"]) {
		em := e.(map[string]any)
		ent := &domainmodel.Entity{Name: extractString(em["Name"])}
		for _, a := range extractBsonArray(em["Attributes"]) {
			am := a.(map[string]any)
			attr := &domainmodel.Attribute{Name: extractString(am["Name"])}
			attr.ID = model.ID("attr-" + attr.Name)
			ent.Attributes = append(ent.Attributes, attr)
		}
		for _, r := range extractBsonArray(em["AccessRules"]) {
			rm := r.(map[string]any)
			rule := &domainmodel.AccessRule{
				DefaultMemberAccessRights: domainmodel.MemberAccessRights(extractString(rm["DefaultMemberAccessRights"])),
				XPathConstraint:           extractString(rm["XPathConstraint"]),
			}
			for _, role := range extractBsonArray(rm["AllowedModuleRoles"]) {
				if s, ok := role.(string); ok {
					rule.ModuleRoleNames = append(rule.ModuleRoleNames, s)
				}
			}
			for _, m := range extractBsonArray(rm["MemberAccesses"]) {
				mm := m.(map[string]any)
				rule.MemberAccesses = append(rule.MemberAccesses, &domainmodel.MemberAccess{
					AttributeName: extractString(mm["Attribute"]),
					AccessRights:  domainmodel.MemberAccessRights(extractString(mm["AccessRights"])),
				})
			}
			ent.AccessRules = append(ent.AccessRules, rule)
		}
		dm.Entities = append(dm.Entities, ent)
	}
	return dm
}

func acc1630Context() (*ExecContext, *bytes.Buffer) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		VersionFunc:     func() types.MPRVersion { return 2 },
		ContentsDirFunc: func() string { return "mprcontents" },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: acc1630Module}, Name: "MyFirstModule"}}, nil
		},
		ListUnitsFunc: func() ([]*types.UnitInfo, error) {
			return []*types.UnitInfo{{ID: acc1630Unit, ContainerID: acc1630Module}}, nil
		},
		ParseDomainModelFromRawFunc: acc1630ParseDomainModel,
	}
	out := &bytes.Buffer{}
	return &ExecContext{Context: context.Background(), Backend: mb, Output: out, Cache: &executorCache{}}, out
}

func TestDiffLocal_GrantOnlyChangeShowsAccessRuleHunk(t *testing.T) {
	unitPath := "mprcontents/84/77/" + acc1630Unit + ".mxunit"
	stubGitRevisions(t, unitPath, map[string]map[string]any{
		"HEAD~1": acc1630DomainModel("ReadOnly"),
		"HEAD":   acc1630DomainModel("ReadWrite"),
	})
	ctx, out := acc1630Context()

	if err := diffLocal(ctx, "HEAD~1..HEAD", DiffOptions{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"-grant read * on entity MyFirstModule.Customer to MyFirstModule.User;",
		"+grant read *, write * on entity MyFirstModule.Customer to MyFirstModule.User;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("diff-local output lacks %q — a grant-only change shows no hunk (#1630):\n%s", want, got)
		}
	}
}
