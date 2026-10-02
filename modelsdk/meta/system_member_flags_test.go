// SPDX-License-Identifier: Apache-2.0

// The System entities' stored system members (owner / changedBy / createdDate /
// changedDate). They were never declared, so every check that asked "does this
// entity store System.owner?" answered no for anything built on a System entity
// — `[System.owner = '[%CurrentUser%]']` on a FileDocument specialization was
// refused by `check --references` while mxbuild 11.14.0 built it clean.
//
// Measured, like the string lengths, from the System module's domain model in a
// deployed `deployment/model/model.mdp`: each root entity's NoGeneralization
// carries HasOwnerAttr / HasChangedByAttr / HasCreatedDateAttr /
// HasChangedDateAttr. Re-measure with
//
//	go test ./modelsdk/meta -run TestSystemMemberFlagsMatchTheDeployedModel -mdp <model.mdp>
package meta

import (
	"encoding/binary"
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestSystemEntityStoresMember(t *testing.T) {
	cases := []struct {
		entity, member string
		want           bool
	}{
		{"FileDocument", SystemMemberOwner, true},
		{"FileDocument", SystemMemberChangedBy, true},
		{"FileDocument", SystemMemberCreatedDate, true},
		{"FileDocument", SystemMemberChangedDate, true},
		// Inherited: System.Image declares no flags of its own.
		{"System.Image", SystemMemberOwner, true},
		{"Image", SystemMemberChangedDate, true},
		{"SynchronizationErrorFile", SystemMemberChangedBy, true},
		{"User", SystemMemberOwner, true},
		{"User", SystemMemberChangedDate, true},
		{"Workflow", SystemMemberOwner, true},
		{"Workflow", SystemMemberChangedBy, false},
		{"QueuedTask", SystemMemberOwner, true},
		{"SynchronizationError", SystemMemberCreatedDate, true},
		{"SynchronizationError", SystemMemberChangedDate, false},
		{"Session", SystemMemberCreatedDate, true},
		{"Session", SystemMemberOwner, false},
		{"UserRole", SystemMemberOwner, false},
		{"UserRole", SystemMemberCreatedDate, false},
	}
	for _, c := range cases {
		got, known := SystemEntityStoresMember(c.entity, c.member)
		if !known {
			t.Errorf("%s: not known", c.entity)
			continue
		}
		if got != c.want {
			t.Errorf("SystemEntityStoresMember(%s, %s) = %v, want %v", c.entity, c.member, got, c.want)
		}
	}
	if _, known := SystemEntityStoresMember("NoSuchEntity", SystemMemberOwner); known {
		t.Error("an unknown System entity must report known=false, not a no")
	}
}

// A specialization's flags are its root's. Declaring them on a specialization
// as well would be a second copy to drift; the walk ignores them anyway.
func TestSystemMemberFlags_OnlyOnRoots(t *testing.T) {
	for _, e := range SystemEntities {
		if e.Generalization == "" {
			continue
		}
		if e.HasOwner || e.HasChangedBy || e.HasCreatedDate || e.HasChangedDate {
			t.Errorf("System.%s extends %s and declares system-member flags; they are inherited from the root", e.Name, e.Generalization)
		}
	}
}

func TestSystemMemberFlagsMatchTheDeployedModel(t *testing.T) {
	if *mdpPath == "" {
		t.Skip("no -mdp given; this test re-measures from a built deployment/model/model.mdp")
	}
	raw, err := os.ReadFile(*mdpPath)
	if err != nil {
		t.Fatalf("read %s: %v", *mdpPath, err)
	}
	measured := systemMemberFlagsFromMDP(raw)
	if len(measured) == 0 {
		t.Fatalf("%s carries no System module domain model", *mdpPath)
	}
	for _, e := range SystemEntities {
		if e.Generalization != "" {
			continue
		}
		m, ok := measured[e.Name]
		if !ok {
			t.Errorf("System.%s is declared but absent from %s", e.Name, *mdpPath)
			continue
		}
		got := [4]bool{e.HasOwner, e.HasChangedBy, e.HasCreatedDate, e.HasChangedDate}
		if got != m {
			t.Errorf("System.%s: declared owner/changedBy/createdDate/changedDate = %v, deployed model says %v", e.Name, got, m)
		}
	}
}

// systemMemberFlagsFromMDP returns entity name -> [owner, changedBy,
// createdDate, changedDate] for the System module's root entities.
func systemMemberFlagsFromMDP(raw []byte) map[string][4]bool {
	out := map[string][4]bool{}
	inSystem := false
	for off := 0; off+4 <= len(raw); {
		size := int(binary.LittleEndian.Uint32(raw[off : off+4]))
		if size < 5 || off+size > len(raw) {
			break
		}
		var doc bson.M
		if err := bson.Unmarshal(raw[off:off+size], &doc); err == nil {
			switch ty, _ := doc["$Type"].(string); ty {
			case "Projects$ModuleImpl":
				name, _ := doc["Name"].(string)
				inSystem = name == "System"
			case "DomainModels$DomainModel":
				if inSystem {
					entities, _ := doc["Entities"].(bson.A)
					for _, ev := range entities {
						ent, _ := ev.(bson.M)
						name, _ := ent["UnqualifiedName"].(string)
						g, _ := ent["MaybeGeneralization"].(bson.M)
						if g == nil {
							g, _ = ent["Generalization"].(bson.M)
						}
						if ty, _ := g["$Type"].(string); ty != "DomainModels$NoGeneralization" {
							continue
						}
						b := func(k string) bool { v, _ := g[k].(bool); return v }
						out[name] = [4]bool{b("HasOwnerAttr"), b("HasChangedByAttr"), b("HasCreatedDateAttr"), b("HasChangedDateAttr")}
					}
					inSystem = false
				}
			}
		}
		off += size
	}
	return out
}
