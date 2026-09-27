package util

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kraken-hub/kraken/internal/store"
)

func TestParseRef(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantApp     string
		wantService string
	}{
		{"empty args", nil, "", ""},
		{"bare app", []string{"myapp"}, "myapp", ""},
		{"service dot app", []string{"service.myapp"}, "myapp", "service"},
		{"service dot only", []string{"service."}, "", "service"},
		{"dot leading empty app", []string{"."}, "", ""},
		{"multiple dots uses first", []string{"a.b.c"}, "b.c", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotApp, gotService := ParseRef(tt.args)
			if gotApp != tt.wantApp || gotService != tt.wantService {
				t.Errorf("ParseRef(%v) = (%q,%q), want (%q,%q)", tt.args, gotApp, gotService, tt.wantApp, tt.wantService)
			}
		})
	}
}

func TestParseServiceRef(t *testing.T) {
	t.Run("no args errors", func(t *testing.T) {
		if _, _, err := ParseServiceRef(nil); err == nil {
			t.Fatal("expected error for empty args")
		}
	})
	t.Run("no service errors", func(t *testing.T) {
		if _, _, err := ParseServiceRef([]string{"myapp"}); err == nil {
			t.Fatal("expected error when no service given")
		}
	})
	t.Run("valid ref resolves app and service", func(t *testing.T) {
		appName, serviceName, err := ParseServiceRef([]string{"web.myapp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if appName != "myapp" || serviceName != "web" {
			t.Errorf("got (%q,%q), want (myapp,web)", appName, serviceName)
		}
	})
}

func TestResolveAppName(t *testing.T) {
	t.Run("arg takes priority", func(t *testing.T) {
		t.Setenv("KRAKEN_APP", "envapp")
		got, err := ResolveAppName([]string{"argapp"})
		if err != nil || got != "argapp" {
			t.Errorf("got (%q,%v), want argapp", got, err)
		}
	})
	t.Run("empty arg falls back to env", func(t *testing.T) {
		t.Setenv("KRAKEN_APP", "envapp")
		got, err := ResolveAppName([]string{""})
		if err != nil || got != "envapp" {
			t.Errorf("got (%q,%v), want envapp", got, err)
		}
	})
	t.Run("no args uses env", func(t *testing.T) {
		t.Setenv("KRAKEN_APP", "envapp")
		got, err := ResolveAppName(nil)
		if err != nil || got != "envapp" {
			t.Errorf("got (%q,%v), want envapp", got, err)
		}
	})
	t.Run("falls back to cwd dirname", func(t *testing.T) {
		t.Setenv("KRAKEN_APP", "")
		dir := t.TempDir()
		sub := filepath.Join(dir, "my-project")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(sub)
		got, err := ResolveAppName(nil)
		if err != nil || got != "my-project" {
			t.Errorf("got (%q,%v), want my-project", got, err)
		}
	})
}

func TestExpandBraceRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want []string
	}{
		{"plain ref unchanged", "myapp", []string{"myapp"}},
		{"service dot app unchanged", "web.myapp", []string{"web.myapp"}},
		{"brace expands to multiple services", "{web,db}.myapp", []string{"web.myapp", "db.myapp"}},
		{"brace with spaces trimmed", "{ web , db }.myapp", []string{"web.myapp", "db.myapp"}},
		{"unmatched brace returned unchanged", "{web,db", []string{"{web,db"}},
		{"empty parts skipped", "{web,,db}.myapp", []string{"web.myapp", "db.myapp"}},
		{"brace with no suffix", "{web,db}", []string{"web", "db"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandBraceRef(tt.ref)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandBraceRef(%q) = %#v, want %#v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestResolveUniqueApp(t *testing.T) {
	t.Setenv("KRAKEN_APP", "")
	t.Run("no args resolves default", func(t *testing.T) {
		t.Setenv("KRAKEN_APP", "defaultapp")
		got, err := ResolveUniqueApp(nil)
		if err != nil || got != "defaultapp" {
			t.Errorf("got (%q,%v), want defaultapp", got, err)
		}
	})
	t.Run("single app across refs", func(t *testing.T) {
		got, err := ResolveUniqueApp([]string{"web.myapp", "db.myapp"})
		if err != nil || got != "myapp" {
			t.Errorf("got (%q,%v), want myapp", got, err)
		}
	})
	t.Run("brace expansion resolves same app", func(t *testing.T) {
		got, err := ResolveUniqueApp([]string{"{web,db}.myapp"})
		if err != nil || got != "myapp" {
			t.Errorf("got (%q,%v), want myapp", got, err)
		}
	})
	t.Run("multiple apps errors", func(t *testing.T) {
		_, err := ResolveUniqueApp([]string{"web.app1", "db.app2"})
		if err == nil {
			t.Fatal("expected error for refs spanning multiple apps")
		}
	})
}

func TestForEachRef(t *testing.T) {
	t.Setenv("KRAKEN_APP", "")
	t.Run("empty args calls fn once with default", func(t *testing.T) {
		t.Setenv("KRAKEN_APP", "defaultapp")
		var calls [][2]string
		err := ForEachRef(nil, func(appName, service string) error {
			calls = append(calls, [2]string{appName, service})
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := [][2]string{{"defaultapp", ""}}
		if !reflect.DeepEqual(calls, want) {
			t.Errorf("got %#v, want %#v", calls, want)
		}
	})
	t.Run("expands brace refs", func(t *testing.T) {
		var calls [][2]string
		err := ForEachRef([]string{"{web,db}.myapp"}, func(appName, service string) error {
			calls = append(calls, [2]string{appName, service})
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := [][2]string{{"myapp", "web"}, {"myapp", "db"}}
		if !reflect.DeepEqual(calls, want) {
			t.Errorf("got %#v, want %#v", calls, want)
		}
	})
	t.Run("propagates fn error", func(t *testing.T) {
		wantErr := os.ErrInvalid
		err := ForEachRef([]string{"web.myapp"}, func(appName, service string) error {
			return wantErr
		})
		if err != wantErr {
			t.Errorf("got %v, want %v", err, wantErr)
		}
	})
}

func TestResolveRef(t *testing.T) {
	t.Setenv("KRAKEN_APP", "")
	t.Run("app given explicitly", func(t *testing.T) {
		appName, serviceName, err := ResolveRef([]string{"web.myapp"})
		if err != nil || appName != "myapp" || serviceName != "web" {
			t.Errorf("got (%q,%q,%v), want (myapp,web,nil)", appName, serviceName, err)
		}
	})
	t.Run("empty app resolved via env", func(t *testing.T) {
		t.Setenv("KRAKEN_APP", "envapp")
		appName, serviceName, err := ResolveRef([]string{"service."})
		if err != nil || appName != "envapp" || serviceName != "service" {
			t.Errorf("got (%q,%q,%v), want (envapp,service,nil)", appName, serviceName, err)
		}
	})
}

func TestEnvFunc(t *testing.T) {
	t.Setenv("KRAKEN_HOME", t.TempDir())
	appPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(appPath, ".env"), []byte("FOO=base\nSHARED=fromenv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appPath, ".env.local"), []byte("SHARED=fromlocal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOSTONLY", "fromhost")
	if err := store.SetVar("STOREVAR", "fromstore"); err != nil {
		t.Fatal(err)
	}

	appEntry := &store.App{
		Key: "myapp",
		Value: store.AppValue{
			Path: appPath,
			Env:  "dev",
		},
	}
	envFunc := EnvFunc(appEntry)

	tests := []struct {
		name string
		want string
	}{
		{"APP_NAME", "myapp"},
		{"APP_PATH", appPath},
		{"APP_ENV", "dev"},
		{"FOO", "base"},
		{"SHARED", "fromlocal"},
		{"STOREVAR", "fromstore"},
		{"HOSTONLY", "fromhost"},
		{"NONEXISTENT_VAR", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := envFunc(tt.name); got != tt.want {
				t.Errorf("envFunc(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestLoadDotEnv(t *testing.T) {
	t.Run("missing files return empty map", func(t *testing.T) {
		got := loadDotEnv(t.TempDir())
		if len(got) != 0 {
			t.Errorf("expected empty map, got %#v", got)
		}
	})
	t.Run(".env.local overrides .env", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\nB=2\n"), 0o644)
		os.WriteFile(filepath.Join(dir, ".env.local"), []byte("B=3\n"), 0o644)
		got := loadDotEnv(dir)
		want := map[string]string{"A": "1", "B": "3"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
}

func TestInitApp(t *testing.T) {
	t.Setenv("KRAKEN_HOME", t.TempDir())
	t.Setenv("KRAKEN_ENV", "")

	t.Run("invalid app name rejected", func(t *testing.T) {
		if _, err := InitApp("Invalid Name", "", "", ""); err == nil {
			t.Fatal("expected error for invalid app name")
		}
	})

	t.Run("missing config file errors", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := InitApp("myapp", dir, "", ""); err == nil {
			t.Fatal("expected error for missing config file")
		}
	})

	t.Run("valid app is registered with defaults", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "kraken.yaml")
		if err := os.WriteFile(cfgPath, []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		appEntry, err := InitApp("myapp", dir, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if appEntry.Key != "myapp" {
			t.Errorf("Key = %q, want myapp", appEntry.Key)
		}
		if appEntry.Value.Path != dir {
			t.Errorf("Path = %q, want %q", appEntry.Value.Path, dir)
		}
		if appEntry.Value.Env != "dev" {
			t.Errorf("Env = %q, want dev", appEntry.Value.Env)
		}
		if appEntry.Value.Config.File != cfgPath {
			t.Errorf("Config.File = %q, want %q", appEntry.Value.Config.File, cfgPath)
		}
		if appEntry.Value.Config.Data == "" {
			t.Error("expected non-empty processed config data")
		}

		loaded, err := store.LoadApp("myapp")
		if err != nil {
			t.Fatalf("app was not saved: %v", err)
		}
		if loaded.Value.Path != dir {
			t.Errorf("loaded Path = %q, want %q", loaded.Value.Path, dir)
		}
	})

	t.Run("explicit env respected", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "kraken.yaml")
		os.WriteFile(cfgPath, []byte("services:\n  web:\n    image: nginx\n"), 0o644)
		appEntry, err := InitApp("myapp2", dir, "", "prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if appEntry.Value.Env != "prod" {
			t.Errorf("Env = %q, want prod", appEntry.Value.Env)
		}
	})

	t.Run("relative config file resolved under app path", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "custom.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644)
		appEntry, err := InitApp("myapp3", dir, "custom.yaml", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if appEntry.Value.Config.File != filepath.Join(dir, "custom.yaml") {
			t.Errorf("Config.File = %q", appEntry.Value.Config.File)
		}
	})
}
