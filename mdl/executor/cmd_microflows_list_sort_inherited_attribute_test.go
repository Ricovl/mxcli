// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1430: the list-operation `sort $L by Attr` qualified an
// INHERITED attribute against the list's own entity, which mxbuild rejects:
//
//	CE1613 "The selected attribute 'Embeddings_SemanticSearch.BookChunk.Similarity'
//	no longer exists."
//
// The retrieve `sort by` path was fixed for the same class (CapTrackV2 §13) via
// resolveAttributeInEntityHierarchy; the list-operation path still prefixed
// the list entity blindly. Reuses inheritedSortBackend: AppUser extends
// SyntheticBase.User, which declares Name; AppUser declares FullName itself.
func listSortAttrOf(t *testing.T, attr string) []string {
	t.Helper()
	fb := &flowBuilder{
		backend:  inheritedSortBackend(),
		spacing:  100,
		varTypes: map[string]string{"Users": "List of SyntheticApp.AppUser"},
	}
	fb.addListOperationAction(&ast.ListOperationStmt{
		OutputVariable: "Sorted",
		Operation:      ast.ListOpSort,
		InputVariable:  "Users",
		SortSpecs:      []ast.SortSpec{{Attribute: attr, Ascending: false}},
	})
	if len(fb.errors) > 0 {
		t.Fatalf("unexpected errors: %v", fb.errors)
	}
	var paths []string
	for _, obj := range fb.objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		lo, ok := act.Action.(*microflows.ListOperationAction)
		if !ok {
			continue
		}
		if so, ok := lo.Operation.(*microflows.SortOperation); ok {
			for _, s := range so.Sorting {
				paths = append(paths, s.AttributeQualifiedName)
			}
		}
	}
	return paths
}

func TestListSort_InheritedAttributeUsesTheDeclaringEntity(t *testing.T) {
	paths := listSortAttrOf(t, "Name")
	if len(paths) != 1 || paths[0] != "SyntheticBase.User.Name" {
		t.Errorf("stored %v, want [SyntheticBase.User.Name] — the list's own entity "+
			"does not declare Name, and mxbuild rejects that reference with CE1613", paths)
	}
}

// CONTROL: an attribute the entity declares itself is still qualified with it.
func TestListSort_OwnAttributeIsUnchanged(t *testing.T) {
	paths := listSortAttrOf(t, "FullName")
	if len(paths) != 1 || paths[0] != "SyntheticApp.AppUser.FullName" {
		t.Errorf("stored %v, want [SyntheticApp.AppUser.FullName]", paths)
	}
}
