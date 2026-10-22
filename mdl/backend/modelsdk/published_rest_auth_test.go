// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
)

// storedAuth reads the two authentication fields as stored.
func storedAuth(t *testing.T, b *Backend, id model.ID) (bson.A, string) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("raw unit: %v", err)
	}
	var d struct {
		AuthenticationTypes     bson.A `bson:"AuthenticationTypes"`
		AuthenticationMicroflow string `bson:"AuthenticationMicroflow"`
	}
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d.AuthenticationTypes, d.AuthenticationMicroflow
}

// TestPublishedRestService_WritesAuthentication pins the writer to what Studio
// Pro stores for each dialog state, measured on ako/TestApp's Services module
// (mendixlabs/mxcli#1331): a marker-1 list in the order the methods were
// ticked, the microflow by qualified name, and the empty list with an empty
// microflow for "no authentication". Each update states the setting; the
// carry of an unstated one is the executor's (cmd_published_rest.go).
func TestPublishedRestService_WritesAuthentication(t *testing.T) {
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
		ContainerID: mod.ID, Name: "ZzAuth", Path: "rest/zza/v1",
		// TestApp's OrdersRestApi_MicroflowSession: Custom ticked first.
		AuthenticationTypes:     []string{"Microflow", "Session"},
		AuthenticationMicroflow: "MyFirstModule.ACT_Auth",
	}
	if err := b.CreatePublishedRestService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}

	steps := []struct {
		name      string
		types     []string
		mf        string
		wantTypes bson.A
	}{
		{"created", svc.AuthenticationTypes, svc.AuthenticationMicroflow, bson.A{int32(1), "Microflow", "Session"}},
		// OrdersRestApi_CustomOff: Custom unticked, microflow cleared.
		{"custom off", []string{"Basic", "Session"}, "", bson.A{int32(1), "Basic", "Session"}},
		// OrdersRestApi_NoAuth.
		{"none", nil, "", bson.A{int32(1)}},
	}
	for i, st := range steps {
		if i > 0 {
			svc.AuthenticationTypes, svc.AuthenticationMicroflow = st.types, st.mf
			if err := b.UpdatePublishedRestService(svc); err != nil {
				t.Fatalf("%s: update: %v", st.name, err)
			}
		}
		types, mf := storedAuth(t, b, svc.ID)
		if !reflect.DeepEqual(types, st.wantTypes) || mf != st.mf {
			t.Errorf("%s: stored %v / %q, want %v / %q", st.name, types, mf, st.wantTypes, st.mf)
		}
		all, err := b.ListPublishedRestServices()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, s := range all {
			if s.Name == "ZzAuth" && (!reflect.DeepEqual(s.AuthenticationTypes, st.types) && len(s.AuthenticationTypes)+len(st.types) > 0 || s.AuthenticationMicroflow != st.mf) {
				t.Errorf("%s: read back %v / %q, want %v / %q", st.name, s.AuthenticationTypes, s.AuthenticationMicroflow, st.types, st.mf)
			}
		}
	}
}

// storedServiceParameters reads the service-level Parameters list as stored:
// its marker, and each element's keys and $ID.
func storedServiceParameters(t *testing.T, b *Backend, id model.ID) (marker int32, keys [][]string, ids []string) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("raw unit: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range d {
		if e.Key != "Parameters" {
			continue
		}
		arr, _ := e.Value.(bson.A)
		if len(arr) == 0 {
			t.Fatalf("Parameters = %v, want a marked list", e.Value)
		}
		marker, _ = arr[0].(int32)
		for _, el := range arr[1:] {
			ed, _ := el.(bson.D)
			var ks []string
			for _, f := range ed {
				ks = append(ks, f.Key)
				if f.Key == "$ID" {
					if bin, ok := f.Value.(bson.Binary); ok {
						ids = append(ids, fmt.Sprintf("%x", bin.Data))
					}
				}
			}
			sort.Strings(ks)
			keys = append(keys, ks)
		}
		return marker, keys, ids
	}
	t.Fatal("no Parameters key")
	return 0, nil, nil
}

// TestPublishedRestService_WritesAuthenticationParameters is
// mendixlabs/mxcli#1417 at the storage layer: the service's Parameters are
// written from the model — Studio Pro's property set, measured on the Siemens
// FSM template's NativeRestAPI.DocumentShare (Description, MicroflowParameter,
// Name, ParameterType, Type) — and read back. A rewrite that states a
// parameter's stored $ID keeps it on the element bound to the same microflow
// parameter, also after a reorder.
//
// Before, the writer carried the stored list over unread: the model had no
// field for it, so a create wrote [3] and the read-back below found nothing.
func TestPublishedRestService_WritesAuthenticationParameters(t *testing.T) {
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
		{Name: "jwt", ParameterType: "Query", MicroflowParameter: "MyFirstModule.ACT_Auth.Token", DataType: "String"},
		{Name: "X-Tenant", ParameterType: "Header", MicroflowParameter: "MyFirstModule.ACT_Auth.Tenant", DataType: "Integer", Description: "tenant id"},
	}
	svc := &model.PublishedRestService{
		ContainerID: mod.ID, Name: "ZzAuthParams", Path: "rest/zzap/v1",
		AuthenticationTypes:     []string{"Microflow"},
		AuthenticationMicroflow: "MyFirstModule.ACT_Auth",
		Parameters:              params,
	}
	if err := b.CreatePublishedRestService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}
	marker, keys, ids := storedServiceParameters(t, b, svc.ID)
	wantKeys := []string{"$ID", "$Type", "Description", "MicroflowParameter", "Name", "ParameterType", "Type"}
	if marker != 3 || len(keys) != 2 || !reflect.DeepEqual(keys[0], wantKeys) || !reflect.DeepEqual(keys[1], wantKeys) {
		t.Fatalf("stored marker %d keys %v, want 3 and two of %v", marker, keys, wantKeys)
	}

	readBack := func() []*model.PublishedRestOperationParameter {
		t.Helper()
		all, err := b.ListPublishedRestServices()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for _, s := range all {
			if s.Name == "ZzAuthParams" {
				return s.Parameters
			}
		}
		t.Fatal("ZzAuthParams not found")
		return nil
	}
	got := readBack()
	if len(got) != len(params) {
		t.Fatalf("read back %d parameters, want %d: %+v", len(got), len(params), got)
	}
	for i, want := range params {
		g := *got[i]
		if g.ID == "" {
			t.Errorf("parameter %d read back without its $ID", i)
		}
		g.ID = ""
		if g != *want {
			t.Errorf("parameter %d = %+v, want %+v", i, g, *want)
		}
	}

	// Reorder, carrying each element's stored $ID with it.
	svc.Parameters = []*model.PublishedRestOperationParameter{got[1], got[0]}
	if err := b.UpdatePublishedRestService(svc); err != nil {
		t.Fatalf("update: %v", err)
	}
	_, _, ids2 := storedServiceParameters(t, b, svc.ID)
	if len(ids2) != 2 || ids2[0] != ids[1] || ids2[1] != ids[0] {
		t.Errorf("after reorder stored $IDs %v, want %v reversed", ids2, ids)
	}
	if again := readBack(); len(again) != 2 || again[0].MicroflowParameter != params[1].MicroflowParameter {
		t.Errorf("after reorder read back %+v", again)
	}

	// No parameters is the empty marked list Studio Pro writes.
	svc.Parameters = nil
	if err := b.UpdatePublishedRestService(svc); err != nil {
		t.Fatalf("update: %v", err)
	}
	if marker, keys, _ := storedServiceParameters(t, b, svc.ID); marker != 3 || len(keys) != 0 {
		t.Errorf("cleared: marker %d, %d elements, want [3]", marker, len(keys))
	}
}
