// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1608 (ako/mxcli#1536): `alter consumed rest service` adds,
// changes and removes operations of an existing service, and leaves every
// operation it does not name as stored — including what Studio Pro wrote that
// MDL cannot say.

const bookingAPIWithQuery = `create consumed rest service MyFirstModule.BookingApi (
  BaseUrl: 'https://api.example.com',
  Authentication: none
)
{
  operation PostBooking (
    Method: post,
    Path: '/bookings',
    Query: ($dryRun: String),
    Body: json from $Payload,
    Response: json
  )
};`

// rawRestService reads the stored service unit.
func rawRestService(t *testing.T, exec *Executor, name string) (string, bson.D) {
	t.Helper()
	svc := restService(t, exec, name)
	raw, err := exec.backend.GetRawUnitBytes(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return string(svc.ID), doc
}

// rawRestOperation returns the stored operation named name, marshalled, so two
// reads compare byte for byte.
func rawRestOperation(t *testing.T, doc bson.D, name string) bson.D {
	t.Helper()
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(doc, "Operations")) {
		if od, ok := el.(bson.D); ok && bsonnav.DGetString(od, "Name") == name {
			return od
		}
	}
	t.Fatalf("no stored operation %s", name)
	return nil
}

func marshalD(t *testing.T, d bson.D) []byte {
	t.Helper()
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// seedStudioProQueryParameter gives PostBooking's query parameter what Studio
// Pro writes and mxcli never does: optional usage, and a test value. An
// element mxcli wrote holds the writer's constants, so it cannot show a
// rewrite that resets to them; this one can.
func seedStudioProQueryParameter(t *testing.T, exec *Executor) {
	t.Helper()
	id, doc := rawRestService(t, exec, "BookingApi")
	op := rawRestOperation(t, doc, "PostBooking")
	params := bsonnav.DGetArrayElements(bsonnav.DGet(op, "QueryParameters"))
	if len(params) != 1 {
		t.Fatalf("fixture has %d query parameters", len(params))
	}
	q := params[0].(bson.D)
	usage := bsonnav.DGetDoc(q, "ParameterUsage")
	bsonnav.DSet(usage, "$Type", "Rest$OptionalQueryParameterUsage")
	usage = append(usage, bson.E{Key: "Included", Value: true})
	bsonnav.DSet(q, "ParameterUsage", usage)
	if !bsonnav.DSet(q, "TestValue", "studio-pro") {
		q = append(q, bson.E{Key: "TestValue", Value: "studio-pro"})
	}
	bsonnav.DSet(op, "QueryParameters", bson.A{int32(2), q})
	if err := exec.backend.UpdateRawUnit(id, marshalD(t, doc)); err != nil {
		t.Fatal(err)
	}
	if got := queryUsage(t, exec); got != "Rest$OptionalQueryParameterUsage" {
		t.Fatalf("seed did not take: %s", got)
	}
}

func queryUsage(t *testing.T, exec *Executor) string {
	t.Helper()
	_, doc := rawRestService(t, exec, "BookingApi")
	op := rawRestOperation(t, doc, "PostBooking")
	q := bsonnav.DGetArrayElements(bsonnav.DGet(op, "QueryParameters"))[0].(bson.D)
	return bsonnav.DGetString(bsonnav.DGetDoc(q, "ParameterUsage"), "$Type")
}

// Step 2 of the report: the statement it proposed, which did not parse.
func TestIssue1608_AlterAddsAnOperationAndLeavesTheOthersAsStored(t *testing.T) {
	exec, out := restProject(t)
	run(t, exec, bookingAPIWithQuery)
	seedStudioProQueryParameter(t, exec)
	_, before := rawRestService(t, exec, "BookingApi")
	storedPost := marshalD(t, rawRestOperation(t, before, "PostBooking"))

	out.Reset()
	run(t, exec, `alter consumed rest service MyFirstModule.BookingApi add operation GetBooking (
  Method: get,
  Path: '/bookings/{id}',
  Parameters: ($id: String),
  Response: json as $Result
);`)
	if got := out.String(); !strings.Contains(got, "Altered consumed rest service: MyFirstModule.BookingApi (2 operations)") {
		t.Errorf("unexpected output:\n%s", got)
	}
	if ops := restOperations(t, exec, "BookingApi"); strings.Join(ops, ",") != "PostBooking,GetBooking" {
		t.Fatalf("operations = %v", ops)
	}
	_, after := rawRestService(t, exec, "BookingApi")
	if !bytes.Equal(marshalD(t, rawRestOperation(t, after, "PostBooking")), storedPost) {
		t.Error("adding GetBooking rewrote PostBooking")
	}
	get := restService(t, exec, "BookingApi").Operations[1]
	if get.HttpMethod != "GET" || get.Path != "/bookings/{id}" || len(get.Parameters) != 1 || get.ResponseType != "JSON" {
		t.Errorf("GetBooking stored as %+v", get)
	}

	// Adding it again is refused, naming the spelling that changes it.
	prog, errs := visitor.Build(`alter consumed rest service MyFirstModule.BookingApi add operation GetBooking (Method: get, Path: '/x', Response: none);`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	err := exec.Execute(prog.Statements[0])
	if err == nil || !strings.Contains(err.Error(), "modify operation GetBooking") {
		t.Errorf("a second add of GetBooking: %v", err)
	}
}

// Control for the test above: the rebuild CREATE OR MODIFY does — here from
// describe's own output — resets the seeded query parameter, so the seed is
// one a rewrite can be caught losing.
func TestIssue1608_CreateOrModifyRebuildResetsStudioProState(t *testing.T) {
	exec, out := restProject(t)
	run(t, exec, bookingAPIWithQuery)
	seedStudioProQueryParameter(t, exec)
	out.Reset()
	run(t, exec, "describe consumed rest service MyFirstModule.BookingApi")
	described := strings.Replace(out.String(), "Timeout: 300", "Timeout: 299", 1) // force the write
	run(t, exec, described)
	if got := queryUsage(t, exec); got == "Rest$OptionalQueryParameterUsage" {
		t.Errorf("CREATE OR MODIFY kept the seeded query parameter usage, so the seed no longer shows " +
			"a rebuild: seed a property it still resets, or the ALTER tests above prove nothing")
	}
}

func TestIssue1608_AlterModifyWritesOnlyTheNamedKeys(t *testing.T) {
	exec, out := restProject(t)
	run(t, exec, bookingAPIWithQuery)
	seedStudioProQueryParameter(t, exec)
	_, before := rawRestService(t, exec, "BookingApi")
	storedQuery := marshalD(t, bson.D{{Key: "q", Value: bsonnav.DGet(rawRestOperation(t, before, "PostBooking"), "QueryParameters")}})

	run(t, exec, `alter consumed rest service MyFirstModule.BookingApi modify operation PostBooking (Timeout: 60, Method: put);`)
	op := restService(t, exec, "BookingApi").Operations[0]
	if op.Timeout != 60 || op.HttpMethod != "PUT" {
		t.Errorf("modify did not write Timeout/Method: %+v", op)
	}
	// The body is in the method element: a method change that keeps it a
	// method with a body keeps the stored body.
	if op.BodyType != "JSON" || op.BodyVariable != "$Payload" || op.Path != "/bookings" || op.ResponseType != "JSON" {
		t.Errorf("modify changed a property it did not name: %+v", op)
	}
	_, after := rawRestService(t, exec, "BookingApi")
	if got := marshalD(t, bson.D{{Key: "q", Value: bsonnav.DGet(rawRestOperation(t, after, "PostBooking"), "QueryParameters")}}); !bytes.Equal(got, storedQuery) {
		t.Error("modify rewrote the query parameters it did not name")
	}

	// The same statement again changes nothing.
	out.Reset()
	run(t, exec, `alter consumed rest service MyFirstModule.BookingApi modify operation PostBooking (Timeout: 60, Method: put);`)
	if got := out.String(); !strings.Contains(got, "Unchanged") {
		t.Errorf("re-running the modify wrote the service:\n%s", got)
	}
}

func TestIssue1608_AlterDropAndSet(t *testing.T) {
	exec, out := restProject(t)
	run(t, exec, bookingAPIStep1)
	run(t, exec, `alter consumed rest service MyFirstModule.BookingApi
  add operation GetBooking (Method: get, Path: '/bookings/{id}', Parameters: ($id: String), Response: json),
  set (BaseUrl: 'https://api.example.com/v2');`)
	if got := restService(t, exec, "BookingApi").BaseUrl; got != "https://api.example.com/v2" {
		t.Errorf("BaseUrl = %q", got)
	}

	run(t, exec, `alter consumed rest service MyFirstModule.BookingApi drop operation PostBooking;`)
	if ops := restOperations(t, exec, "BookingApi"); strings.Join(ops, ",") != "GetBooking" {
		t.Errorf("operations after drop = %v", ops)
	}

	// A name that is not there is refused, listing the ones that are.
	prog, errs := visitor.Build(`alter consumed rest service MyFirstModule.BookingApi drop operation PostBooking;`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	err := exec.Execute(prog.Statements[0])
	if err == nil || !strings.Contains(err.Error(), "GetBooking") {
		t.Errorf("dropping a missing operation: %v", err)
	}

	// describe still runs again after the edits.
	out.Reset()
	run(t, exec, "describe consumed rest service MyFirstModule.BookingApi")
	described := out.String()
	out.Reset()
	run(t, exec, described)
	if got := out.String(); !strings.Contains(got, "Unchanged") {
		t.Errorf("describe after alter does not re-run unchanged:\n%s\n%s", described, got)
	}
}

// `drop operation X, add operation X (…)` in one statement replaces X.
func TestIssue1608_AlterDropThenAddReplaces(t *testing.T) {
	exec, _ := restProject(t)
	run(t, exec, bookingAPIStep1)
	run(t, exec, `alter consumed rest service MyFirstModule.BookingApi
  drop operation PostBooking,
  add operation PostBooking (Method: post, Path: '/v2/bookings', Body: json from $Payload, Response: none);`)
	svc := restService(t, exec, "BookingApi")
	if len(svc.Operations) != 1 || svc.Operations[0].Path != "/v2/bookings" || svc.Operations[0].ResponseType != "NONE" {
		t.Errorf("operations = %+v", svc.Operations)
	}
}
