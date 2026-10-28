---
title: The Warm Loop Fails Quietly
category: bug-pattern
last-synced: b68b6e26
covers:
  - cmd/mxcli
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli/
  - cmd/mxcli/docker/localboot.go
  - cmd/mxcli/docker/runlocal.go
  - cmd/mxcli/docker/webclient_supervisor.go
  - cmd/mxcli/docker/webclient_watch.go
  - cmd/mxcli/docker/webclient_pages.go
  - cmd/mxcli/docker/serve_recovery.go
  - cmd/mxcli/devloop_admin.go
  - cmd/mxcli/devloop_handshake.go
  - cmd/mxcli/docker/runtime_stop_reason.go
  - cmd/mxcli/docker/ensuredb.go
  - cmd/mxcli/docker/javadeps.go
  - cmd/mxcli/docker/mxenv.go
  - cmd/mxcli/docker/pagecheck.go
  - cmd/mxcli/docker/pagecheck_check.go
---

> **Do not duplicate**: the flags and their behaviour live in
> `.claude/skills/mendix/run-local.md`; the individual fixes live in the
> findings. This page describes the shape of failure in a loop mxcli only partly
> owns.

## What this is

`mxcli run --local` orchestrates processes it did not write: mxbuild, a JVM
runtime, a rollup bundler, PostgreSQL, and a browser. Sixteen `cmd/mxcli`
findings are failures in that loop, and they share a signature — **the thing that
breaks is not the thing that reports**. A page goes black with HTTP 200. The
runtime is gone and the CLI sits at several hundred percent CPU. Styles do not
change and nothing is stale. A login that is correct reports "Sign in failed".

## How it fits

**A missing asset answers 200.** Mendix serves an SPA shell before it knows
whether the client bundle exists, so a deleted `deployment/web/dist` renders a
black screen while every HTTP-level check passes. `curl /` is not a liveness
probe for this app; the bundle is.

**A dead child does not stop the parent.** A runtime that exited left a zombie
under a CLI that kept polling, for hours. Anything that supervises a long-lived
child needs to reap it and exit, or the failure presents as a hang rather than an
error.

**The log the user needs is not the log being captured.** Several rounds went
into `runtime.log` holding only the JVM banner: the application's own logging
goes through a subscriber that attaches after startup, so anything logged
*during* the start action — which is exactly where the after-startup test runner
logged — is not in the file. Capturing "the process's stdout" and capturing "the
app's log" are different jobs.

**Everything the loop resolves for another tool is a place to disagree
silently.** A relative `-p` reaches mxbuild as its own error text plus a page of
Windows JSON. A version resolved for one component and defaulted in another
produces a project at a Mendix version nobody asked for. An architecture picked
for `docker build` and not for `--local` gives `exec format error`. A credential
that is right for one API is wrong for the other — the M2EE admin password and
the test endpoint token are different secrets, and passing one where the other
belongs reports "Authentication failed" after the model has already been
modified.

**Guards that name a cause can name the wrong one.** A port-in-use guard blamed a
previous `run --local` that the user had already killed. A guard is a diagnosis,
and a wrong diagnosis costs more than a plain error — this is why the port owner
is now resolved from `/proc` rather than asserted.

**Stale state has more than one home.** A theme edit that does not appear, a
constant that does not reach a running app, a page that dies after ten
hot-applies with a 404 for a hashed chunk — each is a cache or a running process
holding something the file no longer says. The loop's value is that it does not
restart; the cost is that "what is actually being served" and "what is on disk"
are separate questions, and the answer has to be printed rather than assumed.

**A watcher's baseline is a source time, not a clock reading.** This one has
shipped twice, in both directions. The loop took `last = sourceMTime(...)` *after*
each successful apply and again after the boot finished — so any edit made during
a build or during the boot was folded into the baseline and never detected: no
`Change detected`, the app serving the previous model, and re-running the script
writing nothing because the write was already idempotent. The baseline has to be
the source time the last build was **made from**, captured before that build. The
test that catches it makes an edit *during* a build, not between builds, which is
the only arrangement either instance was visible in.

**A supervised child is a supervisor's job, not a handle.** The loop held a bare
pointer to the web-client bundler, so when the bundler died nothing restarted it
and every later change failed; a failed incremental build left it in a state no
later change could leave. The general remedy is that **a fresh bundler is the
universal recovery**: its first build is a full bundle of the current source, so
it covers a dead watcher, missing pages, dangling chunks and a `dist/` someone
else deleted. Reproducing it needs no patience — kill the runner process and the
dead-watcher state is there in seconds.

**The same holds for `mxbuild --serve`, and its state is worse hidden.** On the
versions where mxbuild writes `web/dist` itself, only a serve process's first build
bundles completely — every later one, *even with the model unchanged*, replaced the
bundle with one that has no pages, and reported success. The same process also
kept its first Sass error as CE6093 after the SCSS was fixed. Both were settled by
driving the serve `/build` API from a small harness with no watch loop: the
unchanged-model second build is the row that rules out "something in the change"
and every repair applied to the deployment at once, and a fresh process is the row
that fixes it. So the loop restarts the serve process on those shapes — but must
keep the *incremental* build's `restartRequired`, because a cold build always says
"restart" and taking it would drop every session per change. An earlier hint had
concluded "nothing outside the process fixes it" — true of the deployment, false of
the process; check which "outside" a negative result actually covered.

**The tools the loop drives have their own idea of what is being watched.**
mxbuild's page plugin globs `web/pages` once, at bundler start, and matches later
events against that snapshot — so a page added while the loop runs is never
bundled, and the apply still reports success. A stand-alone repro of the third
party's watcher (add a second file, watch the next bundle emit only the first)
isolates this in seconds and keeps the diagnosis out of mxcli's code. Two
consequences pull against each other: the bundler's working directory cannot be
moved out of `web/` to dodge a Windows directory lock, because the page plugin
resolves paths relative to that cwd.

**Whoever knows the port must publish it, and the consumers must read it.** The
loop prints hints — "Query data: `mxcli oql -p app.mpr`" — that failed on a
non-default admin port, because `oql` and `log` each resolved the port from
flags, env and `.docker/.env` and never read the handshake the loop had just
written. A hint printed by the process that knows a value has to be runnable by a
reader that does not; reading the published handshake fixes every consumer at
once, where adding the flag to the hint fixes one line.

**A flag that consumes another flag's setup silently degrades alone.**
`--page-check` reuses the Playwright storage state that the `--screenshot-user`
step writes, and the login step was gated on the screenshot flags — so every
secured page was checked as the sign-in form and reported healthy, because a
login page *is* a healthy page. The tell was the title. Where one flag depends on
another's side effect, the dependency belongs in the gate, and the checker itself
should fail on the sign-in form so the same mistake cannot pass twice.

**Two boot paths mean two sets of defaults.** `run --local` and `test --local`
start the app through different functions, each owning its own database
configuration, so `--db-type` existed on one and not the other and a machine
without PostgreSQL could run the app but not the tests. The same shape put two
mxbuild resolvers in one command, where the result of the correct one was passed
only to the bundler and the serve step defaulted. When adding a capability to one
boot path, grep for the other.

**Half-applied is a state the loop can reach.** A watch that redeploys while an
`mxcli exec` is mid-script can serve a partial model, and re-running the script
fixes nothing because the write is idempotent and there is nothing left to write.

**A fallback that succeeds can be the defect.** `--ensure-db` fell back to a new
user-owned cluster when the system one would not start. Once that fallback worked
on Debian, it produced a database that passes every later check (role, database,
boot, health) and is empty. The only point where "new" and "already there" can be
told apart is the moment before `initdb`. So decide there, and announce every
`initdb` loudly. Detect an existing cluster by its data directory, not its
service. For a non-root user the directory is `0700 postgres`, so `EACCES` on
`PG_VERSION` is evidence that a cluster exists. A probe that only counts a
successful stat misses the devcontainer user. Any harness that drops privileges
should print its uid first: a control binary that silently ran as root started
the real cluster and provisioned into it.

**A file other processes read needs an owner.** `.mxcli/run-local.json` was
removed by a deferred cleanup in *every* `run --local` process. That included
`--setup`, which the SessionStart hook runs on every resume and compaction, so a
live detached run lost its handshake. Callers then got a 403 and blamed the
password. The 403 was the clue: it needs a runtime that is up. Grep every remover
of a shared path and ask which processes reach it. Only the process whose pid the
file names may remove it. Reasons follow the same rule as logs (above). The
trial-licence stop is an application log line, so it is only in `runtime.log`,
and only the newest boot's section of that file may be blamed.

**A one-shot that runs Gradle must neither leave a daemon nor wait on one without
a bound.** This is three findings with one cause (#1396, #1340 and an earlier
unbounded mxbuild). Append `-Dorg.gradle.daemon=false` to `GRADLE_OPTS`, never
replace the variable: replacing it drops a proxy setting and becomes mx's
misleading "Failed to find some hosts". Then run the step under the process-tree
watchdog with a progress line. The tell in the report was that Mendix's own `mx`
was fast on the same project: same tool, different state. `mxbuild --serve`
keeps its daemon on purpose.

**A signed-in check is a (host, session) pair, not an app.** `playwright check`
signed in on the handshake's `localhost` and then probed the URL as typed. Cookies
belong to a host, so `127.0.0.1` (which the boot output prints) or a vhost got the
sign-in form on every run, and each retry added another session. Count sessions
(`runtime_statistics`) rather than trusting the verdict.

**A settle rule written for one page holds every page to it.** The probe waits
for no open request because a list view's data source shows nothing else (#845).
The client keeps a `POST /xas/` open behind the sign-in form, so the same rule
spent the whole `--wait` there and then advised raising it. Exempt a state that
is a verdict in itself. And verify *where* the browser ended: an unknown `/p/`
URL is redirected to the home page with HTTP 200, so the final path is the only
signal that does not depend on the app's language.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  probes, guards and controls
- `.claude/skills/mendix/run-local.md` — what the flags actually do
- `.claude/skills/verify-in-runtime.md` — proving a fix in a real browser, which
  is the only check that sees most of this class
- [[the-platform-ci-does-not-run]] — the environments several of these failures
  live in exclusively
