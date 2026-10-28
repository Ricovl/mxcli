---
title: OpenAPI import — generate response entities and wire operations to them
status: draft
date: 2026-10-21
related:
  - PROPOSAL_mapping_coverage.md
---

# Proposal: OpenAPI import generates response entities and mappings

**Status:** Draft
**Date:** 2026-10-21
**Issue:** ako/mxcli#1234 (mendixlabs/mxcli#280)

## Problem Statement

`create consumed rest service M.Svc ( OpenAPI: '…' )` imports a spec's
operations, parameters and authentication, but not what the operations return.
Every JSON response becomes `Response: json`, which gives a microflow a raw
string. To get objects, the user has to write the non-persistent entities, the
associations and a `Response: mapping` tree for each operation by hand. That is
the part of a REST integration that takes longest and goes wrong most often.

The report (mendixlabs/mxcli#280) came from the second route into this problem.
An agent asked to "generate the Consume Rest Client from
https://petstore.swagger.io/v2/swagger.json" wrote the entities, JSON
structures and import-mapping documents correctly. It then wired every
operation as `Response: JSON AS $PetJson`, so none of them was used. The tool
gave the agent no way to generate them and no guidance on how to wire them.

Measured on `main` (d8f0371b9) against both Petstore specs, the gap is larger
than the issue describes:

| Spec | What `describe contract operation from openapi` produces today |
|---|---|
| OpenAPI 3.0 (`petstore3.swagger.io/api/v3/openapi.json`) | Operations and parameters correct; every response `json`; request bodies `json from $body`; `BaseUrl: ''` with a warning (relative server URL) |
| **Swagger 2.0** (`petstore.swagger.io/v2/swagger.json`, the reporter's spec) | `BaseUrl: ''`, request bodies dropped, **every response `none`**, and **no warning**. The parser reads only OpenAPI 3 fields, so a 2.0 document parses "successfully" into an empty contract |

The Swagger 2.0 row is a silent downgrade in its own right. It is fixed in
Phase 0 whatever happens to the rest of this proposal.

**Who benefits:** anyone who consumes a third-party API, and agents most of
all, because generating this scaffolding is exactly what they get wrong.

## What exists to build on

The **storage side is already done**. Mendix 11 does not use an import-mapping
document for a consumed REST operation. It stores the mapping **inline** on the
operation as `Rest$ImplicitMappingResponseHandling`, whose `RootMappingElement`
is an `ImportMappings$ObjectMappingElement` tree. mxcli writes and reads that
today (`mdl/backend/modelsdk/consumed_rest_write.go`,
`restImplicitMappingResponseToGen`). `describe` round-trips it (#843), and
`mdl-examples/bug-tests/843-rest-response-mapping.mdl` builds with 0 errors on
11.13.0:

```sql
operation "SearchRoutes" ( …,
  Response: mapping ZZB.Routing {
    RoutingCode = routing_code,
    RoutingName = routing_name
  }
)
```

So the issue's framing ("generate JSON structures and import mappings") is the
wrong target for this document type. The target is **non-persistent entities
plus an inline `Response: mapping`**, which no extra documents need to back.

What does **not** exist:

| Gap | Where |
|---|---|
| No schema model: `openapi.Schema` is `{type, format}` only. There is no `$ref`, `properties`, `items`, `required`, `enum`, `allOf`, `components.schemas`, or Swagger 2.0 `definitions` | `mdl/openapi/parser.go` |
| `resolveResponseType` reads only `content`, so a 2.0 response (`schema` directly on the response) resolves to `NONE` | `mdl/openapi/parser.go` |
| The inline response writer hardcodes an `(Object)` root with `MaxOccurs 1`. An **array-rooted** response (`findPetsByStatus` returns `Pet[]`) and a nested array (`Pet.tags`) cannot be expressed | `consumed_rest_write.go` |
| Value elements are always written with `DataTypes$StringType`, whatever the attribute's type. This is untested for non-String attributes | `consumed_rest_write.go` |
| Status code fixed at 200 | `consumed_rest_write.go` |

## BSON Structure

**Known, and implemented:** `Rest$ImplicitMappingResponseHandling` with
`ContentType`, `StatusCode` and `RootMappingElement` (an
`ImportMappings$ObjectMappingElement` with `Entity`, `ExposedName`,
`JsonPath` such as `(Object)|field`, `ObjectHandling: Create`, `Association`,
`MinOccurs`/`MaxOccurs` and `Children` of
`ImportMappings$ValueMappingElement`). Measured fixes already recorded:
- no `IsDefaultType` on value elements (#1097);
- multi-segment paths (`rest-inline-mapping-paths.mdl`).

**Unknown. These must be measured from a Studio Pro reference before Phase 2:**

1. **Array root.** What Studio Pro stores for an operation whose response is a
   JSON array: the root element's `JsonPath` (`(Array)`? `(Array)|(Object)`?),
   whether there is a wrapper element, and its `MaxOccurs`. Neither the demo
   corpus nor mxcli has an example. A related finding recorded zero
   `Rest$ImplicitMappingBody` across 9 packages, which suggests the corpus is
   thin on inline REST mappings generally.
2. **Nested array** (`Pet.tags: Tag[]`): the element shape and `MaxOccurs -1`,
   by analogy with message definitions and JSON-structure mappings. Verify it,
   don't assume it.
3. **Value element `Type`** for Integer, Long, Decimal, Boolean and DateTime
   attributes: whether `DataTypes$StringType` is accepted, or whether the type
   must match the attribute. `mx check` may tolerate a mismatch that fails at
   runtime, so this needs a runtime check (see Test Plan).
4. **Association direction and name** that Studio Pro generates when it creates
   entities from a JSON response, so generated models match what a Studio Pro
   user would get.
5. **Primitive arrays** (`Pet.photoUrls: string[]`): whether Studio Pro maps
   them at all (helper entity with a value attribute, as JSON structures do?)
   or skips them.

**Requested reference project:** a Mendix 11.x app with one consumed REST
service holding operations whose responses are:
(a) an object with nested object, object array and primitive array;
(b) a top-level array;
(c) an object with Integer, Long, Decimal, Boolean and DateTime fields.
Entities should be created through Studio Pro's own response-to-entity flow.
`mxcli bson dump --type Rest$ConsumedRestService` on it settles items 1–5.

## Proposed MDL Syntax

No new statement. One property on the existing statement, which is off by
default so existing scripts keep their behaviour:

```sql
create or modify consumed rest service Shop.Petstore (
  OpenAPI: 'https://petstore3.swagger.io/api/v3/openapi.json',
  BaseUrl: 'https://petstore3.swagger.io/api/v3',
  GenerateEntities: Yes
);
```

With `GenerateEntities: Yes`, the statement:

1. resolves each operation's success response schema (`200`, `201`, then the
   first `2xx`) for `application/json`;
2. creates or modifies one **non-persistent entity per named schema**
   (`#/components/schemas/Pet` → `Shop.Pet`). Anonymous inline schemas are
   named `<Operation>Response`, or `<Parent><Property>` when nested;
3. creates an association for each nested object and object array;
4. writes each operation's response as `Response: mapping Shop.Pet { … }`.

The output is ordinary MDL. `describe contract operation from openapi '…'`
already prints the MDL an import would run, so it gains the entity and
association statements too. The user (or agent) can review the import, or
paste it and edit it instead of running it:

```sql
create or modify non-persistent entity Shop.Pet (
  Id: Long,
  Name: String,
  Status: String
);
create or modify non-persistent entity Shop.Category ( Id: Long, Name: String );
create or modify association Shop.Pet_Category from Shop.Pet to Shop.Category;  -- direction: BSON item 4

create or modify consumed rest service Shop.Petstore ( … )
{
  operation getPetById ( Method: get, Path: '/pet/{petId}', Parameters: ($petId: Long),
    Response: mapping Shop.Pet {
      Id = id, Name = name, Status = status,
      create Shop.Pet_Category/Shop.Category = category { Id = id, Name = name }
    }
  )
};
```

| OpenAPI | Mendix attribute |
|---|---|
| `integer` / `int32` | Integer |
| `integer` / `int64` | Long |
| `number` | Decimal |
| `boolean` | Boolean |
| `string` / `date-time`, `date` | DateTime |
| `string` (incl. `enum`) | String |
| `$ref` to object | associated entity |
| `array` of object | associated entity, list (BSON item 2) |
| `array` of primitive | BSON item 5 — until measured, reported as not imported |
| `oneOf` / `anyOf` / `additionalProperties` / `not` | not imported, reported by name |
| `allOf` | properties merged into one entity |

Every schema member the import cannot represent is **listed in the output with
its reason**, never dropped silently. This is the lesson of mendixlabs/mxcli#1118
and of the Swagger 2.0 row above.

Attribute names follow the existing rules: Mendix-reserved names are renamed
and reported (`id` → `Id`, `type` → `TypeValue`; CE7247). The mapping keeps
the JSON name, so the wire contract is unaffected.

## Implementation Plan

Each phase is its own PR.

**Phase 0: stop the silent downgrades.** Small, independent, can ship now.
- Detect `swagger: "2.0"` and refuse it with a clear message until Phase 4
  ("Swagger 2.0 is not supported yet; convert with …"), instead of returning an
  empty contract.
- Add skill guidance (`.claude/skills/mendix/` REST client skill) to wire
  responses with `Response: mapping Entity { … }`, and to remember that an
  import-mapping document is not referenced from a consumed REST operation.
  This closes the agent route the reporter took.

**Phase 1: schema model.** Model `Schema` with `$ref` (local `#/` only),
`properties`, `items`, `required`, `enum`, `format`, `allOf`, `nullable`, and
`components.schemas`. Add a resolver with cycle detection: a recursive schema
reuses its entity rather than recursing. Pure unit tests in `mdl/openapi/`.

**Phase 2: generation for object-rooted responses** (needs BSON items 3–4).
Plan entities, associations and mapping trees from Phase 1. Emit them as **AST
statements executed through the existing handlers** rather than writing
documents directly. Entity creation then goes through the normal create/modify
path, so `canon.Reconcile`, GUID preservation on re-import and validation come
for free, and `describe contract … from openapi` prints the same statements.
Wire `GenerateEntities` through grammar, visitor and AST.

**Phase 3: array roots and nested arrays** (needs BSON items 1, 2, 5). Extend
`model.RestResponseMapping` and `restInlineMappingElementToGen` with the
measured array shapes, and describe/round-trip them. This is the first change
to the writer.

**Phase 4: Swagger 2.0.** Normalise 2.0 into the 3.0 model (`definitions` →
`components.schemas`, `host`/`basePath`/`schemes` → server URL, response
`schema` → `content["application/json"]`, `in: body` → request body). Then
remove the Phase 0 refusal.

**Out of scope:** request bodies as `Body: mapping` (export direction; same
generator, separate PR once responses are proven); non-`2xx` responses; XML
responses; generating enumerations from `enum` (String first, an enumeration
is a later opt-in); external `$ref` URLs.

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/openapi/parser.go` | Schema model, `components.schemas`, `$ref` resolver; Swagger 2.0 detection (P0) and normalisation (P4) |
| `mdl/openapi/entities.go` (new) | Schema → planned entities, associations and mapping trees; type table; naming; unsupported-member report |
| `mdl/grammar/MDLParser.g4`, `mdl/visitor/visitor_rest.go`, `restClientSchema` | `GenerateEntities` property |
| `mdl/ast/` | `CreateRestClientStmt.GenerateEntities` |
| `mdl/executor/cmd_rest_clients.go` | `createRestClientFromSpec` and `describeContractFromOpenAPI` run or print the generated statements before the service |
| `model/types.go`, `mdl/backend/modelsdk/consumed_rest_write.go` + read | Array roots and nested arrays (P3), value element types (if item 3 requires it) |
| `.claude/skills/mendix/…` (REST client skill) | P0 guidance |
| `docs-site/src/` REST client reference | `GenerateEntities` and the type table |

## Version Compatibility

The OpenAPI import already requires Mendix 11.0+ (`Rest$JsonBody`; see the
`@version: 11.0+` marker in `20-openapi-import-examples.mdl`).
`Rest$ImplicitMappingResponseHandling` is already written under the same gate.
`GenerateEntities` inherits the gate; no new version row is expected. Confirm
against the reference project that array-rooted mappings exist in the
earliest 11.x supported.

## Test Plan

- **Unit, `mdl/openapi/`:** `$ref` resolution, `allOf` merge, recursive
  schema, unsupported-member report, Swagger 2.0 refusal (P0) and normalisation
  (P4). Petstore 2.0 and 3.0 specs as **local fixtures**: the doctype test
  today fetches over the network.
- **Executor, `MockBackend`:** `GenerateEntities: Yes` creates the expected
  entities and associations and wires each operation. Re-running `create or
  modify` creates nothing new and changes no entity's GUID. Include a control
  with an entity created by Studio Pro, where `GUID != $ID`, because only that
  subject can fail a GUID test (CLAUDE.md).
- **Doctype test:** extend `20-openapi-import-examples.mdl` with
  `GenerateEntities: Yes` over the local fixtures.
- **Build, two variants:** `mx check` 0 errors on the generated Petstore 3
  project, and the same project with each unmeasured shape forced in (array
  root, a typed value element). The second variant is what shows whether
  mxbuild catches a wrong shape at all.
- **Runtime (`verify-in-runtime.md`):** a valid-looking mapping can pass
  `mx check` and still fail on the first call. Boot with `run --local` against
  a stub server returning Petstore payloads. Assert that a microflow calling
  `findPetsByStatus` gets a list of `Pet` with `Category` associated and typed
  attributes filled. This is the only test that proves item 3.

## Open Questions

1. **BSON items 1–5** above. Can you provide the Studio Pro reference project?
   Phase 2 can start on items 3–4 alone; Phase 3 waits for 1, 2 and 5.
2. **What does Studio Pro's own OpenAPI import do with responses?** If it
   generates entities, mxcli should copy its naming and shape exactly rather
   than invent its own. If it does not, `GenerateEntities` is mxcli's own
   addition and the naming below is ours to choose.
3. **Property name and default.** Is `GenerateEntities: Yes` right, and opt-in
   rather than default? Opt-in keeps existing scripts from suddenly creating
   domain-model entities. A default would make the reporter's one-liner just
   work.
4. **Where generated entities go:** always the service's module, or an
   `EntitiesModule:`/folder option? Proposed: the service's module, nothing
   else until asked.
5. **Name collisions** with an existing entity that the import did not create
   (`Shop.Order` already exists and is persistent). Proposed: refuse and name
   the conflict, rather than overwrite or suffix. An import must not convert a
   user's entity.
6. **Shared schemas across services:** two specs that both declare `Error` in
   one module. Proposed: the second import reuses the entity only if its
   attributes match; otherwise it refuses as in (5).
