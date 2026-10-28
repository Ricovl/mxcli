# `.mxcli/run-local.json` — the handshake, and what its absence means

`run-state.json` says what the run is doing; **`run-local.json`** (0600) is how other
commands *reach* the app: its app and admin ports, the admin password and the boot
configuration. `mxcli oql`, `mxcli log` and `constant set --apply` read it; tools that
call the admin API themselves should too.

| When | What happens to it |
|---|---|
| the app serves (boot done) | **written** by the run, with the run's pid |
| the run ends — `Ctrl-C`, `run stop`, or the **runtime stopped on its own** (trial-licence time limit, crash, kill) | **removed** by that run, right after `run-state.json` records `phase: stopped`, `ended` and the `reason` |
| `run stop` on a run killed with `-9` | removed by `run stop` |
| any *other* mxcli process exits (`run --local --setup` from the SessionStart hook, a refused second run) | **untouched** — only the run whose pid is in it may remove it |

## Missing while you believed the app was up?

Run `mxcli run status -p app.mpr` before suspecting credentials. It says one of:

- `stopped: trial licence time limit reached at 18:43:16 (runtime.log); restart with
  mxcli run restart -p …` — the reason comes from `.mxcli/runtime.log` (the newest
  boot's section) when it gives one: the licence limit, an `OutOfMemoryError`, else
  the last `ERROR`/`CRITICAL` line, reported as exactly that;
- `stopped: pid N exited without cleaning up …; its runtime log says: …` — the run
  was killed outright, so it could not record its end;
- `running: … but .mxcli/run-local.json is MISSING` — the app is up and only the
  file is gone; `mxcli run restart` publishes it again.

A 401/403 from the admin API with no handshake means the connection fell back to
default settings (environment, `.docker/.env`, `mxcli-local-dev`), not that a
password is wrong; `oql` and `log` say so and quote `run status`.

Before ako/mxcli#1542, every `run --local` process removed the file on exit whether
or not it had written it — so the SessionStart hook's `--setup` run deleted a live
run's handshake on every session start, resume or compaction.
