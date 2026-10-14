// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// signatureOf is what the signature listings promise to keep of a flow.
type signatureOf struct {
	Name               string
	ContainerID        model.ID
	Excluded           bool
	AllowedModuleRoles []model.ID
	Parameters         []*microflows.MicroflowParameter
	ReturnType         microflows.DataType
}

// TestFlowSignatures_MatchFullDecode pins the body-stripped decode to the full
// one for every microflow and nanoflow of the fixture.
func TestFlowSignatures_MatchFullDecode(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	full := map[model.ID]signatureOf{}
	params := 0
	mfs, err := b.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range mfs {
		full[f.ID] = signatureOf{f.Name, f.ContainerID, f.Excluded, f.AllowedModuleRoles, f.Parameters, f.ReturnType}
		params += len(f.Parameters)
	}
	nfs, err := b.ListNanoflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range nfs {
		full[f.ID] = signatureOf{f.Name, f.ContainerID, f.Excluded, f.AllowedModuleRoles, f.Parameters, f.ReturnType}
		params += len(f.Parameters)
	}
	if len(nfs) == 0 || params == 0 {
		t.Fatalf("fixture has %d nanoflows and %d parameters; the comparison would check little", len(nfs), params)
	}

	got := map[model.ID]signatureOf{}
	smfs, err := b.ListMicroflowSignatures()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range smfs {
		got[f.ID] = signatureOf{f.Name, f.ContainerID, f.Excluded, f.AllowedModuleRoles, f.Parameters, f.ReturnType}
	}
	snfs, err := b.ListNanoflowSignatures()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range snfs {
		got[f.ID] = signatureOf{f.Name, f.ContainerID, f.Excluded, f.AllowedModuleRoles, f.Parameters, f.ReturnType}
	}

	if len(got) != len(full) {
		t.Errorf("signature listings returned %d flows, the full decode %d", len(got), len(full))
	}
	for id, want := range full {
		if g, ok := got[id]; !ok {
			t.Errorf("%s (%s) missing from the signature listings", want.Name, id)
		} else if !reflect.DeepEqual(g, want) {
			t.Errorf("%s: signature listing\n  %+v\nfull decode\n  %+v", want.Name, g, want)
		}
	}
}
