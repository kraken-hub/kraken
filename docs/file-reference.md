# Kraken file reference

Configuration file for kraken applications. By default kraken looks for `kraken.yaml` in the application directory.

---

## Top-level structure

```yaml
services:
  <name>: <service>

volumes:
  <name>: <volume>
```

---

## Environments

Keys prefixed with `~` are environment-scoped and merged only when the app environment matches. This works at any nesting level — inside a service, or at the top level to toggle entire services.

```yaml
~prod:
  services:
    cache:
      image: redis
    worker:
      image: myapp
      scale: 5

services:
  api:
    image: myapp
    ~dev,test:
      env:
        DEBUG: "1"
    ~prod:
      restart: always
```

The `~key` is a comma-separated list of environment names. For `dev,test` mode both `~dev` and `~dev,test` sections match. The matched section is merged into the parent and the `~key` is removed.

For the example above, in `prod` mode the final config will include the `cache` and `worker` services, and `api` will have `restart: always`. Where a merged section lands relative to existing keys follows the source: `~prod` is written before `services:`, so `cache`/`worker` are merged in before `api`; a `~prod` block written after `services:` would instead merge its entries in after.

To split prod-only additions into more than one block, YAML won't allow two `~prod:` keys at the same level — append `#<tag>` (e.g. `~prod#before`, `~prod#after`); anything from `#` onward is ignored when matching, it just makes the key unique.

```yaml
~prod#before:
  services:
    cache:
      image: redis

services:
  api:
    image: myapp

~prod#after:
  services:
    worker:
      image: myapp
      scale: 2
```

Anchors defined inside a `~env` section that doesn't match are still usable — aliases to them elsewhere in the file keep their value after the section is dropped.

---

## Extension fields

Top-level keys starting with `x-` are removed before the config is used — they exist only to hold YAML anchors. They can be aliased directly, merged with `<<:`, and can alias each other.

```yaml
x-common-env: &COMMON_ENV
  LOG_LEVEL: info

services:
  api:
    env: *COMMON_ENV
  worker:
    env:
      <<: *COMMON_ENV
      QUEUE: default
```

Only top-level keys are treated this way; an `x-` key under `services:` is an ordinary service name. An `x-` key merged in from a matching top-level `~env` section is removed as well.

---

## Services

### image

```yaml
image: nginx:1.24
```

Image to use. If omitted and `build` is present, the image name is auto-generated from the app and service names. If `image` is specified together with `build`, the built image is tagged with that name.

---

### pull_policy

```yaml
pull_policy: missing   # always | missing | never (default: missing)
```

Controls when the image is pulled or the base image is refreshed:

| Value | Behavior |
|-------|----------|
| `always` | Always pull before use. When `build:` is present, pulls the `FROM` base image before every build. |
| `missing` | Pull only if the image is not available locally **(default)** |
| `never` | Never pull; fail if the image is not available locally |

---

### build

Three forms are supported.

**String** — path to build context:

```yaml
build: ./app
```

**Mapping:**

```yaml
build:
  context: ./app              # build context path (default: app directory)
  file: Dockerfile            # Dockerfile path within context (default: Dockerfile)
  file_inline: |              # inline Dockerfile content (alternative to file)
    FROM alpine
    RUN apk add curl
  args:                       # build args — mapping or list of KEY=value
    NODE_ENV: production
  target: release             # multi-stage build target
  policy: missing             # always | missing — when to rebuild (default: missing)
  no_cache: false             # disable layer cache (default: false)
  tags:                       # additional image tags
    - myapp:1.0
    - registry.example.com/myapp:latest
  labels:                     # image labels
    app: myapp
  cache_from:                 # images to use as cache source
    - myapp:latest
  shm_size: 128mb             # /dev/shm size
  network: host               # network mode during build
```

`file` and `file_inline` are complementary: `file` sets the Dockerfile path in the context, `file_inline` provides the content. When both are set, the inline content is written to the `file` path. When only `file_inline` is set, it is placed at `Dockerfile`.

`args` can also be a sequence. Keys without `=` are resolved from the host environment:

```yaml
build:
  args:
    - NODE_ENV=production
    - SECRET_TOKEN            # taken from host $SECRET_TOKEN
```

`context` can be a sequence to mix a path with inline files:

```yaml
build:
  context:
    - ./app                   # first scalar is the build context path
    - Dockerfile: |
        FROM node:22
        COPY . .
```

**Sequence** — inline build context (no filesystem path):

```yaml
build:
  - Dockerfile: |
      FROM node:22
      COPY ./app.js /app/index.js
  - ./app.js: |
      const http = require('http')
      ...
  - ./run.sh:
      mode: 775               # octal file mode (default: 0644)
      content: |
        #!/bin/sh
        node /app/index.js
```

The `Dockerfile` key sets the inline Dockerfile. All other keys become files in the build context.

#### `image` + `build` combinations

| `image` | `build` | Behavior |
|---------|---------|----------|
| ✅ | ❌ | Pull image according to `pull_policy` |
| ❌ | ✅ | Build image; `pull_policy` controls whether to pull the `FROM` base image |
| ✅ | ✅ | Build and tag as `image`; `pull_policy` controls the `FROM` base image |

#### Always rebuild with a fresh base image

```yaml
pull_policy: always
build:
  context: .
  policy: always
```

---

### platform

```yaml
platform: linux/amd64
```

Forces the container platform (e.g. `linux/amd64` on an ARM host). Applied to image pull, image build, and container creation.

---

### entrypoint

```yaml
entrypoint: /docker-entrypoint.sh
```

Overrides the image `ENTRYPOINT`. A multiline string is wrapped as `["/bin/sh", "-c", script]`.

---

### cmd

```yaml
cmd: nginx -g 'daemon off;'
```

Overrides the image `CMD`. Same multiline handling as `entrypoint`.

---

### workdir

```yaml
workdir: /app
```

Working directory inside the container.

---

### hostname

```yaml
hostname: myservice
```

Container hostname. Also registered as a network alias on the app network.

---

### network_mode

Selects the container's network stack. When unset (or `bridge`), the container is attached to the `kraken_net` and `kraken_net.{app}` networks as usual.

```yaml
network_mode: host            # use the host's network stack
network_mode: none            # loopback only
network_mode: service:vpn     # share the network stack of this app's `vpn` service container
network_mode: container:vpn   # share the network stack of the container literally named `vpn`
```

With any mode other than `bridge`, the container is **not** attached to the kraken networks: other services can't reach it by service name, and `service:`/`container:` references to it in `dns`/`extra_hosts` won't resolve.

- `host`, `none` — `ports` are ignored (with a warning).
- `service:<name>`, `container:<name>` — the two containers share one IP and `localhost`. `ports`, `hostname`, `dns`, `dns_search`, `dns_opt` and `extra_hosts` belong to the shared stack and are an error on this service. The target container must exist when this one is created and be running when it starts, so define it earlier in the file. Recreating the target (`rebuild`) leaves this container without a network until it is recreated too.
- Any mode other than `bridge` can't be combined with [`networks`](#networks).

---

### networks

Extra networks the service joins, **in addition to** the default ones (`bridge`, `kraken_net`, `kraken_net.{app}`). Each must be declared in the [top-level `networks:`](#networks-top-level) section.

Sequence form:

```yaml
networks:
  - backend
  - shared
```

Mapping form, with extra DNS names on that network:

```yaml
networks:
  shared:
    aliases: [api.example.lh]
  backend:
```

On extra networks the container is resolvable by its container name (`api.my-app`) and its `aliases` — not by the bare service name, so services of different apps don't collide on a shared network.

#### Static IP

`ipv4_address` gives the container a fixed IP on a network. The global `kraken_net` can be listed here without a top-level declaration, so other containers can rely on the address, e.g. as their [`dns`](#dns) server:

```yaml
networks:
  kraken_net:
    ipv4_address: 172.20.0.53
  shared:
    ipv4_address: ${SHARED_IP-}     # empty = assigned by the network
```

- The network must have a subnet: for `kraken_net` set [`KRAKEN_NET_SUBNET`](env-vars.md#networks) before kraken creates it; an external network needs one from its creator. Otherwise the container is not connected and is removed again.
- Pick an address outside the network's dynamic range (`KRAKEN_NET_IP_RANGE` for `kraken_net`), so no other container gets it while this one is being recreated.
- An address already in use is an error (so a service with [`scale`](#scale-experimental) can't share one).
- `kraken run` ignores `ipv4_address`, since the service container may be holding the address; `kraken run --ip <address>` sets one on `kraken_net` explicitly.

---

### env_file

Load environment variables from one or more shell files. Supports variable expansion within the file.

```yaml
env_file: .env

env_file:
  - .env
  - .env.local
```

File format:

```sh
# comments are ignored
BASE_URL=https://example.com
API_URL=${BASE_URL}/api     # variable expansion is supported
export SECRET=abc123        # export keyword is optional
```

Paths are resolved relative to the app directory. If a file does not exist it is silently skipped. Values from `env:` always take precedence over `env_file:`.

---

### env

Mapping form:

```yaml
env:
  NODE_ENV: production
  SECRET_TOKEN:               # null value — resolved from host $SECRET_TOKEN
  DEBUG: "1"
```

Sequence form:

```yaml
env:
  - NODE_ENV=production
  - SECRET_TOKEN              # resolved from host $SECRET_TOKEN
  - DEBUG=1
```

---

### labels

```yaml
labels:
  app.version: "1.0"
  com.example.enable: "true"
  com.example.config:         # nested values are JSON-encoded as strings
    foo: bar
    items:
      - ONE
      - TWO
```

---

### ports

Mapping form:

```yaml
ports:
  "80": "8080"                # host 80 → container 8080/tcp
  "127.0.0.1:443": "443"
```

Sequence form:

```yaml
ports:
  - "80"                      # expose without binding
  - "80:8080"                 # host:container
  - "127.0.0.1:80:8080"       # ip:host:container
  - "5000/udp"                # specify protocol (default: tcp)
```

---

### extra_hosts

```yaml
extra_hosts:
  - mysql:10.0.0.5            # static ip
  - redis:service:redis       # IP of this app's `redis` service container
  - proxy:container:proxy     # IP of the container literally named `proxy`
```

The `hostname:service:name` and `hostname:container:name` forms resolve the container's IP at create time (see [Container references](#container-references)), looking it up on the global `kraken_net` network and falling back to any network the container is attached to.

Entries support brace expansion — alternation and numeric ranges are expanded at parse time:

```yaml
extra_hosts:
  - "{alpha,beta,gamma}.app.lh:container:proxy"
  - "node{1..16}.app.lh:service:proxy"
```

---

### dns

Custom DNS servers for the container. Accepts static IPs, or a [container reference](#container-references) (`service:<name>` / `container:<name>`) to resolve a running container's IP at create time.

```yaml
dns:
  - 8.8.8.8
  - service:dns               # this app's `dns` service container
  - container:dns-global      # the container literally named `dns-global`
```

---

### dns_search

DNS search domains.

```yaml
dns_search:
  - example.com
  - internal.example.com
```

---

### dns_opt

DNS resolver options passed to the container's `resolv.conf`.

```yaml
dns_opt:
  - ndots:5
  - timeout:2
```

---

### volumes

Mapping form:

```yaml
volumes:
  /host/path: /container/path   # absolute bind mount
  ./data: /app/data             # relative to app directory
  .dotfile: /container/.dotfile # dotfile, relative to app directory
  mydata: /app/data             # named volume (see top-level volumes:)
  logs: /app/logs               # local volume under kraken volumes root
  /app/cache:                   # anonymous volume (null target)
```

Sequence form:

```yaml
volumes:
  - /app/cache                  # anonymous volume
  - /opt/data:/var/lib/mysql    # absolute bind mount
  - ./cache:/tmp/cache          # relative to app directory
  - ./conf:/etc/app:ro          # read-only
```

A trailing `:ro` (read-only) or `:rw` (read-write, the default) on the target is accepted in both forms, e.g. `./run.sh: /usr/bin/run.sh:ro`.

Resolution order for the source:
1. Absolute path → used as-is (bind mount; host directory is created if it does not exist)
2. Starts with `./` or `../` → resolved relative to app directory (bind mount; host directory is created if it does not exist)
3. Contains `/` or starts with `.` (e.g. `.dotfile`) → resolved relative to app directory (bind mount; host directory is created if it does not exist)
4. Matches a name in top-level `volumes:` with `driver`/`driver_opts` → `kraken_vol.{app}.{vol}`
5. Matches a name in top-level `volumes:` with `external` → external volume name used as-is
6. Otherwise → managed volume `kraken_vol.{app}.{service}.{md5(path)}`

An existing host file or directory is mounted as-is, so single files can be bind-mounted.

---

### volumes_from

```yaml
volumes_from:
  - data                        # mounts volumes from the `data` service container
  - data:ro                     # read-only
  - container:mybox             # literal container name
  - container:mybox:ro
```

---

### logging

```yaml
logging:
  driver: json-file
  options:
    max-size: 10m
    max-file: "3"
```

---

### restart

String form:

```yaml
restart: unless-stopped
```

Mapping form:

```yaml
restart:
  name: on-failure
  max_retry: 5
```

---

### wait

```yaml
wait: 5s
```

How long to wait after the container starts before proceeding to the next service. Accepts Go duration strings: `5s`, `1m`, `1m30s`, `1h`.

The container's stdout/stderr is streamed during this time. If the container prints a line matching `@kraken wait <duration>`, that duration is used instead of the configured value:

```yaml
wait: 10m
cmd: |
  #!/bin/sh
  do_slow_init
  echo "@kraken wait 3s" # cuts the wait to 3s from this point
  exec myapp
```

---

### extend

Inherit from another service defined in the same file. `ports` and `scale` are **not** inherited from the base.

Simple form:

```yaml
extend: base-service
```

With key filter:

```yaml
extend: base-service&env,volumes    # include only these keys
extend: base-service!ports,labels   # exclude these keys
```

Mapping form:

```yaml
extend:
  service: base-service
  type: exclude                     # or: include
  keys: [ports, labels]
```

With JS merge function — receives `{ local, service }` where `local` is the current service config and `service` is the base:

```yaml
extend:
  service: base-service
  fn: !!js/function |
    ({ local, service }) => ({ ...service, ...local })
```

---

### extends

Inherit from a service in an external file. Unlike `extend`, `ports` **is** inherited from the base.

```yaml
extends:
  file: ../base/kraken.yaml
  service: base-service
```

Relative paths in an extended file are relative to that file, not to the app directory:

- `file:` of an `extends` resolves against the directory of the file that contains it (also inside an extended file);
- `build` (path, `context`, first item of a `context` list), `env_file` and bind-mount `volumes` sources of its services resolve against the extended file's directory. A `build` mapping without `context` uses that directory as context. Named volumes, container-only volumes and container paths (`cmd`, `workdir`, ...) are not paths on the host and stay as they are.

Absolute paths, e.g. `${APP_PATH}/...`, are used as is. `kraken config` shows the rebased paths relative to the app directory:

```yaml
# services/web/kraken.yaml
services:
  web:
    build: ../../docker/web     # → ./docker/web
    env_file: web.env           # → ./services/web/web.env
    volumes:
      ./data: /data             # → ./services/web/data
      cache: /cache             # named volume, unchanged
```

The external file is processed like the app's own config: `${VAR}` values come from the same sources (`APP_NAME`, `APP_PATH`, `APP_ENV`, `.env.local`, `.env`, host env) and `~env` sections in it match the app's environment.

Scale expansion in the base file runs before the `service:` lookup, so if the referenced service has `scale: N`, it no longer exists under its original name — reference a specific replica by its expanded name instead (`service: base-service1`, `service: base-service2`, ...). Referencing the pre-expansion name of a scaled service returns an error.

With JS merge function (same signature as `extend`):

```yaml
extends:
  file: ../base/kraken.yaml
  service: base-service
  fn: !!js/function |
    ({ local, service }) => ({ ...service, ...local })
```

---

### profiles

Makes a service optional. A service without `profiles` is always started; a service with profiles only when one of them is active.

```yaml
services:
  api:
    image: myapp                    # always
  adminer:
    image: adminer
    profiles: [debug]               # only with the debug profile
  mailer:
    image: mailer
    profiles: [debug, mail]         # with debug or mail
```

Activate profiles with `--profile` on `create`, `start` and `rebuild` (repeatable or comma-separated, `*` for all). Without the flag, the app's stored profile is used — set at registration (`kraken init --profile debug`, `kraken start --init "" --profile debug` when it registers the app, or the `KRAKEN_PROFILE` env var) and changed with `kraken config:context --add-profile mail` / `--remove-profile debug`.

| Command | Optional services included |
|---------|----------------------------|
| `kraken start my-app` (no stored profile) | those whose container already exists |
| `kraken rebuild my-app` (no stored profile) | those whose container existed before the rebuild |
| `kraken start my-app` (stored profile `debug`) | exactly the `debug` services, same as `--profile debug` |
| `kraken start my-app --profile debug` | exactly the `debug` services — existing containers of other profiles are not started |
| `kraken rebuild my-app --profile mail` | exactly the `mail` services — other optional containers are removed and not recreated |
| `kraken start adminer.my-app` | `adminer`, whatever its profiles (a service named explicitly is always included) |

`stop`, `restart` and `remove` act on existing containers and ignore profiles.

`run` ignores profiles too, so a service under a profile you never activate is a **run-only service**: it is never started by `start`/`create`/`rebuild`, but `kraken run` can use it. This is useful for tools and one-off tasks that need their own image, entrypoint, env or mounts:

```yaml
services:
  api:
    image: myapp
  cli:
    extend: api
    profiles: [tools]
    entrypoint: [bin/console]
  db-client:
    image: mysql:8
    profiles: [tools]
```

```bash
kraken run cli.my-app -- cache:clear                        # bin/console cache:clear
kraken run cli.my-app --entrypoint "" -- sh                 # shell in the same setup
kraken run db-client.my-app -- mysql -h mysql -u root -p
```

Arguments after `--` replace the service's `cmd`, so put a fixed command prefix in `entrypoint`, not `cmd`.

It will still be started by `--profile '*'` (or a stored profile `*`), and by naming it explicitly (`kraken start cli.my-app`).

---

### scale `experimental`

```yaml
scale: 3
```

Expands the service into N numbered instances: `service1`, `service2`, `service3`. The value `1`, `false`, or omitted means no scaling. Supports variable substitution: `${WORKER_COUNT-1}`.

---

### Container references

`network_mode`, `dns` and `extra_hosts` refer to other containers with a prefix:

| Form | Refers to | Example (app `my-app`) |
|------|-----------|------------------------|
| `service:<name>` | the container of service `<name>` in the same app | `service:mysql` → `mysql.my-app` |
| `container:<name>` | a container by its literal name (any app, or not managed by kraken) | `container:mysql.other-app` |

`volumes_from` uses a bare service name instead (`- data`), with `container:<name>` for a literal container name.

---

## Volumes (top-level)

```yaml
volumes:
  mydata:                           # Docker-managed volume (auto-created)
    driver: local
    driver_opts:
      type: none
      device: /data/myapp
      o: bind
    labels:
      app: myapp

  cache:                            # another Docker-managed volume
    driver: local

  uploads:                          # external volume (must exist)
    external: true

  uploads-prod:                     # external volume with explicit name
    external:
      name: uploads_production
```

Top-level volumes with `driver` or `driver_opts` are managed by kraken (created on start, removed on `remove --purge`). They are named `kraken_vol.{app}.{volName}` and carry labels `kraken.app` and `kraken.target` (YAML vol name). Volumes marked `external` are never created or removed by kraken. Volumes not listed here that are referenced in service `volumes:` are auto-created as managed volumes named `kraken_vol.{app}.{service}.{md5(path)}` with labels `kraken.app`, `kraken.service`, and `kraken.target` (original container path).

---

## Networks (top-level)

Every container is always on Docker's default `bridge` network, the global `kraken_net` (where it is resolvable by container name, e.g. `mysql.my-app`) and the per-app `kraken_net.{app}` (resolvable by service name, e.g. `mysql`). Networks declared here are **extra** networks that services can join with [`networks`](#networks).

```yaml
networks:
  backend:                          # app network (auto-created)

  isolated:                         # app network with settings
    driver: bridge
    driver_opts:
      com.docker.network.bridge.name: br-isolated
    internal: true                  # no outbound traffic
    labels:
      team: core

  shared:                           # external network (must exist)
    external: true

  edge:                             # external network with explicit name
    external: true
    name: edge_net
```

App networks are named `kraken_net.{app}.{name}`, carry labels `kraken.app` and `kraken.network` (YAML network name), are created on start and removed (together with `kraken_net.{app}`) on `remove --purge`. Settings only apply when the network is created — change them on an existing network with `remove --purge` or by removing the network manually. Networks marked `external` are never created or removed by kraken; `external: {name: ...}` is accepted too. The names `default` and `kraken_net` are reserved.

`kraken_net` itself is configured through [environment variables](env-vars.md#networks) (subnet, dynamic IP range), since it is shared by all apps.

---

## Variable substitution

Variable references in scalar values are substituted before parsing.

Syntax: `${VAR}` or `${VAR-default}`.

```yaml
image: myapp:${TAG-latest}          # $TAG or "latest" if unset
scale: ${WORKER_COUNT-1}
env:
  APP_PATH: ${APP_PATH}
```

The following variables are always available:

| Variable | Value |
|----------|-------|
| `APP_NAME` | application name |
| `APP_PATH` | absolute path to the app directory |
| `APP_ENV` | active environment (e.g. `dev`) |

Variables are resolved in this order:
1. `APP_NAME`, `APP_PATH`, `APP_ENV` (above)
2. `.env.local` in the app directory
3. `.env` in the app directory
4. Host environment (`os.Getenv`)

`.env` and `.env.local` support variable expansion within the file:

```sh
BASE_URL=https://example.com
API_URL=${BASE_URL}/api
```

These files only affect YAML template substitution — they are not automatically injected into containers. Use `env_file:` to pass file variables to containers.
