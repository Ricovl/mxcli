# Docker Integration

mxcli provides Docker integration for building, running, and validating Mendix applications without a local Mendix installation. Docker is also required for the testing framework.

## Features

| Command | Description |
|---------|-------------|
| `mxcli docker build` | Build a Mendix application with mxbuild in Docker, including PAD patching |
| `mxcli docker run` | Run a Mendix application in a Docker container |
| `mxcli docker check` | Validate a project with `mx check`, run locally (does not download mxbuild or use Docker; see [Getting mx](docker-check.md#getting-mx)) |
| `mxcli test` | Run test files (uses Docker for `mx create-project` and `mx check`) |
| `mxcli oql` | Query a running Mendix runtime via M2EE admin API |

## Prerequisites

- Docker (or Podman 4.7+) must be installed and running
- The container runtime must be accessible from the command line

### Podman Support

mxcli auto-detects Docker or Podman on PATH. To explicitly select:

```bash
export MXCLI_CONTAINER_CLI=podman
```

Podman 4.7+ ships `podman compose` with full Docker Compose V2 compatibility. All `mxcli docker` subcommands work with both runtimes.

## The mx Tool

The `mx` command-line tool validates and builds Mendix projects. mxcli can auto-download the correct version:

```bash
# Auto-download mxbuild for the project's Mendix version
mxcli setup mxbuild -p app.mpr
```

The mx binary location depends on the environment:

| Environment | Path |
|-------------|------|
| Dev container | `~/.mxcli/mxbuild/{version}/modeler/mx` |
| Repository | `reference/mxbuild/modeler/mx` |

## Quick Start

```bash
# Once per Mendix version: download MxBuild (docker check does not download it)
mxcli setup mxbuild -p app.mpr

# Validate a project
mxcli docker check -p app.mpr

# Build a deployable package
mxcli docker build -p app.mpr
```

## Related Pages

- [mxcli docker build](docker-build.md) -- Building with PAD patching
- [mxcli docker run](docker-run.md) -- Running applications
- [OQL Queries](oql.md) -- Querying running applications
- [Dev Container Setup](devcontainer.md) -- Development environment
