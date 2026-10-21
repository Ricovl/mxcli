// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1600 (ako/mxcli#1510): `describe entity` printed
// `persistent entity Probe.Child extends Probe.Base` for a specialization of a
// NON-persistent Probe.Base. A specialization stores no Persistable flag of its
// own — its MaybeGeneralization is a DomainModels$Generalization, which has no
// such property — so the flag is its root's. entityFromGen defaulted it to true
// and only the NoGeneralization arm ever overwrote it, so every specialization
// read as persistent, whatever its chain said. The same flag feeds
// `show entities where non-persistent`, the catalog, the diagrams, and the
// microflow builder's persistability map.
package modelsdkbackend

import (
	"testing"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func TestReadResolvesPersistabilityFromTheRootGeneralization(t *testing.T) {
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
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}

	// Persistable on a specialization is what the executor's statement says —
	// `create persistent entity … extends` — and the writer drops it, because
	// the Generalization element has nowhere to store it. Declare every
	// specialization persistent, so a reader that returns the declared value (or
	// the old default) fails on the non-persistent chains.
	str := func(n string) []*domainmodel.Attribute {
		return []*domainmodel.Attribute{{Name: n, Type: &domainmodel.StringAttributeType{Length: 20}}}
	}
	for _, e := range []*domainmodel.Entity{
		{Name: "ZzNpBase", Persistable: false, Attributes: str("Code")},
		{Name: "ZzNpChild", Persistable: true, GeneralizationRef: "MyFirstModule.ZzNpBase", Attributes: str("Status")},
		{Name: "ZzNpGrandChild", Persistable: true, GeneralizationRef: "MyFirstModule.ZzNpChild", Attributes: str("Extra")},
		{Name: "ZzPBase", Persistable: true, Attributes: str("Code")},
		{Name: "ZzPChild", Persistable: true, GeneralizationRef: "MyFirstModule.ZzPBase", Attributes: str("Amount")},
		// System.HttpResponse extends the non-persistent System.HttpMessage;
		// System.Image extends the persistent System.FileDocument. The chain
		// crosses into the virtual System module, which is not stored.
		{Name: "ZzResponse", Persistable: true, GeneralizationRef: "System.HttpResponse", Attributes: str("Note")},
		{Name: "ZzPicture", Persistable: true, GeneralizationRef: "System.Image", Attributes: str("Caption")},
	} {
		if err := b.CreateEntity(dm.ID, e); err != nil {
			t.Fatalf("CreateEntity %s: %v", e.Name, err)
		}
	}

	want := map[string]bool{
		"ZzNpBase":       false,
		"ZzNpChild":      false,
		"ZzNpGrandChild": false,
		"ZzPBase":        true,
		"ZzPChild":       true,
		"ZzResponse":     false,
		"ZzPicture":      true,
	}
	check := func(read string, entities []*domainmodel.Entity) {
		t.Helper()
		seen := 0
		for _, e := range entities {
			w, ok := want[e.Name]
			if !ok {
				continue
			}
			seen++
			if e.Persistable != w {
				t.Errorf("%s: %s reads Persistable=%v, want %v (its root generalization's)", read, e.Name, e.Persistable, w)
			}
		}
		if seen != len(want) {
			t.Fatalf("%s: found %d of the %d probe entities", read, seen, len(want))
		}
	}

	got, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	check("GetDomainModel", got.Entities)

	got, err = b.GetDomainModelByID(dm.ID)
	if err != nil {
		t.Fatalf("GetDomainModelByID: %v", err)
	}
	check("GetDomainModelByID", got.Entities)

	all, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range all {
		if d.ContainerID == mod.ID {
			check("ListDomainModels", d.Entities)
		}
		// The System module's own specializations follow the same rule: the
		// HTTP messages and the SOAP fault are non-persistent like their roots.
		for _, e := range d.Entities {
			switch e.Name {
			case "HttpRequest", "HttpResponse", "SoapFault":
				if e.Persistable {
					t.Errorf("ListDomainModels: System.%s reads persistent; its root is not", e.Name)
				}
			}
		}
	}
}

// The resolved flag is a read of the chain, not a property of the stored
// specialization. Comparing it with entityFromGen's default would call every
// specialization of a non-persistent entity changed, and rebuild it on every
// UpdateDomainModel — the churn that a statement which names one association
// already paid for once (ako/mxcli#801).
func TestEntityUnchanged_IgnoresInheritedPersistability(t *testing.T) {
	stored := genDm.NewEntity()
	stored.SetName("Child")
	g := genDm.NewGeneralization()
	g.SetGeneralizationQualifiedName("M.Base")
	stored.SetGeneralization(g)

	read := entityFromGen(stored)
	read.Persistable = false // what the chain resolves to when M.Base is non-persistent
	if !entityUnchanged(read, stored) {
		t.Error("a specialization read back unchanged compares as changed on its inherited Persistable")
	}

	// Control: a root stores its flag, so a different one is a real change.
	root := genDm.NewEntity()
	root.SetName("Base")
	ng := genDm.NewNoGeneralization()
	ng.SetPersistable(true)
	root.SetGeneralization(ng)
	flipped := entityFromGen(root)
	flipped.Persistable = false
	if entityUnchanged(flipped, root) {
		t.Error("a root entity whose Persistable flipped compares as unchanged")
	}
}
