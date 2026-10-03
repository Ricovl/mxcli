// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	mxbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// The activity property columns (mendixlabs/mxcli#1267) measured on Studio
// Pro-authored BSON: ako/TestApp, read through the real model reader, so the
// assertions hold for what Studio Pro stores and not only for hand-built
// objects. Each expected value below was read off the project.

var (
	testAppCatalogOnce sync.Once
	testAppCatalog     *Catalog
	testAppCatalogErr  error
)

func testAppActivities(t *testing.T) *Catalog {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "testapp", "TestApp", "TestApp.mpr")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("TestApp submodule not initialised (git submodule update --init testdata/testapp): %v", err)
	}
	testAppCatalogOnce.Do(func() {
		be := mxbackend.New()
		if err := be.ConnectReadOnly(src); err != nil {
			testAppCatalogErr = err
			return
		}
		defer be.Disconnect()
		cat, err := New()
		if err != nil {
			testAppCatalogErr = err
			return
		}
		b := NewBuilder(cat, be)
		b.SetFullMode(true)
		if err := b.Build(nil); err != nil {
			testAppCatalogErr = err
			return
		}
		testAppCatalog = cat
	})
	if testAppCatalogErr != nil {
		t.Fatalf("build TestApp catalog: %v", testAppCatalogErr)
	}
	return testAppCatalog
}

// one returns the single row a query yields, as strings.
func one(t *testing.T, cat *Catalog, q string) []any {
	t.Helper()
	res, err := cat.Query(q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	if res.Count != 1 {
		t.Fatalf("%s: got %d rows, want 1", q, res.Count)
	}
	return res.Rows[0]
}

func TestTestAppActivityProperties(t *testing.T) {
	cat := testAppActivities(t)

	cases := []struct {
		name  string
		query string
		want  []any
	}{
		{
			// Error handling lives on the action, not the activity; the
			// stored spelling has a capital B.
			"error handling, custom without rollback",
			`SELECT ErrorHandlingType, Caption, AutoGenerateCaption FROM activities
			 WHERE MicroflowQualifiedName = 'FeedbackModule.SUB_Feedback_SendToServer'
			   AND Caption = 'Post feedback to App Insights'`,
			[]any{"CustomWithoutRollBack", "Post feedback to App Insights", int64(0)},
		},
		{
			"error handling, continue",
			`SELECT ErrorHandlingType FROM activities
			 WHERE MicroflowQualifiedName = 'FeedbackModule.SUB_Feedback_SendToServer'
			   AND Caption = 'XSS Sanitize Feedback'`,
			[]any{"Continue"},
		},
		{
			"error handling, custom on a commit",
			`SELECT ErrorHandlingType, WithEvents FROM activities
			 WHERE MicroflowQualifiedName = 'Services.SaveOrder' AND ActionType = 'CommitObjectsAction'`,
			[]any{"Custom", int64(1)},
		},
		{
			// An auto-generated caption stores Studio Pro's placeholder.
			"auto-generated caption",
			`SELECT Caption, AutoGenerateCaption FROM activities
			 WHERE MicroflowQualifiedName = 'Services.SaveOrder' AND ActionType = 'LogMessageAction'`,
			[]any{"Activity", int64(1)},
		},
		{
			"log message",
			`SELECT LogLevel, LogNodeExpression, LogMessage FROM activities
			 WHERE MicroflowQualifiedName = 'Services.SaveOrder' AND ActionType = 'LogMessageAction'`,
			[]any{"Info", "'Soap'", "Saving order for customer {1}"},
		},
		{
			"retrieve over an association",
			`SELECT RetrieveSource FROM activities
			 WHERE MicroflowQualifiedName = 'Services.SaveOrder' AND ActionType = 'RetrieveAction'`,
			[]any{"association"},
		},
		{
			"split caption",
			`SELECT Caption, ConditionRule FROM activities
			 WHERE ActivityType = 'ExclusiveSplit' AND Caption = 'Old password okay?'`,
			[]any{"Old password okay?", ""},
		},
		{
			"rule-based split",
			`SELECT ConditionRule, ConditionExpression FROM activities
			 WHERE MicroflowQualifiedName = 'Rules.MicroflowUsingRule' AND ActivityType = 'ExclusiveSplit'`,
			[]any{"Rules.Rule1", ""},
		},
		{
			"annotation text",
			`SELECT Caption FROM activities
			 WHERE ActivityType = 'Annotation' AND Caption = 'The feedback is sanitized to prevent XSS.'`,
			[]any{"The feedback is sanitized to prevent XSS."},
		},
		{
			"web service call",
			`SELECT ServiceRef, ActionRef, UseRequestTimeout, TimeoutExpression FROM activities
			 WHERE MicroflowQualifiedName = 'Clients.GetOrders' AND ActionType = 'WebServiceCallAction'`,
			[]any{"Clients.OrderSoapClient", "GetOrder", int64(1), "300"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := one(t, cat, tc.query)
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("column %d = %#v, want %#v (row %#v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

// A Studio Pro-authored loop body reaches the table (mendixlabs/mxcli#1266).
func TestTestAppLoopBodiesAreCatalogued(t *testing.T) {
	cat := testAppActivities(t)
	res, err := cat.Query(`SELECT a.ActionType, a.LoopDepth, l.ActivityType
		FROM activities a JOIN activities l ON l.Id = a.ParentLoopId
		WHERE a.MicroflowQualifiedName = 'WorkflowCommons.SUB_TaskAssignmentHelper_Reassign'`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count == 0 {
		t.Fatal("no loop-body rows for SUB_TaskAssignmentHelper_Reassign -- the loop body is not walked")
	}
	for _, r := range res.Rows {
		if r[1] != int64(1) || r[2] != "LoopedActivity" {
			t.Errorf("loop-body row %v: want LoopDepth 1 under a LoopedActivity", r)
		}
	}
	// Generate-jump-to options is stored but not modelled by the reader.
	got := one(t, cat, `SELECT COUNT(*) FROM activities WHERE ActionType = 'GenerateJumpToOptionsAction'`)
	if got[0] == int64(0) {
		t.Error("no GenerateJumpToOptionsAction rows -- the unsupported action is labelled by its Go placeholder type")
	}
}
