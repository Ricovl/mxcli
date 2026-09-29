// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#794: `mxcli diff` rendered an existing enumeration as
//
//	create enumeration .Enum_DistanceUnit (
//
// — an empty module name. The enumeration sat in a FOLDER, so its ContainerID
// is the folder's ID, and diffEnumeration asked the hierarchy for the module
// NAME of that ID directly instead of walking up to the module first. The same
// defect as DROP ENUMERATION's #976, in a second resolver. The consequence is
// not cosmetic: the stored side's header never matches the script's, so an
// untouched enumeration diffs as "modified".
//
// The issue asked for every diff kind to be checked for the same empty
// qualifier, so this is one table over all of them: each document exists in the
// project, in a folder wherever the kind can live in one, and the stored side of
// the diff must name its module.

// emptyQualifier matches a qualified name whose module part is missing:
// ".Name" after a space, a paren or the start of a line.
var emptyQualifier = regexp.MustCompile(`(^|[\s(])\.[A-Za-z_]`)

// diffModuleNameFixture is module M with a folder F holding an enumeration, a
// microflow and a nanoflow; the domain model holds a persistent entity, a view
// entity and an association between two entities. `Root` is the control: the
// same enumeration at the module root, which rendered correctly before the fix.
func diffModuleNameFixture(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	folderID := model.ID("folder-f")

	h := mkHierarchy(mod)
	withContainer(h, folderID, mod.ID)
	h.folderNames[folderID] = "F"

	filed := mkEnumeration(folderID, "Filed", "Red")
	root := mkEnumeration(mod.ID, "Root", "Red")

	dm := mkDomainModel(mod.ID)
	withContainer(h, dm.ID, mod.ID)
	parent := mkEntity(dm.ID, "Parent")
	child := mkEntity(dm.ID, "Child")
	view := mkEntity(dm.ID, "View")
	view.Persistable = false
	view.Source = "DomainModels$OqlViewEntitySource"
	view.OqlQuery = "select 1"
	dm.Entities = []*domainmodel.Entity{parent, child, view}
	dm.Associations = []*domainmodel.Association{mkAssociation(dm.ID, "Child_Parent", child.ID, parent.ID)}

	mf := mkMicroflow(folderID, "MF")
	nf := mkNanoflow(folderID, "NF")

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{filed, root}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{dm}, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListNanoflowsFunc:  func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nf}, nil },
		GetMicroflowFunc:   func(model.ID) (*microflows.Microflow, error) { return mf, nil },
		GetNanoflowFunc:    func(model.ID) (*microflows.Nanoflow, error) { return nf, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func TestDiff_StoredSideNamesItsModule(t *testing.T) {
	cases := []struct {
		kind, src, want string
	}{
		{"enumeration in a folder (#794)", "create or modify enumeration M.Filed (Red 'Red');", "create enumeration M.Filed ("},
		{"enumeration at the module root (control)", "create or modify enumeration M.Root (Red 'Red');", "create enumeration M.Root ("},
		{"entity", "create or modify persistent entity M.Parent ();", "entity M.Parent ("},
		{"view entity", "create or modify view entity M.View () as (select 1);", "create view entity M.View ("},
		{"association", "create or modify association M.Child_Parent from M.Child to M.Parent;", "create association M.Child_Parent"},
		{"microflow in a folder", "create or modify microflow M.MF () begin end;", "M.MF"},
		{"nanoflow in a folder", "create or modify nanoflow M.NF () begin end;", "M.NF"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			ctx := diffModuleNameFixture(t)
			prog, errs := visitor.Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse %q: %v", tc.src, errs)
			}
			res, err := diffStatement(ctx, prog.Statements[0])
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if res.IsNew {
				t.Fatalf("the document exists in the project; diff reported it as new")
			}
			if !strings.Contains(res.Current, tc.want) {
				t.Errorf("stored side does not name its module (want %q):\n%s", tc.want, res.Current)
			}
			t.Logf("stored side:\n%s", res.Current)
			for line := range strings.SplitSeq(res.Current, "\n") {
				if emptyQualifier.MatchString(line) {
					t.Errorf("stored side has an empty module qualifier: %q", line)
				}
			}
		})
	}
}

// A flow's stored side is headed by the statement's own name, so the table above
// cannot catch an empty qualifier inside its body. Those references — called
// microflows, retrieved entities — are printed from flowNameMaps, and the flows
// here are filed in a folder: every name must still carry its module.
func TestDiff_FlowReferenceNamesCarryTheirModule(t *testing.T) {
	ctx := diffModuleNameFixture(t)
	entityNames, microflowNames, err := flowNameMaps(ctx)
	if err != nil {
		t.Fatalf("flowNameMaps: %v", err)
	}
	want := map[string]bool{"M.Parent": true, "M.Child": true, "M.View": true, "M.MF": true, "M.NF": true}
	got := map[string]bool{}
	for _, m := range []map[model.ID]string{entityNames, microflowNames} {
		for _, name := range m {
			got[name] = true
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("reference name %q missing; flowNameMaps produced %v", name, got)
		}
	}
	for name := range got {
		if strings.HasPrefix(name, ".") {
			t.Errorf("reference name %q has an empty module qualifier", name)
		}
	}
}

// The consequence the user sees: an enumeration in a folder, diffed against a
// script that defines it exactly as stored, must be unchanged. Before the fix the
// two headers differed (".Filed" vs "M.Filed") and it diffed as modified.
func TestDiff_UnchangedEnumerationInAFolderIsUnchanged(t *testing.T) {
	ctx := diffModuleNameFixture(t)
	prog, errs := visitor.Build("create or modify enumeration M.Filed (Red '');")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := diffStatement(ctx, prog.Statements[0])
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if res.Current != res.Proposed {
		t.Errorf("an enumeration identical to the stored one must diff as unchanged\ncurrent:\n%s\nproposed:\n%s", res.Current, res.Proposed)
	}
}
