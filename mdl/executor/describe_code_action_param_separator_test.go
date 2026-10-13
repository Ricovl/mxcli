// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// documentedParams has a description on non-final parameters: describe
// prints each as a trailing `--` comment, so the separator must come before it.
func documentedParams() []*types.JavaActionParameter {
	return []*types.JavaActionParameter{
		{Name: "Name", Description: "This sets the Name of the tool.", IsRequired: true, ParameterType: &types.StringType{}},
		{Name: "Amount", Description: "How many.", ParameterType: &types.IntegerType{}},
		{Name: "Extra", ParameterType: &types.StringType{}},
	}
}

// mendixlabs/mxcli#1412: the separator was written at the start of the next
// parameter, i.e. after the previous one's `-- doc` comment, so the comment
// swallowed the comma and the next parameter failed to parse.
func TestDescribeJavaAction_ParamDocCommentKeepsSeparator(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) {
			return &javaactions.JavaAction{
				BaseElement: model.BaseElement{ID: nextID("ja")},
				ContainerID: mod.ID,
				Name:        "CreateTool",
				Parameters:  documentedParams(),
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaAction(ctx, ast.QualifiedName{Module: "MyModule", Name: "CreateTool"}))
	out := buf.String()
	stmt := findStmt[*ast.CreateJavaActionStmt](t, reparse(t, out), out)
	if len(stmt.Parameters) != 3 {
		t.Fatalf("re-parsed %d parameters, want 3\n%s", len(stmt.Parameters), out)
	}
}

func TestDescribeJavaScriptAction_ParamDocCommentKeepsSeparator(t *testing.T) {
	mod := mkModule("WebMod")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ReadJavaScriptActionByNameFunc: func(string) (*types.JavaScriptAction, error) {
			return &types.JavaScriptAction{
				BaseElement: model.BaseElement{ID: nextID("jsa")},
				ContainerID: mod.ID,
				Name:        "CreateTool",
				Platform:    "Web",
				Parameters:  documentedParams(),
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeJavaScriptAction(ctx, ast.QualifiedName{Module: "WebMod", Name: "CreateTool"}))
	out := buf.String()
	stmt := findStmt[*ast.CreateJavaScriptActionStmt](t, reparse(t, out), out)
	if len(stmt.Parameters) != 3 {
		t.Fatalf("re-parsed %d parameters, want 3\n%s", len(stmt.Parameters), out)
	}
}
