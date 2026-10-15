---
title: Mendix whiteboard — sketch an app with an agent, from napkin to running model
status: draft
date: 2026-10-14
related:
  - PROPOSAL_project_brain.md
  - PROPOSAL_graph_analysis.md
  - PROPOSAL_mxcli_dev_warm_loop.md
  - starlark-security.md
  - journey-architecture-viz.md
  - PROPOSAL_vscode_visualizations.md
---

# Proposal: Mendix whiteboard

**Status:** Draft
**Date:** 2026-10-14

## Problem Statement

Requirements for a Mendix app are usually worked out at a whiteboard, then
translated by hand into Studio Pro. The translation loses most of what was
said: the sketch is photographed, the notes are typed up into a backlog, and the
model that gets built is checked against neither. With an agent and mxcli the
translation step can disappear — but only if the conversation happens somewhere
the agent can read and draw, and only if every later artifact (requirement,
MDL, model, running app) stays connected to the sketch it came from.

The whiteboard is that place: a large canvas on which a user sketches a user
journey, screens, flows and data by hand; the agent asks questions until the
sketch is specific, draws progressively more exact versions of it on top, and
turns it into MDL that mxcli checks, applies and runs. The user can at any time
switch a part of the board between **sketch**, **MDL** and **app**, and read
the state of the whole solution through three **lenses**: security,
integration and architecture.

Who benefits:

- **Business analysts and product owners** who think in journeys and screens,
  not in microflows — they get a running app without leaving the whiteboard.
- **Mendix developers** who receive a spec that is already traced to model
  elements, test data and test cases, instead of a slide and a backlog.
- **Architects and security reviewers**, who get the solution's access rules,
  integrations and module structure as pictures computed from the model, not as
  a document someone has to keep current.

An illustrated walkthrough of the concept exists as a sketch deck (16 slides,
private artifact): <https://claude.ai/artifact/J1pLf64B1cFJsQHgL51NbK>. This
proposal is the written form; where the two disagree, this document wins.

What it is not: a replacement for Studio Pro, a general-purpose diagramming
tool, or a store for model facts. The model stays the source of truth for
everything mxcli can compute (see "State stays in its native home" below).

## The concept

### One board, five layers

Every part of the board is drawn in layers, each on top of the previous one,
like onion skin. Any layer can be faded in or out; the earlier ones stay
visible underneath, so the provenance of a field or a flow is always one glance
away.

| # | Layer | Drawn by | Content |
|---|-------|----------|---------|
| 0 | **sketch** | user, pen | Personas, screens as boxes, the flow as arrows, data as boxes |
| 1 | **notes** | agent, from the user's answers | Post-its: the answers to the agent's questions, each one a requirement |
| 2 | **impression** | agent, marker style | Screens with real fields and widgets; the flow as BPMN-like lanes, gateways and timers; entities with attributes and multiplicities |
| 3 | **spec & tests** | agent with user | Requirements tree, test data, Given/When/Then cases, edge cases |
| 4 | **model** | mxcli | Studio Pro-exact renderings of the workflow, domain model and pages, and screenshots of the running app |

The agent never skips a layer for a part of the board, but parts move through
the layers independently: the workflow may be at layer 4 while the pages are
still at layer 2. The deck's recommended order for the model layer is
**workflow first, then domain model, then pages**, each in small iterations.

### Frames are model elements

The unit of the board is a **frame**: a page, a microflow, a workflow, or a
domain-model region. A frame's title is the **qualified name** of the element
it becomes (`Helpdesk.Ticket_Detail`). That one convention joins the board, the
brain and the model with no mapping table: the frame name is the brain anchor
and the MDL target. Arrows between frames mean *navigates to*, *calls* or
*uses entity*, and become buttons, microflow calls and data sources.

### Three views per frame

| View | What it shows | Source |
|------|---------------|--------|
| **Sketch** | Strokes, notes, reference images, the layer stack | The artifact database |
| **MDL** | The frame's script, one file per frame | `whiteboard/<qualified-name>.mdl` in the project repo |
| **App** | Pages: a screenshot of the page in the running app. Workflows, microflows, domain models: mxcli's rendering of the *model* (`describe --format elk/mermaid`), since these have no running view | `mxcli run --local --watch` + Playwright; `mxcli describe` |

### Three lenses

A lens is an overlay the user lays over the whole board. The agent computes it
from the model and draws it onto the same elements the user sketched; problems
are marked in red, each with a numbered finding and a proposed fix.

| Lens | Question | Drawn as | Computed from |
|------|----------|----------|---------------|
| **security** | Who can see and do what? Is every part covered? Does any role lack privileges it needs? | A coloured dot per role on every screen, task and entity (dashed where a role needs access and lacks it); a role × element access matrix; red rings on gaps | `CATALOG.permissions`, `role_mappings`, `navigation_role_homes`, page/task access; the `sec_*` lint rules (`.claude/lint-rules/`, SEC001–SEC010) |
| **integration** | What is connected, what data flows which way, how is it processed, and is there exactly one source of truth for each piece of data? | External systems around the board, plugged into the elements they touch; each arrow labelled with what flows and how (OData, REST, business event); a star on the source of truth; red rings on copies that will drift | `odata_clients_data`, `odata_services_data`, `external_entities_data`, `rest_clients_data`, `published_rest_services_data`, `business_event_services_data`, `import_mappings_data`, `export_mappings_data` |
| **architecture** | Is the internal structure sound and future-proof? What needs refactoring, what should be extracted as its own app? | The board coloured by owning module; a module dependency graph with size/heat; cycles, god modules and extraction candidates marked; a target picture | `mxcli graph-report` and the `CATALOG.graph_*` views (`graph_module_dependencies`, `graph_module_cycles_data`, `graph_god_nodes`, `graph_module_coupling`, `graph_integration_surface`); ARCH lint rules |

The lenses are what makes the board an architecture tool and not only a
drawing tool: every finding is anchored to something the user drew, and every
finding is re-computed after the fix, so "0 gaps" is a measurement, not a claim.

## Design principles

These follow from rules this repository already holds; each is a constraint
on the implementation, not a preference.

1. **State stays in its native home** (CLAUDE.md, "Documentation Artifacts").
   The board stores only what nothing else can: strokes, notes, images, frame
   positions. Requirements live in the project brain; MDL lives in the repo;
   model facts live in the model and are queried, never copied onto the board.
2. **Status is derived, never stored.** "Built" comes from `brain plan`
   resolving anchors; "checked" from `mxcli check`; "verified" from a test or
   screenshot run; lens findings from catalog queries. The board displays these
   and is refreshed on every sync — it never holds a status field a human or
   the agent has to keep up to date.
3. **A human promotes.** The agent drafts requirements and decisions with
   `brain capture`; the user accepts them on the board, and only then does the
   agent run `brain promote` (the brain's own rule, PROPOSAL_project_brain.md
   §4.3).
4. **Renaming is not re-creating.** Renaming a frame or an element on the board
   must become `mxcli rename`, never a drop-and-create: an entity's `GUID` is
   the database's identity, and re-minting it drops the table on the next
   deploy (CLAUDE.md, "A `GUID` Is the Database's Identity"). On a throw-away
   prototype database this is cheap; the whiteboard must not teach a habit that
   loses production data later.
5. **Ambiguity becomes a question, not a guess.** A sketch that does not decide
   something (grid or list, who may reopen) produces an open question pinned to
   the frame (`brain capture --open`), answered with `brain resolve`.

## Where each piece of state lives

| State | Home | Written by | Read by the board as |
|-------|------|------------|----------------------|
| Strokes, typed notes, images, frame geometry and kind | Artifact database (`frames`, `strokes`, `texts`, `images`) | The page | — (it is the board) |
| Requirements, decisions, open questions | `docs/brain/plan/<slice>.md`, `docs/brain/modules/<Module>.md` | Agent (`capture`), user (`promote`, `resolve`) | A requirements tree: slices → frames → requirements, each marked built / planned / open |
| Pending user actions on the tree (accept, reject, answer) | Artifact database (`queue`) | The page | Applied by the agent on the next sync, then cleared |
| MDL per frame | `whiteboard/<qualified-name>.mdl` in the project repo | Agent; later the user, from the MDL view | The MDL view |
| The model | `.mpr` | `mxcli exec` | — (queried) |
| Rendered impressions, model diagrams, app screenshots | Artifact assets | Agent | The impression and app views |
| Lens findings | Recomputed per sync from the catalog | — | Lens overlays (cached in the artifact database only for display) |

The tree in the board's side panel replaces the Sketch Deck's slide list. It is
a view of `brain plan` plus `brain staged`, never an independent list.

## The loop

On "done" (or on any change, once the loop is trusted):

1. **Export** the board's collections from the artifact database and render each
   frame (as Sketch Deck's `sketch_render.py` does).
2. **Read** each changed frame with its notes and the arrows that touch it.
3. **Ask** about anything the sketch does not decide; pin each question to its
   frame and capture it as an open question.
4. **Capture** each new requirement against its frame's qualified name and the
   frame's slice; apply the user's queued accept/reject/answer actions with
   `brain promote`, `brain drop`, `brain resolve`.
5. **Draw** the next layer for the frames that are ready for it.
6. **Write** or update the frame's MDL; `mxcli check`, then
   `mxcli check --references`; pin errors to the frame they came from.
7. **Apply** with `mxcli exec`; `mxcli run --local --watch` hot-reloads page and
   microflow changes in about a second and restarts the runtime for entity
   changes.
8. **Render** the app view: Playwright screenshots of pages, `describe` diagrams
   of everything else; upload them as assets.
9. **Recompute** the active lenses and the requirements tree; publish.

The artifact cannot reach `localhost`, so screenshots are the reliable path for
the app view. A live view through `run --hub` is a later option and is
Linux-only by design (ADR-0009).

## BSON Structure

Not applicable. The whiteboard adds no document types and writes nothing
mxcli does not already write: every model change goes through MDL and the
existing executor (`mxcli exec`), with its existing reconciliation, identity
carry and GUID guard. No storage names, fields or pointers are introduced.

## Proposed MDL Syntax

None. The whiteboard is a consumer of existing MDL, not an extension of it.

Two CLI changes are proposed; both are small and each has a consumer outside
the whiteboard.

### 1. Per-requirement status in `brain plan --json`

`brain plan --json` reports counts per slice (`SliceProgress`: built, planned,
questions, unanchored). The requirements tree needs the state of each
requirement. The per-anchor states already exist (`AnchorFinding` in
`cmd/mxcli/brain/check.go`); the change exposes them grouped by entry:

```json
{
  "slices": [
    {
      "slice": "02-triage",
      "built": 1, "planned": 1, "questions": 0, "unanchored": 0,
      "requirements": [
        { "id": "a1b2c3", "title": "SLA per priority", "state": "built",
          "anchors": [ { "anchor": "@Helpdesk.Ticket.DueAt", "state": "resolved" } ] },
        { "id": "d4e5f6", "title": "Only the service desk reassigns", "state": "planned",
          "anchors": [ { "anchor": "@Helpdesk.ACT_Reassign", "state": "not found" } ] }
      ]
    }
  ],
  "built": 1, "total": 2
}
```

Additive: existing fields keep their meaning.

### 2. `mxcli lens` (phase 3, only if the skill-side queries prove insufficient)

Each lens is first implemented in the skill as catalog SQL (`mxcli -p app.mpr
-c "select … from CATALOG.permissions …"`) plus the existing `lint` and
`graph-report` output. Only if the same queries are needed by a second consumer
(the VS Code extension, `mxcli serve`) do they move into a command:

```bash
mxcli lens security     -p app.mpr --json   # role × element access, gaps
mxcli lens integration  -p app.mpr --json   # systems, flows, sources of truth, copies
mxcli lens architecture -p app.mpr --json   # modules, dependencies, cycles, candidates
```

The output is keyed by qualified name, so any view can place a finding on the
element it is about. Not adding the command up front keeps the CLI surface free
of a shape that has one caller.

## Implementation Plan

The whiteboard is three things: an **artifact page** (the board), a **skill**
(the agent's procedure), and the two **mxcli** changes above. The page and the
skill do not ship in this repository's binary; they follow the Sketch Deck's
model — a template artifact plus a skill — and the skill is published as a
Mendix skill so it reaches user projects.

### Phase 1 — board and loop, layers 0–2, screenshots

- Fork the Sketch Deck template into a "Mendix Whiteboard" template: frame
  kinds (page, workflow, microflow, domain), frame titles as qualified names,
  the requirements tree in place of the slide list, the Sketch / MDL / App
  toggle per frame, a queue collection for tree actions.
- The skill: the loop above for layers 0–2 and the app view of pages.
- `brain plan --json` per-requirement status (change 1).

### Phase 2 — spec, tests and the model layer

- Layer 3: test data and test cases written from the requirements, stored as
  `mxcli test` files next to the frame's MDL.
- Layer 4: model renderings via `describe --format elk/mermaid` for workflows,
  microflows and domain models.
- Editable MDL in the board's MDL view; an edit there outranks the sketch on
  the next sync.

### Phase 3 — lenses

- Security, integration and architecture overlays from catalog queries, lint
  and `graph-report`.
- `mxcli lens` only if a second consumer appears (change 2).

### Phase 4 — start from an existing app

- Seed a board from a project: `describe` per document gives MDL, the catalog
  gives the frames and their arrows, the lenses work unchanged.

### Files to modify/create

| File | Change |
|------|--------|
| `cmd/mxcli/brain/check.go` | Group `AnchorFinding`s by entry; add a per-requirement state to `SliceProgress` |
| `cmd/mxcli/cmd_brain.go` | `planReport` emits `requirements` per slice |
| `cmd/mxcli/brain/brain_test.go` (or a new `plan_json_test.go`) | Per-requirement state test, with control |
| `.claude/skills/mendix/whiteboard/SKILL.md` | The loop, the layer and lens procedures, the naming and rename rules |
| `docs-site/src/` | A user page on whiteboarding with mxcli |
| Phase 3, conditional: `cmd/mxcli/cmd_lens.go`, `mdl/catalog/` | `mxcli lens` |

The template page and the drawing kit live with the template artifact, not in
this repository.

## Version Compatibility

No new Mendix model features are used, so no version gating is added. The
whiteboard inherits the requirements of what it calls:

- `mxcli run --local` — a JDK matching the project's Java version and a
  PostgreSQL database, or `--db-type hsqldb` for none.
- Workflows need a Mendix version with workflows, and the workflow constructs
  the existing MDL supports; the skill must not draw a construct mxcli cannot
  write.
- `run --hub` (an optional live app view) is Linux-only (ADR-0009).

## Test Plan

- **Brain JSON (unit).** A slice with one requirement whose anchor resolves and
  one whose anchor does not: assert `built` and `planned` per requirement.
  Control: stub the per-entry grouping and confirm the test fails with both
  requirements reported the same.
- **Lens queries (integration, `-tags integration`).** A fixture project with a
  known gap of each kind — an entity with no access rule, a page a needed role
  cannot open, an entity copied from a consumed OData service, a module cycle.
  Each lens must report exactly those, and nothing after the fix is applied.
  Control: the same project with the fixes, which must report zero findings.
- **Skill evals.** A recorded board export (frames, strokes, notes) for the
  Helpdesk example; the eval asserts that the produced MDL passes
  `mxcli check --references` and that every frame's qualified name resolves
  after `exec`. Uses the eval framework (`proposal-eval-framework.md`).
- **Rename safety.** A frame renamed on the board must produce `mxcli rename`,
  not drop-and-create. Verified on a **Studio Pro-authored** entity (an
  mxcli-created one has `GUID == $ID` and cannot show the defect), comparing
  the stored `GUID` before and after.
- **Board page.** As Sketch Deck does: a seeded local copy, Playwright, one pass
  over the new controls (frame kind, view toggle, tree actions), no page errors.

## Open Questions

1. **Slices.** Is a slice a board region the user draws, a frame group, or only
   a name prefix on requirements? The brain needs a slice per requirement.
2. **Where the MDL lives.** `whiteboard/<qualified-name>.mdl` per frame, or one
   script per slice? Per frame matches the view; per slice matches how
   requirements are planned.
3. **Edits in two places.** When the user edits MDL in the board and the sketch
   in the same round, which wins? Proposed: MDL edits win; the agent redraws the
   impression from the model and flags the sketch as stale.
4. **Lens scope.** Should the integration lens also read the *runtime*
   (configured endpoints, constants per environment), or only the model? The
   model cannot tell a mock endpoint from production.
5. **Screenshots and privacy.** App screenshots of a database seeded with real
   data would be stored in the artifact's asset store. The default should be
   generated test data only; how is that enforced?
6. **Shared boards.** Several people on one board means several writers to the
   brain's staging queue, which is local and gitignored. Does the queue move
   into the artifact database for shared boards?
