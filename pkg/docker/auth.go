package docker

import (
	"fmt"
	"os"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types/registry"
)

// dockerHubServerAddress is the auths key docker login and
// ~/.docker/config.json use for the default registry (Docker Hub), distinct
// from the "docker.io" hostname distribution/reference normalizes bare image
// names to.
const dockerHubServerAddress = "https://index.docker.io/v1/"

// ResolveRegistryAuth resolves registry credentials for image from the
// Docker CLI config file (~/.docker/config.json), following the same lookup
// order as the docker CLI: a per-registry credHelpers entry, the global
// credsStore, or the static auths entry. hostname is the registry server
// address the credentials were resolved for (needed as the map key for
// ImageBuildOptions.AuthConfigs). found is false, with a nil error, if no
// credentials are configured for the image's registry, in which case the
// pull or build should proceed anonymously.
func ResolveRegistryAuth(image string) (hostname string, auth registry.AuthConfig, found bool, err error) {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return "", registry.AuthConfig{}, false, nil
	}

	hostname = reference.Domain(named)
	if hostname == "docker.io" {
		hostname = dockerHubServerAddress
	}

	// config.Dir() caches its result behind a sync.Once, so a DOCKER_CONFIG
	// override is read directly here instead of relying on that cache.
	configDir := os.Getenv(config.EnvOverrideConfigDir)
	if configDir == "" {
		configDir = config.Dir()
	}
	cfg, err := config.Load(configDir)
	if err != nil {
		return hostname, registry.AuthConfig{}, false, fmt.Errorf("load docker config: %w", err)
	}

	cliAuth, err := cfg.GetAuthConfig(hostname)
	if err != nil {
		return hostname, registry.AuthConfig{}, false, fmt.Errorf("resolve credentials for registry %q: %w", hostname, err)
	}
	if cliAuth == (types.AuthConfig{}) {
		return hostname, registry.AuthConfig{}, false, nil
	}

	return hostname, registry.AuthConfig{
		Username:      cliAuth.Username,
		Password:      cliAuth.Password,
		Auth:          cliAuth.Auth,
		Email:         cliAuth.Email,
		ServerAddress: cliAuth.ServerAddress,
		IdentityToken: cliAuth.IdentityToken,
		RegistryToken: cliAuth.RegistryToken,
	}, true, nil
}

// EncodeAuth base64-encodes auth as the Docker Engine API's registry auth
// header (and the Go SDK's PullOptions.RegistryAuth) expect.
func EncodeAuth(auth registry.AuthConfig) (string, error) {
	return registry.EncodeAuthConfig(auth)
}

// ResolveAllRegistryAuth returns every registry credential configured in the
// Docker CLI config file (~/.docker/config.json) — the default credential
// store plus every per-registry credHelpers entry — keyed by registry
// hostname. This mirrors how `docker build` populates
// ImageBuildOptions.AuthConfigs (see docker/cli's cli/command/image/build.go):
// the client has no way to know in advance which registries a Dockerfile's
// FROM images (possibly built from ARGs) will resolve to, so it sends every
// credential it has and lets the daemon match by hostname at pull time.
// Best-effort: a failing credential helper is skipped rather than blocking
// builds that don't need it, matching the docker CLI's own behavior of
// discarding this lookup's error.
func ResolveAllRegistryAuth() map[string]registry.AuthConfig {
	configDir := os.Getenv(config.EnvOverrideConfigDir)
	if configDir == "" {
		configDir = config.Dir()
	}
	cfg, err := config.Load(configDir)
	if err != nil {
		return nil
	}

	creds, _ := cfg.GetAllCredentials()
	if len(creds) == 0 {
		return nil
	}

	authConfigs := make(map[string]registry.AuthConfig, len(creds))
	for hostname, c := range creds {
		authConfigs[hostname] = registry.AuthConfig{
			Username:      c.Username,
			Password:      c.Password,
			Auth:          c.Auth,
			Email:         c.Email,
			ServerAddress: c.ServerAddress,
			IdentityToken: c.IdentityToken,
			RegistryToken: c.RegistryToken,
		}
	}
	return authConfigs
}
