// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// TestCreatePublishedRestService_RoundTrip creates a published REST service with
// one resource and two operations (one with a path parameter), then confirms it
// round-trips through ListPublishedRestServices.
func TestCreatePublishedRestService_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	svc := &model.PublishedRestService{
		ContainerID: mod.ID,
		Name:        "ZzApi",
		Path:        "rest/zz/v1",
		Version:     "1.0.0",
		ServiceName: "Zz API",
		Resources: []*model.PublishedRestResource{{
			Name: "items",
			Operations: []*model.PublishedRestOperation{
				{Path: "", HTTPMethod: "GET", Microflow: "MyFirstModule.ACT_List"},
				{Path: "{id}", HTTPMethod: "GET", Microflow: "MyFirstModule.ACT_Get"},
			},
		}},
	}
	if err := b.CreatePublishedRestService(svc); err != nil {
		t.Fatalf("CreatePublishedRestService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListPublishedRestServices()
	if err != nil {
		t.Fatalf("ListPublishedRestServices: %v", err)
	}
	var got *model.PublishedRestService
	for _, s := range all {
		if s.Name == "ZzApi" {
			got = s
			break
		}
	}
	if got == nil {
		t.Fatalf("ZzApi not found after create")
	}
	if got.Path != "rest/zz/v1" || got.Version != "1.0.0" {
		t.Errorf("service header not round-tripped: %+v", got)
	}
	if len(got.Resources) != 1 || got.Resources[0].Name != "items" {
		t.Fatalf("resource not round-tripped: %+v", got.Resources)
	}
	if len(got.Resources[0].Operations) != 2 {
		t.Errorf("operations = %d, want 2", len(got.Resources[0].Operations))
	}
}

// TestCreatePublishedRestService_WritesParametersAndBindings is ako/mxcli#571 /
// mendixlabs/mxcli#1206 at the storage layer: the operation's query and body
// parameters, each with its own type, its mapping bindings and its commit
// option are written and read back. Before, only String path parameters were
// written, and ExportMapping / ImportMapping / Commit were constants.
func TestCreatePublishedRestService_WritesParametersAndBindings(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	params := []*model.PublishedRestOperationParameter{
		{Name: "id", ParameterType: "Path", MicroflowParameter: "MyFirstModule.ACT_Put.id", DataType: "Integer"},
		{Name: "verbose", ParameterType: "Query", MicroflowParameter: "MyFirstModule.ACT_Put.verbose", DataType: "Boolean"},
		{Name: "X-Mode", ParameterType: "Header", MicroflowParameter: "MyFirstModule.ACT_Put.mode", DataType: "Enumeration", QualifiedName: "MyFirstModule.Mode", Description: "mode"},
		{Name: "body", ParameterType: "Body", MicroflowParameter: "MyFirstModule.ACT_Put.body", DataType: "Object", QualifiedName: "MyFirstModule.Thing"},
	}
	svc := &model.PublishedRestService{
		ContainerID: mod.ID, Name: "ZzParams", Path: "rest/zzp/v1",
		Resources: []*model.PublishedRestResource{{
			Name: "things",
			Operations: []*model.PublishedRestOperation{
				{
					Path: "{id}", HTTPMethod: "PUT", Microflow: "MyFirstModule.ACT_Put",
					ImportMapping: "MyFirstModule.IMM_Thing", ExportMapping: "MyFirstModule.EMM_Thing",
					Commit: "YesWithoutEvents", ObjectHandlingBackup: "Error",
					OperationParameters: params,
				},
				// No derived parameters: the path's placeholder as a String (the
				// writer's fallback when the microflow could not be read).
				{Path: "{key}", HTTPMethod: "GET", Microflow: "MyFirstModule.ACT_Get"},
			},
		}},
	}
	if err := b.CreatePublishedRestService(svc); err != nil {
		t.Fatalf("CreatePublishedRestService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	all, err := b2.ListPublishedRestServices()
	if err != nil {
		t.Fatalf("ListPublishedRestServices: %v", err)
	}
	var got *model.PublishedRestService
	for _, s := range all {
		if s.Name == "ZzParams" {
			got = s
		}
	}
	if got == nil {
		t.Fatal("ZzParams not found")
	}
	op := got.Resources[0].Operations[0]
	if op.ImportMapping != "MyFirstModule.IMM_Thing" || op.ExportMapping != "MyFirstModule.EMM_Thing" ||
		op.Commit != "YesWithoutEvents" || op.ObjectHandlingBackup != "Error" {
		t.Errorf("bindings read back as import %q export %q commit %q handling %q",
			op.ImportMapping, op.ExportMapping, op.Commit, op.ObjectHandlingBackup)
	}
	if len(op.OperationParameters) != len(params) {
		t.Fatalf("parameters = %d, want %d: %+v", len(op.OperationParameters), len(params), op.OperationParameters)
	}
	for i, want := range params {
		if g := *op.OperationParameters[i]; g != *want {
			t.Errorf("parameter %d = %+v, want %+v", i, g, *want)
		}
	}
	fallback := got.Resources[0].Operations[1]
	if len(fallback.OperationParameters) != 1 || *fallback.OperationParameters[0] != (model.PublishedRestOperationParameter{
		Name: "key", ParameterType: "Path", MicroflowParameter: "MyFirstModule.ACT_Get.key", DataType: "String",
	}) {
		t.Errorf("fallback parameters = %+v", fallback.OperationParameters)
	}
	if fallback.Commit != "Yes" || fallback.ObjectHandlingBackup != "Create" {
		t.Errorf("defaults = commit %q handling %q, want Yes / Create", fallback.Commit, fallback.ObjectHandlingBackup)
	}
}

// TestWithStoredTopLevel carries the stored values of the listed keys,
// inserts a stored-only key in sorted position, and leaves every other key as
// written. The control: an unlisted key keeps the written value.
func TestWithStoredTopLevel(t *testing.T) {
	written, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: "x"}, {Key: "AuthenticationTypes", Value: bson.A{int32(1)}},
		{Key: "Name", Value: "new"}, {Key: "Version", Value: "2"},
	})
	stored, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: "x"}, {Key: "AuthenticationTypes", Value: bson.A{int32(1), "Basic"}},
		{Key: "Name", Value: "old"}, {Key: "PublicDocumentation", Value: ""}, {Key: "Version", Value: "1"},
	})
	out, err := withStoredTopLevel(written, stored, []string{"AuthenticationTypes", "PublicDocumentation", "Missing"})
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, e := range d {
		keys = append(keys, e.Key)
	}
	if got := strings.Join(keys, ","); got != "$ID,AuthenticationTypes,Name,PublicDocumentation,Version" {
		t.Errorf("keys = %s", got)
	}
	m := d.Map()
	if a, ok := m["AuthenticationTypes"].(bson.A); !ok || len(a) != 2 || a[1] != "Basic" {
		t.Errorf("AuthenticationTypes = %v, want the stored [1 Basic]", m["AuthenticationTypes"])
	}
	if m["Name"] != "new" || m["Version"] != "2" {
		t.Errorf("unlisted keys changed: Name %v Version %v", m["Name"], m["Version"])
	}
}
