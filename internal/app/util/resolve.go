package util

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kraken-hub/kraken/internal/config"
	"github.com/kraken-hub/kraken/internal/store"
	"github.com/kraken-hub/kraken/pkg/dotenv"
)

var validAppName = regexp.MustCompile(`^[a-z0-9-]+$`)

// ParseServiceRef parses args as a "[service.]app" ref, validates that a service name is
// present, and resolves the app name. All returned errors indicate invalid user input.
func ParseServiceRef(args []string) (appName, serviceName string, err error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("service name required: use service.app")
	}
	appName, serviceName = ParseRef(args[:1])
	if serviceName == "" {
		return "", "", fmt.Errorf("service name required: use service.app")
	}
	appName, err = ResolveAppName([]string{appName})
	return
}

// EnvFunc returns an env lookup function for YAML template rendering.
// Priority: APP_NAME/APP_PATH/APP_ENV → .env.local → .env → store vars → os.Getenv
func EnvFunc(appEntry *store.App) func(string) string {
	dotenvVars := loadDotEnv(appEntry.Value.Path)
	storeVars, _ := store.Vars()
	return func(name string) string {
		switch name {
		case "APP_NAME":
			return appEntry.Key
		case "APP_PATH":
			return appEntry.Value.Path
		case "APP_ENV":
			return appEntry.Value.Env
		}
		if v, ok := dotenvVars[name]; ok {
			return v
		}
		if v, ok := storeVars[name]; ok {
			return v
		}
		return os.Getenv(name)
	}
}

// loadDotEnv loads .env then .env.local from appPath; later files overwrite earlier.
func loadDotEnv(appPath string) map[string]string {
	result := map[string]string{}
	for _, name := range []string{".env", ".env.local"} {
		vars, err := dotenv.Load(filepath.Join(appPath, name))
		if err != nil {
			continue
		}
		for k, v := range vars {
			result[k] = v
		}
	}
	return result
}

// ParseRef splits "[service.]app" into (appName, serviceName).
// "myapp" → ("myapp",""), "service.myapp" → ("myapp","service"),
// "service." → ("","service") — app resolved later from KRAKEN_APP or current dir name.
func ParseRef(args []string) (string, string) {
	if len(args) == 0 {
		return "", ""
	}
	ref := args[0]
	if idx := strings.Index(ref, "."); idx >= 0 {
		return ref[idx+1:], ref[:idx]
	}
	return ref, ""
}

// ExpandBraceRef expands a brace group in the service position of a ref.
// "{service1,service2}.myapp" → ["service1.myapp", "service2.myapp"]
// Plain refs are returned unchanged as a single-element slice.
func ExpandBraceRef(ref string) []string {
	if !strings.HasPrefix(ref, "{") {
		return []string{ref}
	}
	close := strings.Index(ref, "}")
	if close < 0 {
		return []string{ref}
	}
	inner := ref[1:close]
	suffix := ref[close+1:]
	parts := strings.Split(inner, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p+suffix)
		}
	}
	return result
}

// ResolveUniqueApp expands brace refs from args, resolves all app names, and returns
// the single app name when all refs belong to the same app.
// Returns an error if refs span multiple apps.
func ResolveUniqueApp(args []string) (string, error) {
	var refs []string
	for _, arg := range args {
		refs = append(refs, ExpandBraceRef(arg)...)
	}
	if len(refs) == 0 {
		return ResolveAppName(nil)
	}
	var unique string
	for _, ref := range refs {
		appRaw, _ := ParseRef([]string{ref})
		appName, err := ResolveAppName([]string{appRaw})
		if err != nil {
			return "", err
		}
		if unique == "" {
			unique = appName
		} else if unique != appName {
			return "", fmt.Errorf("refs span multiple apps (%q and %q)", unique, appName)
		}
	}
	return unique, nil
}

// ForEachRef expands brace refs, resolves app names, and calls fn for each (appName, serviceName) pair.
// If args is empty, fn is called once with the default app and no service filter.
func ForEachRef(args []string, fn func(appName, service string) error) error {
	var refs []string
	for _, arg := range args {
		refs = append(refs, ExpandBraceRef(arg)...)
	}
	if len(refs) == 0 {
		refs = []string{""}
	}
	for _, ref := range refs {
		appName, serviceFilter := ParseRef([]string{ref})
		appName, err := ResolveAppName([]string{appName})
		if err != nil {
			return err
		}
		if err := fn(appName, serviceFilter); err != nil {
			return err
		}
	}
	return nil
}

// ResolveRef parses "[service.]app" and resolves an empty app name via ResolveAppName.
func ResolveRef(args []string) (appName, serviceName string, err error) {
	appName, serviceName = ParseRef(args)
	if appName == "" {
		appName, err = ResolveAppName(nil)
	}
	return
}

// ResolveAppName picks the app name from args, KRAKEN_APP env var, or current dir name.
func ResolveAppName(args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if v := os.Getenv("KRAKEN_APP"); v != "" {
		return v, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("app name required (pass as argument or set KRAKEN_APP)")
	}
	return filepath.Base(cwd), nil
}

// InitApp registers an application and processes its config snapshot.
// appPath defaults to cwd; configFile defaults to kraken.yaml inside appPath;
// env defaults to KRAKEN_ENV env var or "dev".
func InitApp(appName, appPath, configFile, env string) (*store.App, error) {
	if !validAppName.MatchString(appName) {
		return nil, fmt.Errorf("app name %q includes invalid characters, only [a-z0-9-] are allowed", appName)
	}

	if appPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		appPath = cwd
	} else if !filepath.IsAbs(appPath) {
		abs, err := filepath.Abs(appPath)
		if err != nil {
			return nil, fmt.Errorf("resolve path: %w", err)
		}
		appPath = abs
	}

	if configFile == "" {
		configFile = filepath.Join(appPath, "kraken.yaml")
	} else if !filepath.IsAbs(configFile) {
		configFile = filepath.Join(appPath, configFile)
	}

	if _, err := os.Stat(configFile); err != nil {
		return nil, fmt.Errorf("config file %q not found", configFile)
	}

	if env == "" {
		env = os.Getenv("KRAKEN_ENV")
	}
	if env == "" {
		env = "dev"
	}

	appEntry := &store.App{
		Key: appName,
		Value: store.AppValue{
			Path: appPath,
			Env:  env,
			Config: store.AppConfig{
				File: configFile,
			},
		},
	}

	configData, err := config.LoadYamlFile(configFile, EnvFunc(appEntry))
	if err != nil {
		return nil, fmt.Errorf("process config: %w", err)
	}
	appEntry.Value.Config.Data = configData

	if err := store.SaveApp(appEntry); err != nil {
		return nil, fmt.Errorf("save app: %w", err)
	}

	return appEntry, nil
}
