package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withStorage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KRAKEN_STORAGE", dir)
	return dir
}

func TestKrakenHome(t *testing.T) {
	t.Run("override via KRAKEN_HOME", func(t *testing.T) {
		t.Setenv("KRAKEN_HOME", "/custom/kraken/home")
		if got := KrakenHome(); got != "/custom/kraken/home" {
			t.Errorf("KrakenHome() = %q, want %q", got, "/custom/kraken/home")
		}
	})

	t.Run("default falls back to user config dir", func(t *testing.T) {
		t.Setenv("KRAKEN_HOME", "")
		wantBase, err := os.UserConfigDir()
		if err != nil {
			t.Skipf("os.UserConfigDir() unavailable: %v", err)
		}
		want := filepath.Join(wantBase, "kraken")
		if got := KrakenHome(); got != want {
			t.Errorf("KrakenHome() = %q, want %q", got, want)
		}
	})
}

func TestPluginsDir(t *testing.T) {
	t.Run("override via KRAKEN_PLUGINS", func(t *testing.T) {
		t.Setenv("KRAKEN_PLUGINS", "/custom/plugins")
		if got := PluginsDir(); got != "/custom/plugins" {
			t.Errorf("PluginsDir() = %q, want %q", got, "/custom/plugins")
		}
	})

	t.Run("default derived from KRAKEN_HOME", func(t *testing.T) {
		t.Setenv("KRAKEN_PLUGINS", "")
		t.Setenv("KRAKEN_HOME", "/custom/kraken/home")
		want := filepath.Join("/custom/kraken/home", "plugins")
		if got := PluginsDir(); got != want {
			t.Errorf("PluginsDir() = %q, want %q", got, want)
		}
	})
}

func TestSaveLoadApp(t *testing.T) {
	withStorage(t)

	app := &App{
		Key: "myapp",
		Value: AppValue{
			Path: "/apps/myapp",
			Env:  "dev",
			Config: AppConfig{
				File: "kraken.yaml",
				Data: "services: {}",
			},
		},
	}

	if err := SaveApp(app); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	got, err := LoadApp("myapp")
	if err != nil {
		t.Fatalf("LoadApp() error = %v", err)
	}
	if got.Key != app.Key || got.Value.Path != app.Value.Path || got.Value.Env != app.Value.Env {
		t.Errorf("LoadApp() = %+v, want %+v", got, app)
	}
	if got.Value.Config.File != app.Value.Config.File || got.Value.Config.Data != app.Value.Config.Data {
		t.Errorf("LoadApp() config = %+v, want %+v", got.Value.Config, app.Value.Config)
	}
}

func TestSaveApp_Overwrite(t *testing.T) {
	withStorage(t)

	app := &App{Key: "myapp", Value: AppValue{Path: "/apps/myapp", Env: "dev"}}
	if err := SaveApp(app); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	app.Value.Env = "prod"
	if err := SaveApp(app); err != nil {
		t.Fatalf("SaveApp() (overwrite) error = %v", err)
	}

	got, err := LoadApp("myapp")
	if err != nil {
		t.Fatalf("LoadApp() error = %v", err)
	}
	if got.Value.Env != "prod" {
		t.Errorf("LoadApp() env = %q, want %q", got.Value.Env, "prod")
	}

	apps, err := ListApps()
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(apps) != 1 {
		t.Errorf("ListApps() len = %d, want 1 (overwrite of same path should not duplicate)", len(apps))
	}
}

func TestLoadApp_NotFound(t *testing.T) {
	withStorage(t)

	_, err := LoadApp("ghost")
	if err == nil {
		t.Fatal("LoadApp() error = nil, want not-found error")
	}
	want := `app "ghost" not found`
	if err.Error() != want {
		t.Errorf("LoadApp() error = %q, want %q", err.Error(), want)
	}
}

func TestListApps_Multiple(t *testing.T) {
	withStorage(t)

	names := []string{"app-one", "app-two", "app-three"}
	for _, n := range names {
		app := &App{Key: n, Value: AppValue{Path: "/apps/" + n}}
		if err := SaveApp(app); err != nil {
			t.Fatalf("SaveApp(%q) error = %v", n, err)
		}
	}

	apps, err := ListApps()
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(apps) != len(names) {
		t.Fatalf("ListApps() len = %d, want %d", len(apps), len(names))
	}

	got := map[string]bool{}
	for _, a := range apps {
		got[a.Key] = true
	}
	for _, n := range names {
		if !got[n] {
			t.Errorf("ListApps() missing app %q", n)
		}
	}
}

func TestListApps_Empty(t *testing.T) {
	withStorage(t)

	apps, err := ListApps()
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("ListApps() len = %d, want 0", len(apps))
	}
}

func TestListApps_SkipsMalformedFiles(t *testing.T) {
	dir := withStorage(t)

	app := &App{Key: "good", Value: AppValue{Path: "/apps/good"}}
	if err := SaveApp(app); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	appsPath := filepath.Join(dir, "apps")
	if err := os.WriteFile(filepath.Join(appsPath, "badhash@broken"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appsPath, "no-at-sign"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write no-@-file: %v", err)
	}

	apps, err := ListApps()
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(apps) != 1 || apps[0].Key != "good" {
		t.Errorf("ListApps() = %+v, want only the well-formed app", apps)
	}
}

func TestDeleteApp(t *testing.T) {
	withStorage(t)

	app := &App{Key: "myapp", Value: AppValue{Path: "/apps/myapp"}}
	if err := SaveApp(app); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	if err := DeleteApp("myapp"); err != nil {
		t.Fatalf("DeleteApp() error = %v", err)
	}

	if _, err := LoadApp("myapp"); err == nil {
		t.Error("LoadApp() after DeleteApp() error = nil, want not-found error")
	}
}

func TestDeleteApp_MissingIsNoop(t *testing.T) {
	withStorage(t)

	if err := DeleteApp("ghost"); err != nil {
		t.Errorf("DeleteApp() on missing app error = %v, want nil", err)
	}
}

func TestAppExists(t *testing.T) {
	withStorage(t)

	exists, err := AppExists("myapp")
	if err != nil {
		t.Fatalf("AppExists() error = %v", err)
	}
	if exists {
		t.Error("AppExists() = true, want false before save")
	}

	app := &App{Key: "myapp", Value: AppValue{Path: "/apps/myapp"}}
	if err := SaveApp(app); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	exists, err = AppExists("myapp")
	if err != nil {
		t.Fatalf("AppExists() error = %v", err)
	}
	if !exists {
		t.Error("AppExists() = false, want true after save")
	}
}

func TestAppFileName_Format(t *testing.T) {
	name := appFileName("myapp", "/apps/myapp")
	parts := strings.SplitN(name, "@", 2)
	if len(parts) != 2 {
		t.Fatalf("appFileName() = %q, want <hash>@<name> format", name)
	}
	if len(parts[0]) != 7 {
		t.Errorf("appFileName() hash part = %q, want length 7", parts[0])
	}
	if parts[1] != "myapp" {
		t.Errorf("appFileName() name part = %q, want %q", parts[1], "myapp")
	}
}

func TestGetVar_Missing(t *testing.T) {
	withStorage(t)

	got, err := GetVar("ghost")
	if err != nil {
		t.Fatalf("GetVar() error = %v", err)
	}
	if got != "" {
		t.Errorf("GetVar() = %q, want empty string", got)
	}
}

func TestSetGetVar(t *testing.T) {
	withStorage(t)

	if err := SetVar("color", "blue"); err != nil {
		t.Fatalf("SetVar() error = %v", err)
	}

	got, err := GetVar("color")
	if err != nil {
		t.Fatalf("GetVar() error = %v", err)
	}
	if got != "blue" {
		t.Errorf("GetVar() = %q, want %q", got, "blue")
	}
}

func TestSetVar_Overwrite(t *testing.T) {
	withStorage(t)

	if err := SetVar("color", "blue"); err != nil {
		t.Fatalf("SetVar() error = %v", err)
	}
	if err := SetVar("color", "red"); err != nil {
		t.Fatalf("SetVar() (overwrite) error = %v", err)
	}

	got, err := GetVar("color")
	if err != nil {
		t.Fatalf("GetVar() error = %v", err)
	}
	if got != "red" {
		t.Errorf("GetVar() = %q, want %q", got, "red")
	}

	vars, err := Vars()
	if err != nil {
		t.Fatalf("Vars() error = %v", err)
	}
	if len(vars) != 1 {
		t.Errorf("Vars() len = %d, want 1 (overwrite should not duplicate)", len(vars))
	}
}

func TestUnsetVar(t *testing.T) {
	withStorage(t)

	if err := SetVar("color", "blue"); err != nil {
		t.Fatalf("SetVar() error = %v", err)
	}
	if err := UnsetVar("color"); err != nil {
		t.Fatalf("UnsetVar() error = %v", err)
	}

	got, err := GetVar("color")
	if err != nil {
		t.Fatalf("GetVar() error = %v", err)
	}
	if got != "" {
		t.Errorf("GetVar() after UnsetVar() = %q, want empty string", got)
	}
}

func TestUnsetVar_MissingIsNoop(t *testing.T) {
	withStorage(t)

	if err := UnsetVar("ghost"); err != nil {
		t.Errorf("UnsetVar() on missing key error = %v, want nil", err)
	}
}

func TestVars_Multiple(t *testing.T) {
	withStorage(t)

	want := map[string]string{
		"color": "blue",
		"size":  "large",
		"count": "3",
	}
	for k, v := range want {
		if err := SetVar(k, v); err != nil {
			t.Fatalf("SetVar(%q) error = %v", k, err)
		}
	}

	got, err := Vars()
	if err != nil {
		t.Fatalf("Vars() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("Vars() len = %d, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Vars()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestVars_Empty(t *testing.T) {
	withStorage(t)

	got, err := Vars()
	if err != nil {
		t.Fatalf("Vars() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Vars() len = %d, want 0", len(got))
	}
}

func TestVars_SkipsMalformedFiles(t *testing.T) {
	dir := withStorage(t)

	if err := SetVar("good", "value"); err != nil {
		t.Fatalf("SetVar() error = %v", err)
	}

	varsPath := filepath.Join(dir, "vars")
	if err := os.WriteFile(filepath.Join(varsPath, "badhash@broken"), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}

	got, err := Vars()
	if err != nil {
		t.Fatalf("Vars() error = %v", err)
	}
	if len(got) != 1 || got["good"] != "value" {
		t.Errorf("Vars() = %+v, want only the well-formed var", got)
	}
}

func TestVarFileName_Format(t *testing.T) {
	name := varFileName("color")
	parts := strings.SplitN(name, "@", 2)
	if len(parts) != 2 {
		t.Fatalf("varFileName() = %q, want <hash>@<key> format", name)
	}
	if len(parts[0]) != 7 {
		t.Errorf("varFileName() hash part = %q, want length 7", parts[0])
	}
	if parts[1] != "color" {
		t.Errorf("varFileName() key part = %q, want %q", parts[1], "color")
	}
}
