package docker

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types/registry"
)

func writeDockerConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.ConfigFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(config.EnvOverrideConfigDir, dir)
	return dir
}

func TestResolveRegistryAuth_NoConfigFile(t *testing.T) {
	t.Setenv(config.EnvOverrideConfigDir, t.TempDir())

	_, _, found, err := ResolveRegistryAuth("example.com/foo:bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected no credentials to be found")
	}
}

func TestResolveRegistryAuth_StaticAuth(t *testing.T) {
	auth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	writeDockerConfig(t, `{"auths":{"example.com":{"auth":"`+auth+`"}}}`)

	hostname, ac, found, err := ResolveRegistryAuth("example.com/foo:bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected credentials to be found")
	}
	if hostname != "example.com" {
		t.Fatalf("hostname = %q, want %q", hostname, "example.com")
	}
	if ac.Username != "user" || ac.Password != "pass" {
		t.Fatalf("unexpected auth config: %+v", ac)
	}
}

func TestResolveRegistryAuth_StaticAuthMiss(t *testing.T) {
	auth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	writeDockerConfig(t, `{"auths":{"other.example.com":{"auth":"`+auth+`"}}}`)

	_, _, found, err := ResolveRegistryAuth("example.com/foo:bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected no credentials to be found for unrelated registry")
	}
}

func TestResolveRegistryAuth_DockerHubNormalization(t *testing.T) {
	auth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	writeDockerConfig(t, `{"auths":{"`+dockerHubServerAddress+`":{"auth":"`+auth+`"}}}`)

	hostname, ac, found, err := ResolveRegistryAuth("redis:7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected credentials to be found for docker.io image via legacy index server key")
	}
	if hostname != dockerHubServerAddress {
		t.Fatalf("hostname = %q, want %q", hostname, dockerHubServerAddress)
	}
	if ac.Username != "user" {
		t.Fatalf("unexpected auth config: %+v", ac)
	}
}

func TestResolveRegistryAuth_InvalidImageRef(t *testing.T) {
	_, _, found, err := ResolveRegistryAuth("INVALID!!REF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected no credentials for an unparseable image reference")
	}
}

func TestResolveRegistryAuth_MalformedConfig(t *testing.T) {
	writeDockerConfig(t, `not valid json`)

	_, _, found, err := ResolveRegistryAuth("example.com/foo:bar")
	if err == nil {
		t.Fatal("expected an error for a malformed config file")
	}
	if found {
		t.Fatal("expected found=false alongside the error")
	}
}

// installFakeCredentialHelper writes a fake docker-credential-<suffix>
// script implementing the get command of the credential-helper protocol
// (https://github.com/docker/docker-credential-helpers): it reads the
// server URL from stdin and either prints a JSON credentials payload or
// exits non-zero with an error message on stdout.
func installFakeCredentialHelper(t *testing.T, suffix, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake credential helper script requires a POSIX shell")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "docker-credential-"+suffix)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestResolveRegistryAuth_CredentialHelper(t *testing.T) {
	installFakeCredentialHelper(t, "krakentest", `cat <<'EOF'
{"ServerURL":"example.com","Username":"helper-user","Secret":"helper-pass"}
EOF`)
	writeDockerConfig(t, `{"credHelpers":{"example.com":"krakentest"}}`)

	hostname, ac, found, err := ResolveRegistryAuth("example.com/foo:bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected credentials to be found via credential helper")
	}
	if hostname != "example.com" {
		t.Fatalf("hostname = %q, want %q", hostname, "example.com")
	}
	if ac.Username != "helper-user" || ac.Password != "helper-pass" {
		t.Fatalf("unexpected auth config: %+v", ac)
	}
}

func TestResolveRegistryAuth_CredentialHelperNotFound(t *testing.T) {
	installFakeCredentialHelper(t, "krakentest", `echo "credentials not found in native keychain"; exit 1`)
	writeDockerConfig(t, `{"credHelpers":{"example.com":"krakentest"}}`)

	_, _, found, err := ResolveRegistryAuth("example.com/foo:bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected no credentials when the helper reports not-found")
	}
}

func TestResolveRegistryAuth_CredentialHelperError(t *testing.T) {
	installFakeCredentialHelper(t, "krakentest", `echo "boom"; exit 1`)
	writeDockerConfig(t, `{"credHelpers":{"example.com":"krakentest"}}`)

	_, _, found, err := ResolveRegistryAuth("example.com/foo:bar")
	if err == nil {
		t.Fatal("expected an error when the credential helper fails")
	}
	if found {
		t.Fatal("expected found=false alongside the error")
	}
}

func TestResolveAllRegistryAuth_NoConfigFile(t *testing.T) {
	t.Setenv(config.EnvOverrideConfigDir, t.TempDir())

	if auths := ResolveAllRegistryAuth(); len(auths) != 0 {
		t.Fatalf("expected no credentials, got %+v", auths)
	}
}

func TestResolveAllRegistryAuth_ReturnsEveryRegistry(t *testing.T) {
	authA := base64.StdEncoding.EncodeToString([]byte("user-a:pass-a"))
	authB := base64.StdEncoding.EncodeToString([]byte("user-b:pass-b"))
	writeDockerConfig(t, `{"auths":{
		"registry-a.example.com":{"auth":"`+authA+`"},
		"registry-b.example.com":{"auth":"`+authB+`"}
	}}`)

	auths := ResolveAllRegistryAuth()
	if len(auths) != 2 {
		t.Fatalf("expected 2 registries, got %+v", auths)
	}
	if auths["registry-a.example.com"].Username != "user-a" {
		t.Fatalf("unexpected auth for registry-a: %+v", auths["registry-a.example.com"])
	}
	if auths["registry-b.example.com"].Username != "user-b" {
		t.Fatalf("unexpected auth for registry-b: %+v", auths["registry-b.example.com"])
	}
}

func TestResolveAllRegistryAuth_IncludesCredentialHelperRegistries(t *testing.T) {
	installFakeCredentialHelper(t, "krakentest", `cat <<'EOF'
{"ServerURL":"example.com","Username":"helper-user","Secret":"helper-pass"}
EOF`)
	writeDockerConfig(t, `{"credHelpers":{"example.com":"krakentest"}}`)

	auths := ResolveAllRegistryAuth()
	if auths["example.com"].Username != "helper-user" {
		t.Fatalf("expected credential-helper registry included, got %+v", auths)
	}
}

func TestEncodeAuth(t *testing.T) {
	encoded, err := EncodeAuth(registry.AuthConfig{Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, err := registry.DecodeAuthConfig(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Username != "user" || decoded.Password != "pass" {
		t.Fatalf("unexpected round-tripped auth config: %+v", decoded)
	}
}
