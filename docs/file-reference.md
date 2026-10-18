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
  app:
    image: myapp
    ~dev,test:
      env:
        DEBUG: "1"
    ~prod:
      restart: always
```

The `~key` is a comma-separated list of environment names. For `dev,test` mode both `~dev` and `~dev,test` sections match. The matched section is merged into the parent and the `~key` is removed.

For the example above, in `prod` mode the final config will include the `cache` and `worker` services, and `app` will have `restart: always`. Where a merged section lands relative to existing keys follows the source: `~prod` is written before `services:`, so `cache`/`worker` are merged in before `app`; a `~prod` block written after `services:` would instead merge its entries in after.

To split prod-only additions into more than one block, YAML won't allow two `~prod:` keys at the same level — append `#<tag>` (e.g. `~prod#before`, `~prod#after`); anything from `#` onward is ignored when matching, it just makes the key unique.

```yaml
~prod#before:
  services:
    cache:
      image: redis

services:
  app:
    image: myapp

~prod#after:
  services:
    worker:
      image: myapp
      scale: 2
```

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
  - redis:container:redis     # resolve container IP at create time
```

The `hostname:container:name` form looks up the IP of container `name` on the global `kraken_net` network, falling back to any network the container is attached to.

Entries support brace expansion — alternation and numeric ranges are expanded at parse time:

```yaml
extra_hosts:
  - "{alpha,beta,gamma}.app.lh:container:proxy"
  - "node{1..16}.app.lh:container:proxy"
```

---

### dns

Custom DNS servers for the container. Accepts static IPs or `container:<name>` to resolve a running container's IP at create time.

```yaml
dns:
  - 8.8.8.8
  - container:dns
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
```

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
  - php                         # mounts volumes from the `php` service container
  - php:ro                      # read-only
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
  service: php
```

Scale expansion in the base file runs before the `service:` lookup, so if the referenced service has `scale: N`, it no longer exists under its original name — reference a specific replica by its expanded name instead (`service: php1`, `service: php2`, ...). Referencing the pre-expansion name of a scaled service returns an error.

With JS merge function (same signature as `extend`):

```yaml
extends:
  file: ../base/kraken.yaml
  service: php
  fn: !!js/function |
    ({ local, service }) => ({ ...service, ...local })
```

---

### scale `experimental`

```yaml
scale: 3
```

Expands the service into N numbered instances: `service1`, `service2`, `service3`. The value `1`, `false`, or omitted means no scaling. Supports variable substitution: `${WORKER_COUNT-1}`.

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
