# Plugins

Kraken plugins are JavaScript (ES6) modules loaded at startup. They can listen to lifecycle events and modify container configuration.

---

## Directory structure

```
my-plugin/
├── kraken.json    # manifest (required)
└── main.js        # entry point
```

### kraken.json

```json
{
    "name": "my-plugin",
    "version": "1.0.0",
    "main": "main.js"
}
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | — | Plugin name, used as install directory name |
| `version` | yes | — | Version string |
| `main` | no | `main.js` | Entry point file |

---

## Entry point

The entry point must export a default function that receives the `kraken` object:

```js
export default function doInit(kraken) {
    kraken.on('container:create', (event) => {
        const cfg = event.data;
        // modify cfg
    });
}
```

---

## kraken API

### `kraken.on(event, callback)`

Register a handler for a lifecycle event. The callback receives an event object with a `data` property containing the event payload. Mutations to `data` are applied before the core action runs.

### `kraken.pluginDir`

Absolute path to the plugin's own directory. Use it to reference bundled files:

```js
const bin = kraken.pluginDir + '/bin/mytool';
```

---

## Native modules

### `kraken/vars`

Access the kraken variable store (managed via `kraken vars:set` / `kraken vars:unset`):

```js
import vars from 'kraken/vars';

vars.get('SSH_AUTH_SOCK')   // string | undefined
vars.all()                  // { [key: string]: string }
```

### `kraken/env`

Access environment variables from the process environment and kraken env files:

```js
import env from 'kraken/env';

env.get('KRAKEN_PLUGIN_MY_OPTION')   // string | null
```

### `kraken/ctx`

Access kraken context:

```js
import ctx from 'kraken/ctx';

ctx.home   // absolute path to krakenHome
```

---

## Events

### `container:create`

Fired before each container is created. Mutate `event.data` to customise the container.

```js
kraken.on('container:create', (event) => {
    const cfg = event.data;

    cfg.Volumes = [...(cfg.Volumes || []), {
        Source: '/host/path',
        Target: '/container/path',
        ReadOnly: true,
    }];

    cfg.VolumesFrom = [...(cfg.VolumesFrom || []), 'my-container:ro'];
});
```

**Modifiable `event.data` fields:**

| Field | Type | Description |
|-------|------|-------------|
| `Env` | object | Environment variables `{ KEY: value }` |
| `Volumes` | array | `{ Source, Target, ReadOnly }` |
| `VolumesFrom` | array | `["container", "container:ro"]` |
| `ExtraHosts` | array | `["hostname:ip"]` |
| `UserLabels` | object | User-defined labels |

---

Plugins are installed to `{KRAKEN_PLUGINS}/` (see [env vars](env-vars.md#paths)).
