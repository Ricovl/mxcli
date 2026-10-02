// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestLive_CreateEntityAttributeTypes is the live proof for #923: CREATE ENTITY
// must store each attribute with its declared type and String length. Before the
// fix every attribute came back String(200) on both the 11.14 and 11.15 servers,
// because the entity constructor's attributes carried a `$Type` that PED reads as
// "drop the type", and no length was ever sent.
//
// The entity (and its enumeration) are left in place, uniquely named
// Zz_R12_AttrTypes_<stamp>, so a run can be inspected in Studio Pro.
//
//	MXCLI_MCP_URL=http://localhost/mcp MXCLI_MCP_DIAL=host.docker.internal:7793 \
//	MXCLI_MCP_MODULE=MyFirstModule \
//	go test ./mdl/backend/mcp/ -run TestLive_CreateEntityAttributeTypes -v
func TestLive_CreateEntityAttributeTypes(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	if url == "" {
		t.Skip("set MXCLI_MCP_URL to run the live MCP integration test")
	}
	module := envOr("MXCLI_MCP_MODULE", "MyFirstModule")
	c, err := NewClient(ClientOptions{URL: url, Dial: os.Getenv("MXCLI_MCP_DIAL")})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	si, err := c.Initialize()
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Logf("connected to %s %s", si.Name, si.Version)
	b := &Backend{client: c}

	stamp := time.Now().Format("0102150405")
	enumName := "Zz_R12_AttrEnum_" + stamp
	entName := "Zz_R12_AttrTypes_" + stamp
	enumRef := module + "." + enumName

	if err := b.ensureSchema(enumerationDocType); err != nil {
		t.Fatalf("ensureSchema enum: %v", err)
	}
	enum := &model.Enumeration{Name: enumName, Values: []model.EnumerationValue{{Name: "Draft"}, {Name: "Done"}}}
	if err := b.pedCreateDocument(module, enumerationDocType, enumName, buildEnumContent(enum), ""); err != nil {
		t.Fatalf("create enumeration %s: %v", enumRef, err)
	}

	attrs := []*domainmodel.Attribute{
		attr("S50", &domainmodel.StringAttributeType{Length: 50}),
		attr("S200", &domainmodel.StringAttributeType{Length: 200}),
		attr("SUnl", &domainmodel.StringAttributeType{Length: 0}),
		attr("I", &domainmodel.IntegerAttributeType{}),
		attr("L", &domainmodel.LongAttributeType{}),
		attr("D", &domainmodel.DecimalAttributeType{}),
		attr("B", &domainmodel.BooleanAttributeType{}),
		attr("DT", &domainmodel.DateTimeAttributeType{LocalizeDate: true}),
		attr("E", &domainmodel.EnumerationAttributeType{EnumerationRef: enumRef}),
		attr("AN", &domainmodel.AutoNumberAttributeType{}),
		attr("HS", &domainmodel.HashedStringAttributeType{}),
		attr("Bin", &domainmodel.BinaryAttributeType{}),
	}
	entity := newPersistentEntity(entName, attrs...)
	if err := b.CreateEntity(model.ID(sessionDMPrefix+module), entity); err != nil {
		t.Fatalf("CreateEntity %s.%s: %v", module, entName, err)
	}
	t.Logf("created %s.%s (left in place)", module, entName)

	got := liveAttrTypes(t, b, module, entName)
	for _, a := range attrs {
		if want := describeAttrType(a.Type); got[a.Name] != want {
			t.Errorf("attribute %s stored as %s, want %s", a.Name, got[a.Name], want)
		}
	}
	t.Logf("read back: %v", got)

	// ALTER ENTITY … ADD ATTRIBUTE kept its type before the fix, but never sent a
	// length either.
	entID := model.ID("mcp~ent~" + module + "~" + entName)
	b.registerSynthetic(entID, entName)
	added := attr("Added40", &domainmodel.StringAttributeType{Length: 40})
	if err := b.AddAttribute(model.ID(sessionDMPrefix+module), entID, added); err != nil {
		t.Fatalf("AddAttribute: %v", err)
	}
	if g := liveAttrTypes(t, b, module, entName)["Added40"]; g != "String(40)" {
		t.Errorf("added attribute stored as %s, want String(40)", g)
	}
}

// liveAttrTypes reads an entity's attributes back over PED, keyed by name.
func liveAttrTypes(t *testing.T, b *Backend, module, entName string) map[string]string {
	t.Helper()
	idx, err := b.entityIndex(module, entName)
	if err != nil {
		t.Fatalf("entity %s not found: %v", entName, err)
	}
	names, err := b.liveAttributeNames(module, idx)
	if err != nil {
		t.Fatalf("liveAttributeNames: %v", err)
	}
	_, live, err := b.liveEntityDetails(module, idx, len(names))
	if err != nil {
		t.Fatalf("liveEntityDetails: %v", err)
	}
	got := map[string]string{}
	for i, n := range names {
		got[n] = describeAttrType(live[i].typ)
	}
	return got
}

// describeAttrType renders an attribute type with the facets #923 is about
// (String length, enumeration reference) for comparison.
func describeAttrType(at domainmodel.AttributeType) string {
	switch v := at.(type) {
	case nil:
		return "<nil>"
	case *domainmodel.StringAttributeType:
		return fmt.Sprintf("String(%d)", v.Length)
	case *domainmodel.EnumerationAttributeType:
		return "Enumeration(" + v.EnumerationRef + ")"
	default:
		return at.GetTypeName()
	}
}
