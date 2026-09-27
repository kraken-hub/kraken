package util

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
)

func TestResolveDNS(t *testing.T) {
	t.Run("empty input returns as-is", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		got := ResolveDNS(tk.Kraken, nil)
		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("static entries pass through unchanged", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		got := ResolveDNS(tk.Kraken, []string{"8.8.8.8", "1.1.1.1"})
		want := []string{"8.8.8.8", "1.1.1.1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("container reference resolved to IP", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerNetworkIPFn: func(ctx context.Context, nameOrID, networkName string) (string, error) {
				if nameOrID == "db.myapp" && networkName == GlobalNetwork {
					return "10.0.0.5", nil
				}
				return "", nil
			},
		}
		tk := newTestKraken(t, rt)
		got := ResolveDNS(tk.Kraken, []string{"container:db.myapp", "8.8.8.8"})
		want := []string{"10.0.0.5", "8.8.8.8"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("unresolvable container reference dropped with warning", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerNetworkIPFn: func(ctx context.Context, nameOrID, networkName string) (string, error) {
				return "", errors.New("not found")
			},
		}
		tk := newTestKraken(t, rt)
		got := ResolveDNS(tk.Kraken, []string{"container:missing", "8.8.8.8"})
		want := []string{"8.8.8.8"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
		if !strings.Contains(tk.stdout(), "Warning") {
			t.Errorf("expected warning logged, stdout = %q", tk.stdout())
		}
	})
}

func TestResolveExtraHosts(t *testing.T) {
	t.Run("empty input returns as-is", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		got := ResolveExtraHosts(tk.Kraken, nil)
		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("static hostname:ip entries pass through", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		got := ResolveExtraHosts(tk.Kraken, []string{"myhost:10.0.0.9"})
		want := []string{"myhost:10.0.0.9"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("hostname:container:name resolved to IP", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerNetworkIPFn: func(ctx context.Context, nameOrID, networkName string) (string, error) {
				if nameOrID == "db.myapp" && networkName == GlobalNetwork {
					return "10.0.0.5", nil
				}
				return "", nil
			},
		}
		tk := newTestKraken(t, rt)
		got := ResolveExtraHosts(tk.Kraken, []string{"myhost:container:db.myapp"})
		want := []string{"myhost:10.0.0.5"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("unresolvable entry dropped with warning", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerNetworkIPFn: func(ctx context.Context, nameOrID, networkName string) (string, error) {
				return "", nil
			},
		}
		tk := newTestKraken(t, rt)
		got := ResolveExtraHosts(tk.Kraken, []string{"myhost:container:missing", "static:1.2.3.4"})
		want := []string{"static:1.2.3.4"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
}

func TestEnsureImage(t *testing.T) {
	t.Run("build policy missing skips build when image exists", func(t *testing.T) {
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImageBuildFn: func(ctx context.Context, cfg *runtime.BuildOptions, out io.Writer) error {
				t.Fatal("ImageBuild should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "kraken_myapp_web", Build: &runtime.BuildOptions{Policy: "missing"}}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("build policy missing builds when image absent", func(t *testing.T) {
		var built bool
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return false, nil },
			ImageBuildFn: func(ctx context.Context, cfg *runtime.BuildOptions, out io.Writer) error {
				built = true
				if len(cfg.Tags) == 0 || cfg.Tags[0] != "kraken_myapp_web" {
					t.Errorf("Tags = %#v", cfg.Tags)
				}
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "kraken_myapp_web", Build: &runtime.BuildOptions{Policy: "missing"}}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !built {
			t.Error("expected ImageBuild to be called")
		}
	})

	t.Run("build policy always rebuilds even when image exists", func(t *testing.T) {
		var built bool
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImageBuildFn: func(ctx context.Context, cfg *runtime.BuildOptions, out io.Writer) error {
				built = true
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "kraken_myapp_web", PullPolicy: "always", Build: &runtime.BuildOptions{Policy: "always"}}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !built {
			t.Error("expected ImageBuild to be called")
		}
	})

	t.Run("build error propagates", func(t *testing.T) {
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return false, nil },
			ImageBuildFn: func(ctx context.Context, cfg *runtime.BuildOptions, out io.Writer) error {
				return errors.New("build failed")
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "kraken_myapp_web", Build: &runtime.BuildOptions{}}
		if err := EnsureImage(tk.Kraken, cfg); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("no image and no build is a no-op", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		cfg := &runtime.ContainerOptions{Name: "web.myapp"}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("pull policy always always pulls", func(t *testing.T) {
		var pulled bool
		rt := &fakeRuntime{
			ImagePullFn: func(ctx context.Context, image, platform string, out io.Writer) error {
				pulled = true
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "nginx", PullPolicy: "always"}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pulled {
			t.Error("expected ImagePull to be called")
		}
	})

	t.Run("pull policy never skips pull regardless of existence", func(t *testing.T) {
		rt := &fakeRuntime{
			ImagePullFn: func(ctx context.Context, image, platform string, out io.Writer) error {
				t.Fatal("ImagePull should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "nginx", PullPolicy: "never"}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("pull policy missing pulls only when absent", func(t *testing.T) {
		var pulled bool
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return false, nil },
			ImagePullFn: func(ctx context.Context, image, platform string, out io.Writer) error {
				pulled = true
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "nginx"}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pulled {
			t.Error("expected ImagePull to be called")
		}
	})

	t.Run("pull policy missing skips pull when image exists", func(t *testing.T) {
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImagePullFn: func(ctx context.Context, image, platform string, out io.Writer) error {
				t.Fatal("ImagePull should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "nginx"}
		if err := EnsureImage(tk.Kraken, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("pull error propagates", func(t *testing.T) {
		rt := &fakeRuntime{
			ImagePullFn: func(ctx context.Context, image, platform string, out io.Writer) error {
				return errors.New("pull failed")
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Name: "web.myapp", Image: "nginx", PullPolicy: "always"}
		if err := EnsureImage(tk.Kraken, cfg); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStartContainers(t *testing.T) {
	t.Run("starts filtered service only", func(t *testing.T) {
		var started []string
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return []runtime.ContainerInfo{
					{ID: "1", Name: "web.myapp", Labels: map[string]string{LabelService: "web"}},
					{ID: "2", Name: "db.myapp", Labels: map[string]string{LabelService: "db"}},
				}, nil
			},
			ContainerStartFn: func(ctx context.Context, id string) error {
				started = append(started, id)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := StartContainers(tk.Kraken, "myapp", "web"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(started, []string{"1"}) {
			t.Errorf("started = %v, want [1]", started)
		}
	})

	t.Run("starts all when no filter", func(t *testing.T) {
		var started []string
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return []runtime.ContainerInfo{
					{ID: "1", Name: "web.myapp", Labels: map[string]string{LabelService: "web"}},
					{ID: "2", Name: "db.myapp", Labels: map[string]string{LabelService: "db"}},
				}, nil
			},
			ContainerStartFn: func(ctx context.Context, id string) error {
				started = append(started, id)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := StartContainers(tk.Kraken, "myapp", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sort.Strings(started)
		if !reflect.DeepEqual(started, []string{"1", "2"}) {
			t.Errorf("started = %v, want [1 2]", started)
		}
	})

	t.Run("start error is logged, not fatal", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return []runtime.ContainerInfo{{ID: "1", Name: "web.myapp", Labels: map[string]string{LabelService: "web"}}}, nil
			},
			ContainerStartFn: func(ctx context.Context, id string) error {
				return errors.New("boom")
			},
		}
		tk := newTestKraken(t, rt)
		if err := StartContainers(tk.Kraken, "myapp", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ContainerList error propagates", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return nil, errors.New("boom")
			},
		}
		tk := newTestKraken(t, rt)
		if err := StartContainers(tk.Kraken, "myapp", ""); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStopContainers(t *testing.T) {
	t.Run("stops only running containers matching filter", func(t *testing.T) {
		var stopped []string
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return []runtime.ContainerInfo{
					{ID: "1", Name: "web.myapp", State: "running", Labels: map[string]string{LabelService: "web"}},
					{ID: "2", Name: "db.myapp", State: "exited", Labels: map[string]string{LabelService: "db"}},
					{ID: "3", Name: "cache.myapp", State: "running", Labels: map[string]string{LabelService: "cache"}},
				}, nil
			},
			ContainerStopFn: func(ctx context.Context, id string, timeout *time.Duration) error {
				stopped = append(stopped, id)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := StopContainers(tk.Kraken, "myapp", "", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sort.Strings(stopped)
		if !reflect.DeepEqual(stopped, []string{"1", "3"}) {
			t.Errorf("stopped = %v, want [1 3] (exited and non-matching excluded)", stopped)
		}
	})

	t.Run("stop error is logged, not fatal", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return []runtime.ContainerInfo{{ID: "1", Name: "web.myapp", State: "running", Labels: map[string]string{LabelService: "web"}}}, nil
			},
			ContainerStopFn: func(ctx context.Context, id string, timeout *time.Duration) error {
				return errors.New("boom")
			},
		}
		tk := newTestKraken(t, rt)
		if err := StopContainers(tk.Kraken, "myapp", "", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRemoveContainers(t *testing.T) {
	t.Run("removes filtered service only", func(t *testing.T) {
		var removed []string
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return []runtime.ContainerInfo{
					{ID: "1", Name: "web.myapp", Labels: map[string]string{LabelService: "web"}},
					{ID: "2", Name: "db.myapp", Labels: map[string]string{LabelService: "db"}},
				}, nil
			},
			ContainerRemoveFn: func(ctx context.Context, id string, force, removeVolumes bool) error {
				removed = append(removed, id)
				if !removeVolumes {
					t.Error("expected removeVolumes=true")
				}
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := RemoveContainers(tk.Kraken, "myapp", "db", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(removed, []string{"2"}) {
			t.Errorf("removed = %v, want [2]", removed)
		}
	})

	t.Run("remove error is logged, not fatal", func(t *testing.T) {
		rt := &fakeRuntime{
			ContainerListFn: func(ctx context.Context, all bool, labels map[string]string) ([]runtime.ContainerInfo, error) {
				return []runtime.ContainerInfo{{ID: "1", Name: "web.myapp", Labels: map[string]string{LabelService: "web"}}}, nil
			},
			ContainerRemoveFn: func(ctx context.Context, id string, force, removeVolumes bool) error {
				return errors.New("boom")
			},
		}
		tk := newTestKraken(t, rt)
		if err := RemoveContainers(tk.Kraken, "myapp", "", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRemoveImages(t *testing.T) {
	t.Setenv("KRAKEN_HOME", t.TempDir())

	t.Run("app not found errors", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		if err := RemoveImages(tk.Kraken, "nonexistent", ""); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("removes existing images for services, skips missing", func(t *testing.T) {
		if err := store.SaveApp(&store.App{
			Key: "removeimgapp",
			Value: store.AppValue{
				Path:   "/app/removeimgapp",
				Config: store.AppConfig{Data: "services:\n  web:\n    build: .\n  db:\n    image: postgres\n  cache:\n    image: redis\n"},
			},
		}); err != nil {
			t.Fatal(err)
		}

		var removedImages []string
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) {
				return image == BuiltImageName("removeimgapp", "web") || image == "postgres", nil
			},
			ImageRemoveFn: func(ctx context.Context, image string, force bool) error {
				removedImages = append(removedImages, image)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := RemoveImages(tk.Kraken, "removeimgapp", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{BuiltImageName("removeimgapp", "web"), "postgres"}
		if !reflect.DeepEqual(removedImages, want) {
			t.Errorf("removedImages = %v, want %v", removedImages, want)
		}
	})

	t.Run("service filter narrows removal", func(t *testing.T) {
		if err := store.SaveApp(&store.App{
			Key: "removeimgapp2",
			Value: store.AppValue{
				Path:   "/app/removeimgapp2",
				Config: store.AppConfig{Data: "services:\n  web:\n    build: .\n  worker:\n    build: .\n"},
			},
		}); err != nil {
			t.Fatal(err)
		}

		var removedImages []string
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImageRemoveFn: func(ctx context.Context, image string, force bool) error {
				removedImages = append(removedImages, image)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := RemoveImages(tk.Kraken, "removeimgapp2", "worker"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{BuiltImageName("removeimgapp2", "worker")}
		if !reflect.DeepEqual(removedImages, want) {
			t.Errorf("removedImages = %v, want %v", removedImages, want)
		}
	})

	t.Run("remove image error is logged, not fatal", func(t *testing.T) {
		if err := store.SaveApp(&store.App{
			Key: "removeimgapp3",
			Value: store.AppValue{
				Path:   "/app/removeimgapp3",
				Config: store.AppConfig{Data: "services:\n  web:\n    build: .\n"},
			},
		}); err != nil {
			t.Fatal(err)
		}
		rt := &fakeRuntime{
			ImageExistsFn: func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImageRemoveFn: func(ctx context.Context, image string, force bool) error {
				return errors.New("in use")
			},
		}
		tk := newTestKraken(t, rt)
		if err := RemoveImages(tk.Kraken, "removeimgapp3", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCreateContainers(t *testing.T) {
	t.Setenv("KRAKEN_HOME", t.TempDir())

	t.Run("app not found errors", func(t *testing.T) {
		tk := newTestKraken(t, &fakeRuntime{})
		if err := CreateContainers(tk.Kraken, "nonexistent", "", false); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("creates container for single service without starting", func(t *testing.T) {
		appPath := t.TempDir()
		if err := store.SaveApp(&store.App{
			Key: "createapp",
			Value: store.AppValue{
				Path:   appPath,
				Config: store.AppConfig{Data: "services:\n  web:\n    image: nginx\n"},
			},
		}); err != nil {
			t.Fatal(err)
		}

		var createdName string
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return true, "net-" + name, nil },
			ImageExistsFn:   func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImageVolumesFn:  func(ctx context.Context, image string) ([]string, error) { return nil, nil },
			ContainerExistsFn: func(ctx context.Context, nameOrID string) (bool, error) {
				return false, nil
			},
			ContainerCreateFn: func(ctx context.Context, cfg *runtime.ContainerOptions) (string, error) {
				createdName = cfg.Name
				return "container-id-1", nil
			},
			NetworkConnectFn: func(ctx context.Context, networkID, containerID string, aliases ...string) error {
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := CreateContainers(tk.Kraken, "createapp", "", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createdName != "web.createapp" {
			t.Errorf("createdName = %q, want web.createapp", createdName)
		}
	})

	t.Run("skips creation and starts when container already exists", func(t *testing.T) {
		appPath := t.TempDir()
		if err := store.SaveApp(&store.App{
			Key: "createapp2",
			Value: store.AppValue{
				Path:   appPath,
				Config: store.AppConfig{Data: "services:\n  web:\n    image: nginx\n"},
			},
		}); err != nil {
			t.Fatal(err)
		}

		var started bool
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return true, "net-" + name, nil },
			ImageExistsFn:   func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImageVolumesFn:  func(ctx context.Context, image string) ([]string, error) { return nil, nil },
			ContainerExistsFn: func(ctx context.Context, nameOrID string) (bool, error) {
				return true, nil
			},
			ContainerStartFn: func(ctx context.Context, id string) error {
				started = true
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := CreateContainers(tk.Kraken, "createapp2", "", true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !started {
			t.Error("expected existing container to be started")
		}
	})

	t.Run("service filter limits to one service", func(t *testing.T) {
		appPath := t.TempDir()
		if err := store.SaveApp(&store.App{
			Key: "createapp3",
			Value: store.AppValue{
				Path:   appPath,
				Config: store.AppConfig{Data: "services:\n  web:\n    image: nginx\n  db:\n    image: postgres\n"},
			},
		}); err != nil {
			t.Fatal(err)
		}

		var createdNames []string
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) { return true, "net-" + name, nil },
			ImageExistsFn:   func(ctx context.Context, image string) (bool, error) { return true, nil },
			ImageVolumesFn:  func(ctx context.Context, image string) ([]string, error) { return nil, nil },
			ContainerExistsFn: func(ctx context.Context, nameOrID string) (bool, error) {
				return false, nil
			},
			ContainerCreateFn: func(ctx context.Context, cfg *runtime.ContainerOptions) (string, error) {
				createdNames = append(createdNames, cfg.Name)
				return "cid", nil
			},
			NetworkConnectFn: func(ctx context.Context, networkID, containerID string, aliases ...string) error {
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		if err := CreateContainers(tk.Kraken, "createapp3", "db", false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"db.createapp3"}
		if !reflect.DeepEqual(createdNames, want) {
			t.Errorf("createdNames = %v, want %v", createdNames, want)
		}
	})
}
