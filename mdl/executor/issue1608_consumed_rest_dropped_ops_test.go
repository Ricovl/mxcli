// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// Step 4: CREATE OR MODIFY naming only new operations removes PostBooking.
// That is what "modify to this shape" means, but it has to say so.
func TestIssue1608_CreateOrModifyNamesTheOperationsItRemoves(t *testing.T) {
	exec, out := restProject(t)
	run(t, exec, bookingAPIStep1)
	out.Reset()
	run(t, exec, `create or modify consumed rest service MyFirstModule.BookingApi (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation GetBooking (Method: get, Path: '/bookings/{id}', Parameters: ($id: String), Response: json as $Result)
  operation PutBooking (Method: put, Path: '/bookings/{id}', Parameters: ($id: String), Body: json from $Payload, Response: json as $Result)
};`)
	got := out.String()
	for _, want := range []string{"PostBooking", "alter consumed rest service MyFirstModule.BookingApi add operation"} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}

	// Control: a CREATE OR MODIFY that keeps every operation says nothing of
	// the kind.
	out.Reset()
	run(t, exec, `create or modify consumed rest service MyFirstModule.BookingApi (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation GetBooking (Method: get, Path: '/bookings/{id}', Parameters: ($id: String), Response: json)
  operation PutBooking (Method: put, Path: '/bookings/{id}', Parameters: ($id: String), Body: json from $Payload, Response: json)
};`)
	if got := out.String(); strings.Contains(got, "drops") {
		t.Errorf("a statement that keeps every operation reported a drop:\n%s", got)
	}
}
