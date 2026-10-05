# Working Outside Studio Pro

A project edited with mxcli, git and an editor is sometimes opened in Studio Pro
afterwards. Two kinds of state that Studio Pro depends on are invisible to
`mx check`, and mxcli checks both.

## The ContentsHash index: `mxcli fix hashes`

In an MPR v2 project (Mendix 10.18 and later) the `.mpr` file indexes every
`mprcontents/**/*.mxunit` file by its hash: `Unit.ContentsHash` is
`base64(SHA-256(file))`. mxcli's writer and Studio Pro keep the two in step.
Anything else that changes a `.mxunit` does not: restoring a unit with
`git checkout`, `git restore`, a revert or a merge leaves the index describing
bytes that are no longer on disk. `mx check` and MxBuild read the files and do
not notice; Studio Pro uses the index to decide what changed.

```bash
mxcli fix hashes -p app.mpr            # verify; exits 1 when something is off
mxcli fix hashes -p app.mpr --repair   # rewrite mismatched hashes from the files
```

| Reported | Meaning | `--repair` |
|----------|---------|------------|
| `MISMATCH` | the file's hash differs from the stored one | rewrites the stored hash from the file |
| `MISSING` | the `.mpr` indexes a unit whose file does not exist | reported only — restore the file |
| `ORPHAN` | a `.mxunit` file no unit in the `.mpr` indexes | reported only — `mxcli diag --check-units --fix` removes it |

The repair treats the files as the source of truth and never changes them. It
updates every hash in one SQLite transaction, and, like every mxcli write, is
refused while Studio Pro has the project open (the `.mpr.lock` beside it). An
MPR v1 project keeps unit contents inside the `.mpr`, has no index to drift,
and the command says so.

`mxcli exec` and `mxcli docker check` run the verify too and print a one-line
warning when it finds a mismatch or a missing file. It never fails them;
measured on a 900-unit project, the whole verify takes about 0.2 s.

## Git states that crash Studio Pro

Studio Pro 11.13 fails to open a project with *"Unable to find 'system'
property in 'system'"* when the project folder is a git repository and

- the checked-out branch has **no upstream** — remedy:
  `git push -u origin <branch>` before opening the project (or
  `git remote add origin <url>` first, if the repository has no remote); or
- git reports **"detected dubious ownership"** — typical for a folder shared
  between the host and a devcontainer, owned by a different user on each side.
  Remedy, on the machine that runs Studio Pro:
  `git config --global --add safe.directory <path>`.

`mxcli docker check`, `mxcli run --local` and `mxcli diag -p app.mpr` warn about
both. The warnings are advice and never fail a command. They are silent:

- outside a git repository, or when git is not installed;
- on a **detached HEAD** — how CI systems and `git checkout <sha>` leave a
  repository; there is no branch to push, and nobody opens such a checkout in
  Studio Pro;
- when the `CI` environment variable is set, or `MXCLI_NO_GIT_WARNINGS=1`.

Dubious ownership is judged by the git mxcli runs. Inside a devcontainer that is
the container's git, which may see ownership differently from the host's Studio
Pro — so a clean report from inside the container does not rule it out on the
host.

## One report: `mxcli diag -p`

```bash
mxcli diag -p app.mpr
```

prints mxcli's diagnostics followed by a *Project* section with both checks.
