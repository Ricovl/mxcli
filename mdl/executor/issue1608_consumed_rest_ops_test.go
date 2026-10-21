// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1608 (ako/mxcli#1536), against a real project: the report's
// script, step by step. A consumed REST service with one operation could not
// gain a second from MDL — no ALTER, a DESCRIBE that did not re-parse, and a
// CREATE OR MODIFY that deleted the unlisted operation saying only
// "Modified rest client: MyFirstModule.BookingApi (2 operations)".

const bookingAPIStep1 = `create consumed rest service MyFirstModule.BookingApi (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation PostBooking (
    Method: post,
    Path: '/bookings',
    Body: json from $Payload,
    Response: json as $Result
  )
};`

// restProject is an executor connected to a copy of the fixture project,
// writing its output to the returned buffer.
func restProject(t *testing.T) (*Executor, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	exec := New(&out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	out.Reset()
	return exec, &out
}

// restOperations reads the stored service's operation names.
func restOperations(t *testing.T, exec *Executor, name string) []string {
	t.Helper()
	svcs, err := exec.backend.ListConsumedRestServices()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range svcs {
		if s.Name == name {
			var names []string
			for _, op := range s.Operations {
				names = append(names, op.Name)
			}
			return names
		}
	}
	t.Fatalf("no consumed rest service %s", name)
	return nil
}

func restService(t *testing.T, exec *Executor, name string) *model.ConsumedRestService {
	t.Helper()
	svcs, err := exec.backend.ListConsumedRestServices()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range svcs {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no consumed rest service %s", name)
	return nil
}

// Step 3: DESCRIBE's output runs again, and changes nothing.
func TestIssue1608_DescribeOutputReExecutes(t *testing.T) {
	exec, out := restProject(t)
	run(t, exec, bookingAPIStep1)
	out.Reset()
	run(t, exec, "describe consumed rest service MyFirstModule.BookingApi")
	described := out.String()
	if !strings.Contains(described, "Response: json") {
		t.Fatalf("unexpected describe output:\n%s", described)
	}

	prog, errs := visitor.Build(described)
	if len(errs) > 0 {
		t.Fatalf("describe output does not re-parse: %v\n%s", errs, described)
	}
	out.Reset()
	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			t.Fatalf("re-executing describe output: %v\n%s", err, described)
		}
	}
	if got := out.String(); !strings.Contains(got, "Unchanged") {
		t.Errorf("re-executing describe output wrote the service:\n%s", got)
	}
	if ops := restOperations(t, exec, "BookingApi"); len(ops) != 1 || ops[0] != "PostBooking" {
		t.Errorf("operations after re-exec = %v", ops)
	}
}
