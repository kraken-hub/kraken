package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func daemonHost(t *testing.T, rt Runtime) string {
	t.Helper()
	dr, ok := rt.(*DockerRuntime)
	if !ok {
		t.Fatalf("expected *DockerRuntime, got %T", rt)
	}
	if dr.cli == nil {
		t.Fatalf("DockerRuntime.cli is nil")
	}
	return dr.cli.DaemonHost()
}

func TestNew_ExplicitHostWinsOverDriver(t *testing.T) {
	cases := []struct {
		name   string
		driver string
	}{
		{"empty driver", ""},
		{"podman driver", RuntimePodman},
		{"docker driver", RuntimeDocker},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const host = "/some/explicit/socket.sock"
			rt, err := New(tc.driver, host)
			if err != nil {
				t.Fatalf("New(%q, %q) returned error: %v", tc.driver, host, err)
			}
			got := daemonHost(t, rt)
			want := "unix://" + host
			if got != want {
				t.Errorf("DaemonHost() = %q, want %q", got, want)
			}
		})
	}
}

func TestNew_DockerDriverNoHost(t *testing.T) {
	rt, err := New(RuntimeDocker, "")
	if err != nil {
		t.Fatalf("New(%q, \"\") returned error: %v", RuntimeDocker, err)
	}
	if _, ok := rt.(*DockerRuntime); !ok {
		t.Fatalf("expected *DockerRuntime, got %T", rt)
	}
}

func TestPodmanSocketPath_XDGRuntimeDirFound(t *testing.T) {
	dir := t.TempDir()
	podmanDir := filepath.Join(dir, "podman")
	if err := os.MkdirAll(podmanDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sockPath := filepath.Join(podmanDir, "podman.sock")
	if err := os.WriteFile(sockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("XDG_RUNTIME_DIR", dir)

	got := podmanSocketPath()
	if got != sockPath {
		t.Errorf("podmanSocketPath() = %q, want %q", got, sockPath)
	}
}

func TestPodmanSocketPath_XDGRuntimeDirSetButSocketMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)

	if _, err := os.Stat("/run/user/1000/podman/podman.sock"); err == nil {
		t.Skip("real /run/user/1000/podman/podman.sock exists on this host; cannot test fallback-to-empty deterministically")
	}
	if _, err := os.Stat("/run/podman/podman.sock"); err == nil {
		t.Skip("real /run/podman/podman.sock exists on this host; cannot test fallback-to-empty deterministically")
	}

	got := podmanSocketPath()
	if got != "" {
		t.Errorf("podmanSocketPath() = %q, want empty string", got)
	}
}

func TestPodmanSocketPath_NoXDGRuntimeDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")

	if _, err := os.Stat("/run/user/1000/podman/podman.sock"); err == nil {
		t.Skip("real /run/user/1000/podman/podman.sock exists on this host; cannot test empty-result deterministically")
	}
	if _, err := os.Stat("/run/podman/podman.sock"); err == nil {
		t.Skip("real /run/podman/podman.sock exists on this host; cannot test empty-result deterministically")
	}

	got := podmanSocketPath()
	if got != "" {
		t.Errorf("podmanSocketPath() = %q, want empty string", got)
	}
}

func TestNewPodmanRuntime_SocketFound(t *testing.T) {
	dir := t.TempDir()
	podmanDir := filepath.Join(dir, "podman")
	if err := os.MkdirAll(podmanDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sockPath := filepath.Join(podmanDir, "podman.sock")
	if err := os.WriteFile(sockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)

	rt, err := newPodmanRuntime()
	if err != nil {
		t.Fatalf("newPodmanRuntime() returned error: %v", err)
	}
	got := daemonHost(t, rt)
	want := "unix://" + sockPath
	if got != want {
		t.Errorf("DaemonHost() = %q, want %q", got, want)
	}
}

func TestNewPodmanRuntime_NoSocket(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")

	if _, err := os.Stat("/run/user/1000/podman/podman.sock"); err == nil {
		t.Skip("real /run/user/1000/podman/podman.sock exists on this host")
	}
	if _, err := os.Stat("/run/podman/podman.sock"); err == nil {
		t.Skip("real /run/podman/podman.sock exists on this host")
	}

	_, err := newPodmanRuntime()
	if err == nil {
		t.Fatal("expected error when no podman socket is found, got nil")
	}
}

func TestNew_PodmanDriverViaSocket(t *testing.T) {
	dir := t.TempDir()
	podmanDir := filepath.Join(dir, "podman")
	if err := os.MkdirAll(podmanDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	sockPath := filepath.Join(podmanDir, "podman.sock")
	if err := os.WriteFile(sockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)

	rt, err := New(RuntimePodman, "")
	if err != nil {
		t.Fatalf("New(%q, \"\") returned error: %v", RuntimePodman, err)
	}
	got := daemonHost(t, rt)
	want := "unix://" + sockPath
	if got != want {
		t.Errorf("DaemonHost() = %q, want %q", got, want)
	}
}

// autoDetect() depends on the real absolute paths /var/run/docker.sock and
// /run/podman/podman.sock (and /run/user/1000/podman/podman.sock), none of
// which are overridable via environment variables or parameters. Its
// "nothing found" error branch and its docker-fallback branch therefore
// cannot be exercised hermetically without either a real daemon socket
// present or modifying production code to inject the paths. We only assert
// that calling it does not panic and returns a result consistent with
// whether a podman or docker socket actually exists on this machine.
func TestAutoDetect_DoesNotPanicAndIsConsistentWithHostState(t *testing.T) {
	rt, err := autoDetect()

	podmanSock := podmanSocketPath()
	_, podmanErr := os.Stat(podmanSock)
	podmanAvailable := podmanSock != "" && podmanErr == nil

	_, dockerErr := os.Stat(dockerSocket)
	dockerAvailable := dockerErr == nil

	switch {
	case podmanAvailable || dockerAvailable:
		if err != nil {
			t.Errorf("autoDetect() returned error %v despite a socket being available (podman=%v, docker=%v)", err, podmanAvailable, dockerAvailable)
		}
		if rt == nil {
			t.Error("autoDetect() returned nil Runtime despite no error")
		}
	default:
		if err == nil {
			t.Error("autoDetect() returned no error despite no known socket being available")
		}
		if rt != nil {
			t.Error("autoDetect() returned non-nil Runtime alongside an error")
		}
	}
}
