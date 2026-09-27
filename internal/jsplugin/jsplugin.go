package jsplugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/grafana/sobek"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/store"
)

// Manifest represents the kraken.json file.
type Manifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Main    string `json:"main"`
}

func ReadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "kraken.json"))
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Main == "" {
		m.Main = "main.js"
	}
	return &m, nil
}

// Loader holds a shared JS runtime and module cache for all plugins.
type Loader struct {
	kraken *app.Kraken
	vm     *sobek.Runtime

	mu    sync.Mutex
	cache map[string]*sobek.SourceTextModuleRecord
	paths map[sobek.ModuleRecord]string // reverse map for relative import resolution

	// natives maps import specifier -> JS source for Go-backed modules.
	natives map[string]string
}

func NewLoader(kraken *app.Kraken) *Loader {
	l := &Loader{
		kraken:  kraken,
		vm:      sobek.New(),
		cache:   make(map[string]*sobek.SourceTextModuleRecord),
		paths:   make(map[sobek.ModuleRecord]string),
		natives: make(map[string]string),
	}
	l.setupNatives()
	return l
}

// RegisterModule adds a native JS module available via import.
func (l *Loader) RegisterModule(specifier, src string) {
	l.natives[specifier] = src
}

func (l *Loader) setupNatives() {
	l.vm.Set("__kraken_vars__", func() map[string]string {
		v, _ := store.Vars()
		return v
	})
	l.natives["kraken/vars"] = `
export function get(key) { return __kraken_vars__()[key]; }
export function all() { return __kraken_vars__(); }
export default { get, all };
`
	l.vm.Set("__kraken_env__", os.Getenv)
	l.natives["kraken/env"] = `
export function get(key) { return __kraken_env__(key) || null; }
export default { get };
`
	l.natives["kraken/ctx"] = `
export const home = __kraken_home__();
export default { home };
`
	l.vm.Set("__kraken_home__", store.KrakenHome)
}

// LoadDir loads each subdirectory under dir as a plugin using its kraken.json manifest.
func (l *Loader) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, e.Name())
		manifest, err := ReadManifest(pluginDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue // no kraken.json, skip
			}
			return fmt.Errorf("plugin %s: %w", pluginDir, err)
		}
		if err := l.Load(filepath.Join(pluginDir, manifest.Main)); err != nil {
			return err
		}
	}
	return nil
}

// Load reads a JS plugin entry point and registers its hooks on kraken.
func (l *Loader) Load(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("read plugin %s: %w", abs, err)
	}
	return l.execute(abs, string(data))
}

func (l *Loader) resolve(referencingScriptOrModule interface{}, specifier string) (sobek.ModuleRecord, error) {
	// Resolve relative imports against the referencing module's directory.
	if len(specifier) > 0 && (specifier[0] == '.' || specifier[0] == '/') {
		refPath := ""
		if ref, ok := referencingScriptOrModule.(sobek.ModuleRecord); ok {
			l.mu.Lock()
			refPath = l.paths[ref]
			l.mu.Unlock()
		}
		if refPath != "" {
			specifier = filepath.Join(filepath.Dir(refPath), specifier)
		}
	}

	l.mu.Lock()
	if m, ok := l.cache[specifier]; ok {
		l.mu.Unlock()
		return m, nil
	}
	l.mu.Unlock()

	var src string
	if native, ok := l.natives[specifier]; ok {
		src = native
	} else {
		data, err := os.ReadFile(specifier)
		if err != nil {
			return nil, fmt.Errorf("module not found: %s", specifier)
		}
		src = string(data)
	}

	m, err := sobek.ParseModule(specifier, src, l.resolve)
	if err != nil {
		return nil, err
	}

	l.mu.Lock()
	l.cache[specifier] = m
	l.paths[m] = specifier
	l.mu.Unlock()

	return m, nil
}

func (l *Loader) execute(name, code string) error {
	m, err := sobek.ParseModule(name, code, l.resolve)
	if err != nil {
		return fmt.Errorf("plugin %s: %w", name, err)
	}

	l.mu.Lock()
	l.cache[name] = m
	l.paths[m] = name
	l.mu.Unlock()

	if err := m.Link(); err != nil {
		return fmt.Errorf("plugin %s: link: %w", name, err)
	}

	p := m.Evaluate(l.vm)
	if p.State() == sobek.PromiseStateRejected {
		return fmt.Errorf("plugin %s: %v", name, p.Result())
	}

	b, _ := m.ResolveExport("default")
	if b == nil {
		return fmt.Errorf("plugin %s: missing default export", name)
	}

	fn, ok := sobek.AssertFunction(l.vm.GetModuleInstance(b.Module).GetBindingValue(b.BindingName))
	if !ok {
		return fmt.Errorf("plugin %s: default export must be a function", name)
	}

	krakenObj := l.buildKrakenObj(filepath.Dir(name))
	if _, err := fn(sobek.Undefined(), l.vm.ToValue(krakenObj)); err != nil {
		return fmt.Errorf("plugin %s: install: %w", name, err)
	}
	return nil
}

func (l *Loader) buildKrakenObj(pluginDir string) *sobek.Object {
	obj := l.vm.NewObject()
	obj.Set("on", func(event string, fn sobek.Callable) {
		l.kraken.Before(event, makeHandler(l.vm, fn))
	})
	obj.Set("pluginDir", pluginDir)
	return obj
}

func makeHandler(vm *sobek.Runtime, fn sobek.Callable) func(*app.Payload) error {
	return func(p *app.Payload) error {
		obj := vm.NewObject()
		obj.Set("data", vm.ToValue(p.Data))
		_, err := fn(sobek.Undefined(), vm.ToValue(obj))
		return err
	}
}
