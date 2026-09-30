// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestExtractReferencedEntities(t *testing.T) {
	tests := []struct {
		name  string
		xpath string
		want  string
	}{
		{
			"simple association",
			"[Module.Order_Customer = $Customer]",
			"Module.Order_Customer",
		},
		{
			"multi-hop path",
			"[Module.Order_Customer/Module.Customer/Name = $val]",
			"Module.Order_Customer,Module.Customer",
		},
		{
			"no qualified names",
			"[IsActive = true]",
			"",
		},
		{
			"system owner with quoted token",
			"[System.owner = '[%CurrentUser%]']",
			"System.owner",
		},
		{
			"multiple associations",
			"[Module.A/Module.B and Module.C/Module.D]",
			"Module.A,Module.B,Module.C,Module.D",
		},
		{
			"qualified name inside string literal ignored",
			"[Name = 'Module.Value']",
			"",
		},
		{
			"mixed",
			"[Module.Assoc/Module.Entity/Attr = 'test' and System.owner = '[%CurrentUser%]']",
			"Module.Assoc,Module.Entity,System.owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractReferencedEntities(tt.xpath)
			if got != tt.want {
				t.Errorf("extractReferencedEntities(%q) = %q, want %q", tt.xpath, got, tt.want)
			}
		})
	}
}

func TestContainsVariable(t *testing.T) {
	tests := []struct {
		xpath string
		want  bool
	}{
		{"[Name = $Username]", true},
		{"[IsActive = true]", false},
		{"[System.owner = '[%CurrentUser%]']", false},
		{"[$var/Attr >= $other]", true},
	}

	for _, tt := range tests {
		t.Run(tt.xpath, func(t *testing.T) {
			got := containsVariable(tt.xpath)
			if got != tt.want {
				t.Errorf("containsVariable(%q) = %v, want %v", tt.xpath, got, tt.want)
			}
		})
	}
}

func TestResolveEntityRefFromBSON(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			"with EntityRef map",
			map[string]any{
				"EntityRef": map[string]any{"QualifiedName": "Module.Entity"},
			},
			"Module.Entity",
		},
		// The shapes Studio Pro stores, as buildXPathExpressions decodes them
		// (the v1 driver, so nested documents are bson.D). Only QualifiedName was read, which no
		// stored EntityRef carries, so every page/snippet XPath constraint was
		// recorded with an empty TargetEntity (Evora: 52 of 52) and the bare
		// attribute names in it could not be resolved.
		{
			"stored DirectEntityRef",
			map[string]any{
				"EntityRef": bson.D{
					{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
					{Key: "Entity", Value: "Module.Entity"},
				},
			},
			"Module.Entity",
		},
		{
			"stored IndirectEntityRef ends on the last step's entity",
			map[string]any{
				"EntityRef": bson.D{
					{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
					{Key: "Steps", Value: bson.A{int32(2),
						bson.D{
							{Key: "$Type", Value: "DomainModels$EntityRefStep"},
							{Key: "Association", Value: "Module.A_B"},
							{Key: "DestinationEntity", Value: "Module.B"},
						},
						bson.D{
							{Key: "$Type", Value: "DomainModels$EntityRefStep"},
							{Key: "Association", Value: "Module.B_C"},
							{Key: "DestinationEntity", Value: "Module.C"},
						},
					}},
				},
			},
			"Module.C",
		},
		{
			"no EntityRef",
			map[string]any{"Name": "test"},
			"",
		},
		{
			"empty EntityRef",
			map[string]any{"EntityRef": map[string]any{}},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEntityRefFromBSON(tt.raw)
			if got != tt.want {
				t.Errorf("resolveEntityRefFromBSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestXpathID(t *testing.T) {
	// Deterministic: same input -> same output
	id1 := xpathID("doc1", "comp1", "[Active = true]")
	id2 := xpathID("doc1", "comp1", "[Active = true]")
	if id1 != id2 {
		t.Errorf("xpathID not deterministic: %q != %q", id1, id2)
	}

	// Different inputs -> different outputs
	id3 := xpathID("doc1", "comp1", "[Active = false]")
	if id1 == id3 {
		t.Errorf("xpathID collision: both returned %q", id1)
	}

	// 32 hex characters (16 bytes)
	if len(id1) != 32 {
		t.Errorf("xpathID length = %d, want 32", len(id1))
	}
}
