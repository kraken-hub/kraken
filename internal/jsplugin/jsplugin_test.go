package jsplugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newTestKraken() *app.Kraken {
	return app.New("kraken-test", "0.0.0-test")
}

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		wantName    string
		wantVersion string
		wantMain    string
		wantErr     bool
	}{
		{
			name:        "valid full manifest",
			data:        `{"name":"my-plugin","version":"1.2.3","main":"index.js"}`,
			wantName:    "my-plugin",
			wantVersion: "1.2.3",
			wantMain:    "index.js",
		},
		{
			name:        "missing main defaults",
			data:        `{"name":"my-plugin","version":"1.0.0"}`,
			wantName:    "my-plugin",
			wantVersion: "1.0.0",
			wantMain:    "main.js",
		},
		{
			name:        "empty main defaults",
			data:        `{"name":"my-plugin","version":"1.0.0","main":""}`,
			wantName:    "my-plugin",
			wantVersion: "1.0.0",
			wantMain:    "main.js",
		},
		{
			name:    "malformed json",
			data:    `{"name": "broken"`,
			wantErr: true,
		},
		{
			name:    "not an object",
			data:    `"just a string"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseManifest([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (manifest=%+v)", m)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", m.Name, tt.wantName)
			}
			if m.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", m.Version, tt.wantVersion)
			}
			if m.Main != tt.wantMain {
				t.Errorf("Main = %q, want %q", m.Main, tt.wantMain)
			}
		})
	}
}

func TestReadManifest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "kraken.json"), `{"name":"acme","version":"2.0.0"}`)

		m, err := ReadManifest(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.Name != "acme" || m.Version != "2.0.0" || m.Main != "main.js" {
			t.Errorf("unexpected manifest: %+v", m)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		_, err := ReadManifest(dir)
		if err == nil {
			t.Fatal("expected error for missing kraken.json, got nil")
		}
		if !os.IsNotExist(err) {
			t.Errorf("expected a not-exist error, got: %v", err)
		}
	})

	t.Run("malformed manifest", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "kraken.json"), `not json at all`)

		_, err := ReadManifest(dir)
		if err == nil {
			t.Fatal("expected error for malformed kraken.json, got nil")
		}
	})
}

// TestLoader_LoadAndEmit exercises the full path: load a plugin from disk,
// register a container:create handler via kraken.on, then fire the event
// through the Kraken event emitter and verify the JS handler mutated the
// payload's Data (a *runtime.ContainerOptions).
func TestLoader_LoadAndEmit(t *testing.T) {
	dir := t.TempDir()
	mainJS := `
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.Image = 'mutated-image';
	});
}
`
	pluginPath := filepath.Join(dir, "main.js")
	writeFile(t, pluginPath, mainJS)

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.Load(pluginPath); err != nil {
		t.Fatalf("Load: %v", err)
	}

	opts := &runtime.ContainerOptions{Image: "original-image"}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if opts.Image != "mutated-image" {
		t.Errorf("Image = %q, want %q", opts.Image, "mutated-image")
	}
}

// TestLoader_RelativeImport verifies that a plugin's main module can import
// a sibling module via a relative specifier, and that the resolved module
// participates correctly in the container:create handler.
func TestLoader_RelativeImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "helper.js"), `export function greet() { return 'hello-from-helper'; }`)
	writeFile(t, filepath.Join(dir, "main.js"), `
import { greet } from './helper.js';
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.Hostname = greet();
	});
}
`)

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.Load(filepath.Join(dir, "main.js")); err != nil {
		t.Fatalf("Load: %v", err)
	}

	opts := &runtime.ContainerOptions{}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if opts.Hostname != "hello-from-helper" {
		t.Errorf("Hostname = %q, want %q", opts.Hostname, "hello-from-helper")
	}
}

// TestLoader_NativeVarsModule verifies that the built-in "kraken/vars" native
// module surfaces values stored via internal/store.SetVar.
func TestLoader_NativeVarsModule(t *testing.T) {
	t.Setenv("KRAKEN_HOME", t.TempDir())

	if err := store.SetVar("greeting", "howdy"); err != nil {
		t.Fatalf("SetVar: %v", err)
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.js"), `
import vars from 'kraken/vars';
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.Name = vars.get('greeting');
	});
}
`)

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.Load(filepath.Join(dir, "main.js")); err != nil {
		t.Fatalf("Load: %v", err)
	}

	opts := &runtime.ContainerOptions{}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if opts.Name != "howdy" {
		t.Errorf("Name = %q, want %q", opts.Name, "howdy")
	}
}

// TestLoader_NativeEnvModule verifies that "kraken/env" surfaces host
// environment variables and returns null for unset ones.
func TestLoader_NativeEnvModule(t *testing.T) {
	t.Setenv("KRAKEN_TEST_ENV_VAR", "env-value")

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.js"), `
import env from 'kraken/env';
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.Name = env.get('KRAKEN_TEST_ENV_VAR');
		payload.data.Hostname = env.get('KRAKEN_TEST_VAR_NOT_SET') === null ? 'was-null' : 'not-null';
	});
}
`)

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.Load(filepath.Join(dir, "main.js")); err != nil {
		t.Fatalf("Load: %v", err)
	}

	opts := &runtime.ContainerOptions{}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if opts.Name != "env-value" {
		t.Errorf("Name = %q, want %q", opts.Name, "env-value")
	}
	if opts.Hostname != "was-null" {
		t.Errorf("Hostname = %q, want %q", opts.Hostname, "was-null")
	}
}

// TestLoader_NativeCtxModule verifies that "kraken/ctx" exposes the resolved
// kraken home directory.
func TestLoader_NativeCtxModule(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KRAKEN_HOME", home)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.js"), `
import ctx from 'kraken/ctx';
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.Name = ctx.home;
	});
}
`)

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.Load(filepath.Join(dir, "main.js")); err != nil {
		t.Fatalf("Load: %v", err)
	}

	opts := &runtime.ContainerOptions{}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if opts.Name != home {
		t.Errorf("Name = %q, want %q", opts.Name, home)
	}
}

// TestLoader_PluginDirExposed verifies that the "pluginDir" property passed
// to the plugin's default export matches the directory of the loaded file.
func TestLoader_PluginDirExposed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.js"), `
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.WorkDir = kraken.pluginDir;
	});
}
`)

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.Load(filepath.Join(dir, "main.js")); err != nil {
		t.Fatalf("Load: %v", err)
	}

	opts := &runtime.ContainerOptions{}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if opts.WorkDir != absDir {
		t.Errorf("WorkDir = %q, want %q", opts.WorkDir, absDir)
	}
}

func TestLoader_Load_Errors(t *testing.T) {
	t.Run("file does not exist", func(t *testing.T) {
		k := newTestKraken()
		l := NewLoader(k)
		err := l.Load(filepath.Join(t.TempDir(), "missing.js"))
		if err == nil {
			t.Fatal("expected error for missing plugin file")
		}
	})

	t.Run("invalid js syntax", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "main.js")
		writeFile(t, path, `export default function( {{{`)

		k := newTestKraken()
		l := NewLoader(k)
		if err := l.Load(path); err == nil {
			t.Fatal("expected parse error for invalid js")
		}
	})

	t.Run("missing default export", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "main.js")
		writeFile(t, path, `export const foo = 1;`)

		k := newTestKraken()
		l := NewLoader(k)
		err := l.Load(path)
		if err == nil {
			t.Fatal("expected error for missing default export")
		}
		if !strings.Contains(err.Error(), "missing default export") {
			t.Errorf("error = %v, want to contain %q", err, "missing default export")
		}
	})

	t.Run("default export not a function", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "main.js")
		writeFile(t, path, `export default 42;`)

		k := newTestKraken()
		l := NewLoader(k)
		err := l.Load(path)
		if err == nil {
			t.Fatal("expected error for non-function default export")
		}
		if !strings.Contains(err.Error(), "must be a function") {
			t.Errorf("error = %v, want to contain %q", err, "must be a function")
		}
	})

	t.Run("install function throws", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "main.js")
		writeFile(t, path, `export default function(kraken) { throw new Error('boom'); }`)

		k := newTestKraken()
		l := NewLoader(k)
		err := l.Load(path)
		if err == nil {
			t.Fatal("expected error when install function throws")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("error = %v, want to contain %q", err, "boom")
		}
	})

	t.Run("missing relative import", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "main.js")
		writeFile(t, path, `
import { greet } from './does-not-exist.js';
export default function(kraken) {}
`)

		k := newTestKraken()
		l := NewLoader(k)
		err := l.Load(path)
		if err == nil {
			t.Fatal("expected error for missing relative import")
		}
	})
}

func TestLoader_LoadDir(t *testing.T) {
	base := t.TempDir()

	pluginADir := filepath.Join(base, "plugin-a")
	writeFile(t, filepath.Join(pluginADir, "kraken.json"), `{"name":"plugin-a","version":"1.0.0"}`)
	writeFile(t, filepath.Join(pluginADir, "main.js"), `
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.Name = 'plugin-a-ran';
	});
}
`)

	// no kraken.json: should be silently skipped, not treated as an error.
	pluginBDir := filepath.Join(base, "plugin-b")
	writeFile(t, filepath.Join(pluginBDir, "main.js"), `export default function(kraken) {};`)

	// a plain file at the top level, alongside the plugin directories, must be ignored.
	writeFile(t, filepath.Join(base, "README.md"), "not a plugin")

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.LoadDir(base); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	opts := &runtime.ContainerOptions{}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if opts.Name != "plugin-a-ran" {
		t.Errorf("Name = %q, want %q (plugin-a should have loaded)", opts.Name, "plugin-a-ran")
	}
}

func TestLoader_LoadDir_MissingDir(t *testing.T) {
	k := newTestKraken()
	l := NewLoader(k)

	err := l.LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected nil error for missing plugins dir, got: %v", err)
	}
}

func TestLoader_LoadDir_MalformedManifestReturnsError(t *testing.T) {
	base := t.TempDir()

	badDir := filepath.Join(base, "bad-plugin")
	writeFile(t, filepath.Join(badDir, "kraken.json"), `not valid json`)
	writeFile(t, filepath.Join(badDir, "main.js"), `export default function(kraken) {};`)

	k := newTestKraken()
	l := NewLoader(k)

	if err := l.LoadDir(base); err == nil {
		t.Fatalf("LoadDir should surface an error for a malformed manifest, got nil")
	}
}

func TestLoader_RegisterModule(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.js"), `
import custom from 'kraken/custom-test-module';
export default function(kraken) {
	kraken.on('container:create', function(payload) {
		payload.data.Name = custom.value;
	});
}
`)

	k := newTestKraken()
	l := NewLoader(k)
	l.RegisterModule("kraken/custom-test-module", `export default { value: 'from-custom-module' };`)

	if err := l.Load(filepath.Join(dir, "main.js")); err != nil {
		t.Fatalf("Load: %v", err)
	}

	opts := &runtime.ContainerOptions{}
	payload := app.NewPayload("container:create")
	payload.Data = opts

	if err := k.Emit("container:create", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if opts.Name != "from-custom-module" {
		t.Errorf("Name = %q, want %q", opts.Name, "from-custom-module")
	}
}

// sanity check that Manifest round-trips through encoding/json the way the
// rest of the suite assumes.
func TestManifestJSONRoundtrip(t *testing.T) {
	m := Manifest{Name: "x", Version: "1", Main: "index.js"}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if *got != m {
		t.Errorf("got %+v, want %+v", *got, m)
	}
}
