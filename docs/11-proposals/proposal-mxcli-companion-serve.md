---
title: mxcli companion, slice 2 — a long-lived mxcli the mod talks to over a Unix socket
status: draft
date: 2026-10-11
related:
  - proposal-mxcli-companion-mod.md
  - PROPOSAL_concurrent_access.md
  - PROPOSAL_multi_project_tree.md
---

# mxcli companion, slice 2 — a long-lived mxcli the mod talks to over a Unix socket

Slice 2 of the [companion umbrella](proposal-mxcli-companion-mod.md). Slice 1
shipped the Project pane with one mxcli process per click. This slice gives the
mod one mxcli that stays running for the session, so a click costs a request
rather than a process start and a project open, and so the model's changes —
the agent's and Studio Pro's — arrive as events instead of being guessed from
tool calls. Every later slice (brain, verification, journeys, the inbox) reads
through it.

## 1. Problem

**Measured on Evora** (5,748 tree nodes, 1.2 MB of tree JSON), idle machine,
three runs each, `bin/mxcli` built from `main` on 2026-10-11:

| What | Time |
|---|---|
| `describe microflow …` as its own process | 0.95 / 1.03 / 1.28 s |
| `describe page …` as its own process | 0.53 / 0.57 / 0.33 s |
| connect and `show modules`, nothing else | 0.93 / 0.89 / 0.86 s |
| `project-tree` as its own process | 1.28 / 1.06 / 1.13 s |
| **20 describes in one process** (10 microflows, 10 pages) | 3.61 / 3.34 / 3.24 s |

Twenty describes in one process take about what three take as separate
processes: after the first, a describe costs under 0.2 s. Most of a click is
starting mxcli and opening the project, every time. Under load it gets worse —
on the same day, with the machine busy, single describes took 2.4–7.7 s
(slice 1's PR records them), which is the experience of a user whose laptop is
also running Studio Pro and a Mendix runtime.

Two more costs of the slice-1 shape:

- **Changes are guessed.** The tree reloads when a `tool.call` hook sees the
  agent run something that looks like a write (`exec`, a writing `-c`, …). A
  change made in Studio Pro, or by a command the pattern misses, leaves the pane
  stale until someone presses Refresh. That is open question 4 of the umbrella.
- **Every later view multiplies it.** The Brain, Verification and Journeys views
  each need several reads per render (`brain plan`, test results, catalog
  queries). At a process per read they would take seconds per redraw.

## 2. What the mod API allows (checked against Claude Code 2.1.293)

| Need | Available | Consequence |
|---|---|---|
| Keep a child running | `$.process.spawn({ argv })` streams the child's stdout and stderr piece by piece; the child lives as long as the loop reading it, and **dies with the module** (unload, reload, session end) | No orphan daemons: the mod owns the process's life |
| Send it requests over stdin | **No.** `input` is "text written to the child's standard input, which is then closed" ("a later form may take the text in pieces") | A stdio JSON-RPC (LSP-style) protocol is impossible today |
| Talk to it another way | `$.http.fetch(url, { method, body, socketPath })` — HTTP, optionally **over a Unix socket** instead of TCP | Requests go to an HTTP server on a socket |
| Hear from it | its stdout, through the spawn loop | Events (ready, changed) are lines on stdout |

So the shape is fixed by the API: **requests over HTTP on a Unix socket,
events on stdout, life tied to the mod.**

## 3. Design

### 3.1 The command

```
mxcli serve --socket -p app.mpr
```

`serve` today is a treemap over TCP (`cmd/mxcli/serve.go`, `--port`). `--socket`
adds an API mode beside it: no TCP listener, no HTML, JSON only. One command name
keeps "a long-running mxcli serving this project" in one place; the existing
treemap mode is unchanged. (Open question 1 weighs a separate name.)

### 3.2 Socket

- mxcli chooses the path and announces it (§3.4). The mod does not pick one: it
  has no temp-directory API and Unix socket paths are capped (108 bytes on Linux,
  104 on macOS), which a project path under `.mxcli/` can exceed.
- Location: `$XDG_RUNTIME_DIR/mxcli/<hash>.sock` when set, else
  `$TMPDIR/mxcli-<uid>/<hash>.sock`; `<hash>` from the absolute `.mpr` path. The
  directory is created `0700`, the socket `0600`: only the user reaches it.
- A stale socket from a crashed run is removed at start (nothing answers on it).
- No TCP at all: nothing to firewall, no port to allocate, no auth token to
  manage, and no network listener in a feature that needs none.

### 3.3 API (v1)

Every response body is **byte-for-byte what the CLI prints with `--json`** for
the same question, produced by the same function (umbrella P1: one
implementation). The server is a transport, not a second implementation.

| Route | Answers with | CLI equivalent |
|---|---|---|
| `GET /v1/health` | `{ mpr, mxcliVersion, generation }` | — |
| `GET /v1/tree` | the tree, including `describable` | `mxcli project-tree` |
| `POST /v1/describe` `{ type, qualifiedName }` | `{ name, type, mdl, executable }` | `mxcli describe … --json` |

Errors are HTTP 4xx/5xx with `{ error }`, the text the CLI would print. Read-only:
there is **no write route** in this slice. Changing the model stays the agent's,
through the commands it already runs, where `check`, permissions and review see
it. Later slices add read routes (`/v1/brain/plan`, `/v1/tests/latest`,
catalog queries) on the same rule.

### 3.4 Events (stdout, one JSON object per line)

```json
{"event":"ready","socket":"/run/user/1000/mxcli/3f2a.sock","mxcliVersion":"…","generation":1}
{"event":"changed","generation":2,"paths":["mprcontents/…/….mxunit"]}
{"event":"error","message":"…"}
```

`changed` comes from a file watcher on the `.mpr` and, for MPR v2, the
`mprcontents/` tree — the one `cmd/mxcli/tui/watcher.go` already implements
(fsnotify, 500 ms debounce). It fires for any writer: the agent, a script,
Studio Pro. One gap to close while lifting it: fsnotify does not recurse, and
the TUI watcher adds the `mprcontents/` subdirectories once, at start, so a
document written into a hash directory created later goes unseen. The shared
watcher adds a directory when its `Create` event arrives. The mod drops its caches and
re-fetches on `changed`; the `tool.call` heuristics of slice 1 are removed once
this is measured to catch everything they did (§5, hypothesis 3).

### 3.5 Holding the model without blocking writers

This is the slice's real risk. The server keeps the project open read-only and
caches per **generation**: the tree once, each describe once. On `changed` it
bumps the generation, drops the caches and **closes the connection**, reopening
lazily on the next request.

Two rules follow from the concurrent-access design
(`PROPOSAL_concurrent_access.md`, proposed, not implemented): it gives readers a
shared lock that blocks writers. A server holding a shared lock for its whole
life would stall every `mxcli exec`. So:

1. **The server never holds a lock between requests.** If that proposal lands,
   the server takes its shared lock per request (or per cache fill) and releases
   it, and a writer waiting on it waits for one request, not a session.
2. **An open read connection must not block a write**, for MPR v1 (one SQLite
   file) and v2 (`mprcontents/` plus the SQLite index). This is measured before
   anything else is built (§5, hypothesis 1). If it does block, the server reads
   what it needs into memory and closes the connection after every cache fill —
   slower first reads, still one process.

### 3.6 The mod

- On first open of the Project pane (not at session start: a session that never
  opens the pane spawns nothing), the mod spawns `mxcli serve --socket -p <mpr>`
  and waits for `ready`.
- Requests use `$.http.fetch("http://mxcli/v1/…", { socketPath })`.
- **Fallback, always:** if `ready` does not come within a few seconds, the child
  exits, or the mxcli is older than `serve --socket` (the version skew slice 1
  already met: a committed companion with an older downloaded mxcli), the mod
  keeps the slice-1 path — one process per request — and says so once in the
  pane header. Nothing breaks on an older mxcli; it is only slower.
- The slice-1 per-document cache stays as the mod's own, keyed by generation.

## 4. Changes

| File | Change |
|---|---|
| `cmd/mxcli/serve.go`, `cmd/mxcli/serve_api.go` (new) | `--socket` mode: socket placement and permissions, HTTP routes, `ready`/`changed`/`error` lines on stdout |
| `cmd/mxcli/project_tree.go`, `cmd/mxcli/cmd_describe.go` | Factor the tree and describe-to-JSON paths into functions both the CLI and the server call, so their output cannot diverge |
| `cmd/mxcli/tui/watcher.go` → shared package | Lift the watcher out of the TUI so `serve` and the TUI use one implementation, and add directories created after start (§3.4) |
| `mods/companion/hooks/` | Spawn and supervise the server; fetch over the socket; react to `changed`; fall back to per-request processes |
| `docs-site/src/tools/companion.md` | What runs in the background and why; how to tell it fell back |

No BSON or MDL changes; nothing writes the project.

## 5. Hypotheses, each with the measurement that settles it

1. **A held read-only connection does not block `mxcli exec`** on MPR v1 and v2.
   Integration test: open the server's connection, run a write through the
   executor in a second process, assert it succeeds and that `changed` follows.
   This is first: §3.5's design depends on it.
2. **A warm click is under 0.3 s on Evora.** Measure tree and describe through
   the socket on an idle machine and under load, against §1's table.
3. **The watcher catches every change the `tool.call` heuristics caught, plus
   Studio Pro's.** Replay slice 1's reload test cases, then save in Studio Pro
   with the pane open.
4. **Memory is acceptable.** Record the server's resident size with Evora loaded
   and after a hundred describes; decide whether caches need a bound.
5. **The socket works where the pane works** — Linux, macOS, Windows (Go and the
   OS support AF_UNIX on Windows 10 and later; whether `$.http`'s `socketPath`
   does there is unverified). Where it does not, the fallback is the answer.

## 6. Test plan

- **Go:** route tests asserting each response equals the CLI's `--json` output
  for the same project (the "one implementation" rule, held by a test); socket
  permission and stale-socket tests; watcher → `changed` integration test
  (`-tags integration`); hypothesis 1 as an integration test with a control (the
  same write with no server running).
- **Mod** (`claude plugin test`): a mocked spawn stream and `http.fetch` beneath
  the plugin — `ready` then requests over the socket; `changed` invalidates the
  cache and redraws; no `ready` → fallback to per-request processes; an mxcli
  without `--socket` → fallback.
- **By hand:** Evora, terminal and desktop; edit in Studio Pro with the pane
  open.

## 7. Alternatives considered

- **stdio JSON-RPC** (a `serve --stdio`, or custom requests on `mxcli lsp`). The
  natural shape, and blocked: the mod cannot write to a spawned child's stdin
  after the first text. Revisit if `$.process.spawn` takes input in pieces; the
  routes would map one to one onto methods.
- **TCP on localhost.** Works with plain `$.http.fetch`, but needs a free port
  and an auth token (any local process can connect to a port), and makes a
  network listener out of a feature that needs none.
- **The TUI's agent socket** (`mxcli tui --agent-socket`). A Unix socket already,
  but its own JSON protocol rather than HTTP, and coupled to a running TUI.
- **Precomputed MDL in the catalog** (the FTS `source` table). Fast to read, but
  a full catalog build to fill and stale between builds; it suits search, not a
  pane that must show the current model.
- **Keep one process per click.** Under a second on an idle machine is tolerable
  for a tree and a describe; it is not for views that read several things per
  redraw, and it can never see Studio Pro's changes.

## 8. Open questions

1. **Command name.** `serve --socket` (one "serving" command, two modes) or a
   separate `mxcli daemon` / `mxcli companion serve`? The first keeps one place;
   the second keeps the treemap's flags out of the API's help.
2. **Who else uses it.** VS Code spawns `project-tree` and `describe` per request
   too (`vscode-mdl/src/projectTreeProvider.ts`, `mdlContentProvider.ts`); it can
   speak HTTP over a socket today. Fold it in here, or after the mod has proven
   the API?
3. **Several apps in one repo.** One server per `.mpr`, or one server with the
   project as a request parameter (`PROPOSAL_multi_project_tree.md`)?
4. **Retiring the `tool.call` heuristics** entirely, or keeping them as a
   belt-and-braces trigger when the watcher misses (e.g. network filesystems
   where fsnotify is unreliable)?
