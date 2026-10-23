# mxcli docker check

The `mxcli docker check` command validates a Mendix project without building it. It runs the Mendix `mx check` tool against the project and reports errors, warnings, and deprecations.

## Usage

```bash
mxcli docker check -p app.mpr
```

## What It Does

1. **Resolves the `mx` binary** -- Takes the `mx` next to `--mxbuild-path`, else the first it finds on PATH, in a Studio Pro installation, or in the download cache at `~/.mxcli/mxbuild/<version>/modeler/mx`. It does not download one (see [Getting mx](#getting-mx))
2. **Runs `mx check`** -- Executes the Mendix project checker against a temporary copy of the `.mpr` file, as a local process; despite the `docker` in its name, it does not use Docker
3. **Reports results** -- Outputs errors, warnings, and deprecations to the console

If the project has errors, the command exits with a non-zero status code.

Before `mx check` it also warns, without failing, about two problems `mx check`
cannot see: an MPR v2 `ContentsHash` index that disagrees with the `.mxunit`
files (`mxcli fix hashes`), and git states that make Studio Pro 11.13 fail to
open the project. See [Working Outside Studio Pro](outside-studio-pro.md).

## Getting mx

`docker check` does not download MxBuild. With no `mx` to find it stops with
`mx not found` and names the command that sets one up:

```bash
# Once per Mendix version: download MxBuild (including mx) for the project's version
mxcli setup mxbuild -p app.mpr

# Then check
mxcli docker check -p app.mpr
```

The downloaded mxbuild is cached at `~/.mxcli/mxbuild/{version}/` for reuse.
`mxcli new` and `mxcli docker build` download it too. On macOS and Windows,
`setup mxbuild` prefers Studio Pro's bundled MxBuild, because the CDN only
publishes Linux binaries.

The `mx` found need not be the project's version: an `mx` on PATH, or any
cached version, is used when the project's own is not there. `mx check` accepts
another patch of the same minor; `mx update-widgets` does not and the check then
runs without it (see `mxcli docker check --help`). Pass `--mxbuild-path
~/.mxcli/mxbuild/<version>` to choose one.

## Example Output

A clean project:

```
Using mx: /home/vscode/.mxcli/mxbuild/11.6.3.12345/modeler/mx
Checking project /workspaces/app/app.mpr...
The app contains: 0 errors.
Project check passed.
```

A project with errors:

```
Using mx: /home/vscode/.mxcli/mxbuild/11.6.3.12345/modeler/mx
Checking project /workspaces/app/app.mpr...
CE0001: Entity 'MyModule.Customer' has no access rules configured.
CE0463: Widget definition changed for 'DataGrid2'.
The app contains: 2 errors.
project check failed: exit status 1
```

## When to Use

`mxcli docker check` is faster than a full build (`mxcli docker build`) because it only validates the project structure without compiling or packaging. This makes it useful for:

- **Quick validation** after making MDL changes
- **Pre-build gate** -- `mxcli docker build` runs `mx check` automatically before building (use `--skip-check` to bypass)
- **CI pipelines** -- fail fast on invalid projects before spending time on a full build
- **Iterative development** -- check after each MDL script execution to catch errors early

## Integration with mxcli docker build

The `mxcli docker build` command runs `mx check` automatically as a pre-build step. If the check fails, the build is aborted. You can skip this with:

```bash
mxcli docker build -p app.mpr --skip-check
```

## Integration with the TUI

When using mxcli in interactive REPL mode, the TUI can auto-check the project on file changes, giving immediate feedback on whether MDL modifications introduced errors. Like `docker check`, it runs `mx check` on a temporary copy, so the project's `theme-cache/` and `deployment/` are not touched; it does not run update-widgets, so it checks the project as stored. `mxcli eval`'s `mx_check` check does the same.

## Related Pages

- [Docker Build](docker-build.md) -- Building a Mendix application with PAD patching
- [Docker Run](docker-run.md) -- Building and running the app in Docker
- [Testing](testing.md) -- MDL test framework
- [Linting](linting.md) -- MDL lint rules
