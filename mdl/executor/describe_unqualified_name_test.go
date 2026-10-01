// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1218 (3): `describe entity String` failed with "module name
// is required: objects must be created within a module" — the create path's
// message, which reads as if describe were creating something. It now names
// the unqualified name.
func TestDescribeUnqualifiedEntityNamesTheName(t *testing.T) {
	for _, typ := range []ast.DescribeObjectType{ast.DescribeEntity, ast.DescribeAssociation} {
		ctx, _ := newMockCtx(t)
		err := execDescribe(ctx, &ast.DescribeStmt{ObjectType: typ, Name: ast.QualifiedName{Name: "String"}})
		if err == nil {
			t.Fatalf("%v: no error for an unqualified name", typ)
		}
		msg := err.Error()
		if strings.Contains(msg, "created") {
			t.Errorf("%v: %q still speaks of creating objects", typ, msg)
		}
		if !strings.Contains(msg, `"String" is not a qualified`) {
			t.Errorf("%v: %q does not name the unqualified name", typ, msg)
		}
	}
}
