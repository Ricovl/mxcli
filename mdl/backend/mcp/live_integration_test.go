// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"os"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestLive_EntityRoundTrip exercises the real PED entity choreography against a
// running Studio Pro MCP server. It is skipped unless MXCLI_MCP_URL is set.
//
// Example (Studio Pro open, project has an empty user module "MyFirstModule"):
//
//	MXCLI_MCP_URL=http://localhost/mcp \
//	MXCLI_MCP_DIAL=host.docker.internal:7782 \
//	MXCLI_MCP_MODULE=MyFirstModule \
//	go test ./mdl/backend/mcp/ -run TestLive -v
//
// The test creates a uniquely-named entity, validates it, reads it back, and
// removes it, leaving the model as it found it.
func TestLive_EntityRoundTrip(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	if url == "" {
		t.Skip("set MXCLI_MCP_URL to run the live MCP integration test")
	}
	module := os.Getenv("MXCLI_MCP_MODULE")
	if module == "" {
		module = "MyFirstModule"
	}

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
	const name = "MxcliMcpProbe"

	// create
	entity := newPersistentEntity(name, attr("Title", stringType{}))
	if err := b.ensureSchema("DomainModels$Entity", "DomainModels$Attribute"); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	value, err := b.buildEntityValue(entity)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	if err := b.pedUpdate(module, pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "add", Value: value}}); err != nil {
		t.Fatalf("add entity: %v", err)
	}

	// validate + read back
	if err := b.pedCheckErrors(module); err != nil {
		t.Errorf("check errors after create: %v", err)
	}
	idx, err := b.entityIndex(module, name)
	if err != nil {
		t.Fatalf("entity not found after create: %v", err)
	}
	t.Logf("created %s.%s at /entities/%d", module, name, idx)

	// cleanup
	if err := b.pedUpdate(module, pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "remove", Index: &idx}}); err != nil {
		t.Fatalf("remove entity: %v", err)
	}
	if _, err := b.entityIndex(module, name); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("entity should be gone after remove, got: %v", err)
	}
}

// stringType is a tiny AttributeType for the live test (avoids importing the
// full domainmodel constructors here).
type stringType struct{}

func (stringType) GetTypeName() string { return "String" }

// TestLive_AttributeDefault exercises applyAttributeDefaults against a running
// Studio Pro 11.12+ MCP server: it creates a throwaway entity with String /
// Integer / Boolean (and, if MXCLI_MCP_ENUM is set, Enumeration) attributes,
// sets their defaults via the value/defaultValue path-op, reads them back,
// validates, re-applies (idempotency), and removes the entity. Skipped unless
// MXCLI_MCP_URL is set. Example:
//
//	MXCLI_MCP_URL=http://localhost/mcp MXCLI_MCP_DIAL=host.docker.internal:7784 \
//	MXCLI_MCP_MODULE=MES MXCLI_MCP_ENUM=MES.WorkOrderStatus:Draft \
//	go test ./mdl/backend/mcp/ -run TestLive_AttributeDefault -v
func TestLive_AttributeDefault(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	if url == "" {
		t.Skip("set MXCLI_MCP_URL to run the live MCP integration test")
	}
	module := os.Getenv("MXCLI_MCP_MODULE")
	if module == "" {
		module = "MyFirstModule"
	}
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

	const name = "MxcliDefaultProbe"
	want := map[string]string{"S": "hello", "N": "0", "B": "true"}
	plainAttrs := []*domainmodel.Attribute{
		withDefault(attr("S", &domainmodel.StringAttributeType{}), "hello"),
		withDefault(attr("N", &domainmodel.IntegerAttributeType{}), "0"),
		withDefault(attr("B", &domainmodel.BooleanAttributeType{}), "true"),
	}
	if env := os.Getenv("MXCLI_MCP_ENUM"); env != "" {
		ref, val, ok := strings.Cut(env, ":")
		if ok {
			plainAttrs = append(plainAttrs,
				withDefault(attr("E", &domainmodel.EnumerationAttributeType{EnumerationRef: ref}), val))
			want["E"] = val
		}
	}

	entity := newPersistentEntity(name, plainAttrs...)
	if err := b.ensureSchema("DomainModels$Entity", "DomainModels$Attribute"); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	value, err := b.buildEntityValue(entity)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	if err := b.pedUpdate(module, pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "add", Value: value}}); err != nil {
		t.Fatalf("add entity: %v", err)
	}
	entIdx, err := b.entityIndex(module, name)
	if err != nil {
		t.Fatalf("entity not found after create: %v", err)
	}
	defer func() {
		_ = b.pedUpdate(module, pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "remove", Index: &entIdx}})
	}()

	// Apply defaults (the feature under test), then read them back.
	if err := b.applyAttributeDefaults(module, entIdx, plainAttrs); err != nil {
		t.Fatalf("applyAttributeDefaults: %v", err)
	}
	if err := b.pedCheckErrors(module); err != nil {
		t.Errorf("check errors after defaults: %v", err)
	}
	liveNames, _ := b.liveAttributeNames(module, entIdx)
	liveDefaults, err := b.liveAttributeDefaults(module, entIdx, len(liveNames))
	if err != nil {
		t.Fatalf("liveAttributeDefaults: %v", err)
	}
	got := map[string]string{}
	for i, n := range liveNames {
		got[n] = liveDefaults[i]
	}
	for n, w := range want {
		if got[n] != w {
			t.Errorf("attribute %s default = %q, want %q", n, got[n], w)
		}
	}
	t.Logf("defaults applied: %v", got)

	// Idempotency: a second apply must be a clean no-op (values unchanged).
	if err := b.applyAttributeDefaults(module, entIdx, plainAttrs); err != nil {
		t.Errorf("second applyAttributeDefaults (idempotent): %v", err)
	}
}

func withDefault(a *domainmodel.Attribute, def string) *domainmodel.Attribute {
	a.Value = &domainmodel.AttributeValue{DefaultValue: def}
	return a
}

// TestLive_CheckErrorsIsScoped is the control for ped_check_errors' scoping. It
// needs a document Studio Pro reports an error on, named by MXCLI_MCP_BROKEN as
// "<documentType>:<qualifiedName>" (e.g. an enumeration without values on 11.15:
// "Enumerations$Enumeration:MyFirstModule.Zz_Mcp115_BrokenEnum"), and a clean
// MXCLI_MCP_MODULE domain model. The broken document must fail its own check, and
// must NOT fail the domain model's: on 11.15 the pre-11.15 `documents` argument is
// silently ignored and the check runs project-wide, which fails both.
//
//	MXCLI_MCP_URL=http://localhost/mcp MXCLI_MCP_DIAL=host.docker.internal:7793 \
//	MXCLI_MCP_BROKEN='Enumerations$Enumeration:MyFirstModule.Zz_Mcp115_BrokenEnum' \
//	go test ./mdl/backend/mcp/ -run TestLive_CheckErrorsIsScoped -v
func TestLive_CheckErrorsIsScoped(t *testing.T) {
	url := os.Getenv("MXCLI_MCP_URL")
	broken := os.Getenv("MXCLI_MCP_BROKEN")
	if url == "" || broken == "" {
		t.Skip("set MXCLI_MCP_URL and MXCLI_MCP_BROKEN to run the live check-scoping control")
	}
	docType, docName, ok := strings.Cut(broken, ":")
	if !ok {
		t.Fatalf("MXCLI_MCP_BROKEN = %q, want <documentType>:<qualifiedName>", broken)
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
	t.Logf("connected to %s %s (check_errors filters advertised: %v)", si.Name, si.Version, c.SupportsToolArg("ped_check_errors", "filters"))
	b := &Backend{client: c}

	if err := b.checkDocumentNow(docType, docName); err == nil {
		t.Errorf("%s %s: no error reported for the deliberately broken document", docType, docName)
	} else {
		t.Logf("broken document reported: %v", err)
	}
	if err := b.pedCheckErrors(module); err != nil {
		t.Errorf("domain model of %s failed on another document's error: %v", module, err)
	}
}
