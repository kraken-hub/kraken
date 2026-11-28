# Environment variables

## Runtime

| Variable | Default | Description |
|----------|---------|-------------|
| `KRAKEN_RUNTIME` | auto-detect | Container runtime: `docker` or `podman`. Auto-detects from available sockets if not set. |
| `KRAKEN_HOST` | — | Docker/Podman daemon socket or TCP address (e.g. `unix:///var/run/docker.sock`, `tcp://host:2375`). |
| `KRAKEN_DOCKER_API_VERSION` | `1.41` | Docker API version to use. Set to a specific version (e.g. `1.43`) to pin it. Set to empty (`KRAKEN_DOCKER_API_VERSION=`) to auto-negotiate with the daemon. Minimum supported version is `1.41`. |

## Paths

| Variable | Default | Description |
|----------|---------|-------------|
| `KRAKEN_HOME` | OS data dir (`~/.config/kraken`, `/var/lib/kraken`) | Base directory for all kraken data. |
| `KRAKEN_STORAGE` | `{krakenHome}/storage/fs` | Directory for app registrations and vars storage. |
| `KRAKEN_PLUGINS` | `{krakenHome}/plugins` | Directory where JS plugins are installed. |

## Application

| Variable | Default | Description |
|----------|---------|-------------|
| `KRAKEN_APP` | cwd directory name | Default application name when no `[app]` argument is given. |
| `KRAKEN_ENV` | `dev` | Default environment used when registering an app (`kraken init`, `kraken start --init`) and no `--env` flag is given. |
| `KRAKEN_PROFILE` | — | Default [profile(s)](file-reference.md#profiles) stored when registering an app (`kraken init`, `kraken start --init`) and no `--profile` flag is given, comma-separated (`debug,mail`, `*` for all). |
| `KRAKEN_SHELL` | `/bin/sh` | Shell used for interactive `run` and `exec` when no command is specified. |

## Networks

Used only when kraken creates the global `kraken_net` network. An existing network is left as is: to apply new values, remove the containers on it and the network (`docker network rm kraken_net`), then start the apps again.

| Variable | Default | Description |
|----------|---------|-------------|
| `KRAKEN_NET_SUBNET` | assigned by Docker | Subnet of `kraken_net` in CIDR form (e.g. `172.20.0.0/16`). Needed for [static IPs](file-reference.md#static-ip) on it. Creation fails with "Pool overlaps" if another network already uses the range. |
| `KRAKEN_NET_IP_RANGE` | whole subnet | Part of the subnet that dynamic IPs are taken from, in CIDR form (e.g. `172.20.128.0/17`). Static IPs outside it are never given to another container. Requires `KRAKEN_NET_SUBNET`. Docker uses the first address of the range as the gateway. |

## Output

| Variable | Default | Description |
|----------|---------|-------------|
| `KRAKEN_NO_BANNER` | — | Set to any non-empty value to suppress the ASCII art banner shown when running `kraken` with no arguments. |
| `NO_COLOR` | — | Set to any non-empty value to disable colored output. Follows the [no-color.org](https://no-color.org) standard. |

## Container identity variables

These are automatically injected into every container's environment by kraken:

| Variable | Value |
|----------|-------|
| `KRAKEN_APP` | Application name |
| `KRAKEN_SERVICE` | Service name |

## YAML template variables

These are available as `${VAR}` in `kraken.yaml` (not environment variables, but listed here for completeness):

| Variable | Value |
|----------|-------|
| `APP_NAME` | Application name |
| `APP_PATH` | Absolute path to the app directory |
| `APP_ENV` | Active environment (e.g. `dev`) |

Values from `.env` and `.env.local` in the app directory are also available as template variables (see [file reference](file-reference.md#variable-substitution)).
