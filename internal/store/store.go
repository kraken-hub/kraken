// Package store provides JSON-file-backed storage for application state,
// variables, and plugins.
//
// Default base directory: platform user config dir / kraken (override with KRAKEN_HOME).
// Individual paths can be overridden with KRAKEN_STORAGE, KRAKEN_PLUGINS.
//
// App files are named {shortHash}@{appName}, e.g. 55a0f9a@arcadia
package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppConfig holds the config file path and processed YAML snapshot.
type AppConfig struct {
	File string `json:"file"`
	Data string `json:"data"`
}

// AppValue holds the runtime metadata for an application.
type AppValue struct {
	Path   string    `json:"path"`
	Env    string    `json:"env"` // comma-separated active envs, e.g. "local,dev,ssl"
	Config AppConfig `json:"config"`
}

// App is the top-level record stored on disk.
type App struct {
	Key   string   `json:"key"`
	Value AppValue `json:"value"`
}

// KrakenHome returns the base kraken directory.
// Override with KRAKEN_HOME; defaults to a platform-appropriate user directory.
func KrakenHome() string {
	return krakenHome()
}

// krakenHome is the unexported implementation.
func krakenHome() string {
	if v := os.Getenv("KRAKEN_HOME"); v != "" {
		return v
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "kraken")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".kraken")
	}
	return "/var/lib/kraken"
}

// storageRoot returns the base storage directory.
func storageRoot() string {
	if v := os.Getenv("KRAKEN_STORAGE"); v != "" {
		return v
	}
	return filepath.Join(krakenHome(), "storage", "fs")
}

// PluginsDir returns the directory where JS plugin files are stored.
func PluginsDir() string {
	if v := os.Getenv("KRAKEN_PLUGINS"); v != "" {
		return v
	}
	return filepath.Join(krakenHome(), "plugins")
}

func appsDir() (string, error) {
	dir := filepath.Join(storageRoot(), "apps")
	return dir, os.MkdirAll(dir, 0o755)
}

// appFileName returns the on-disk filename for an app: {shortHash}@{name}
func appFileName(name, path string) string {
	sum := sha256.Sum256([]byte(path))
	short := fmt.Sprintf("%x", sum[:])[:7]
	return short + "@" + name
}

// findAppFile searches appsDir for a file matching *@name.
func findAppFile(name string) (string, error) {
	dir, err := appsDir()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	suffix := "@" + name
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", os.ErrNotExist
}

// ---- App operations ----

// SaveApp persists an App entry.
func SaveApp(app *App) error {
	dir, err := appsDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(app, "", "    ")
	if err != nil {
		return err
	}
	filename := appFileName(app.Key, app.Value.Path)
	return os.WriteFile(filepath.Join(dir, filename), data, 0o644)
}

// LoadApp loads an App by name. Returns a descriptive error if not found.
func LoadApp(name string) (*App, error) {
	path, err := findAppFile(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("app %q not found", name)
		}
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var app App
	if err := json.Unmarshal(data, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// ListApps returns all registered apps.
func ListApps() ([]*App, error) {
	dir, err := appsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var apps []*App
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(e.Name(), "@") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var app App
		if err := json.Unmarshal(data, &app); err != nil {
			continue
		}
		apps = append(apps, &app)
	}
	return apps, nil
}

// DeleteApp removes an app entry.
func DeleteApp(name string) error {
	path, err := findAppFile(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.Remove(path)
}

// AppExists returns true when an app with the given name is registered.
func AppExists(name string) (bool, error) {
	_, err := findAppFile(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ---- Vars operations ----

// Var is a single stored variable.
type Var struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func varsDir() (string, error) {
	dir := filepath.Join(storageRoot(), "vars")
	return dir, os.MkdirAll(dir, 0o755)
}

func varFileName(key string) string {
	sum := sha256.Sum256([]byte(key))
	short := fmt.Sprintf("%x", sum[:])[:7]
	return short + "@" + key
}

// Vars loads all stored variables as a map.
func Vars() (map[string]string, error) {
	dir, err := varsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	m := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(e.Name(), "@") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var v Var
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		m[v.Key] = v.Value
	}
	return m, nil
}

// GetVar returns a single variable value.
func GetVar(key string) (string, error) {
	dir, err := varsDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, varFileName(key)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	var v Var
	if err := json.Unmarshal(data, &v); err != nil {
		return "", err
	}
	return v.Value, nil
}

// SetVar stores a variable.
func SetVar(key, value string) error {
	dir, err := varsDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(Var{Key: key, Value: value}, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, varFileName(key)), data, 0o644)
}

// UnsetVar removes a variable.
func UnsetVar(key string) error {
	dir, err := varsDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, varFileName(key)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
