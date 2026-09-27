package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	RuntimeDocker = "docker"
	RuntimePodman = "podman"
)

// dockerSocket is the default Docker socket path.
const dockerSocket = "/var/run/docker.sock"

// podmanSockets lists Podman socket candidates in priority order.
var podmanSockets = []string{
	// $XDG_RUNTIME_DIR/podman/podman.sock
	"",
	// fallback for UID 1000
	"/run/user/1000/podman/podman.sock",
	// rootful podman
	"/run/podman/podman.sock",
}

// New creates a Runtime for the given driver name and optional host override.
//
//   - driver: "docker" | "podman" | "" (auto-detect)
//   - host:   explicit socket path or empty string for auto-detection
func New(driver, host string) (Runtime, error) {
	if host != "" {
		// Explicit host always wins
		return newDockerRuntime(host)
	}

	switch driver {
	case RuntimePodman:
		return newPodmanRuntime()
	case RuntimeDocker:
		return newDockerRuntime("")
	default:
		return autoDetect()
	}
}

// autoDetect tries to connect to Podman first, then falls back to Docker.
func autoDetect() (Runtime, error) {
	// Try Podman socket
	sock := podmanSocketPath()
	if sock != "" {
		if _, err := os.Stat(sock); err == nil {
			return newDockerRuntime(sock)
		}
	}

	// Fall back to Docker
	if _, err := os.Stat(dockerSocket); err == nil {
		return newDockerRuntime("")
	}

	return nil, fmt.Errorf("no container runtime socket found; is Docker or Podman running?")
}

func newPodmanRuntime() (Runtime, error) {
	sock := podmanSocketPath()
	if sock == "" {
		return nil, fmt.Errorf("could not locate podman socket")
	}
	return newDockerRuntime(sock)
}

// podmanSocketPath returns the best Podman socket path available.
func podmanSocketPath() string {
	// Check $XDG_RUNTIME_DIR first
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		p := filepath.Join(xdg, "podman", "podman.sock")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range podmanSockets[1:] {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
