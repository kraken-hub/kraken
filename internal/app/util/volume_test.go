package util

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
)

func TestAppVolumeName(t *testing.T) {
	if got := AppVolumeName("myapp", "data"); got != "kraken_vol.myapp.data" {
		t.Errorf("AppVolumeName() = %q, want kraken_vol.myapp.data", got)
	}
}

func TestServiceVolumeName(t *testing.T) {
	sum := md5.Sum([]byte("/var/lib/data"))
	want := fmt.Sprintf("kraken_vol.myapp.web.%x", sum)
	if got := ServiceVolumeName("myapp", "web", "/var/lib/data"); got != want {
		t.Errorf("ServiceVolumeName() = %q, want %q", got, want)
	}
}

func appEntryWithConfig(data string) *store.App {
	return &store.App{
		Key: "myapp",
		Value: store.AppValue{
			Path:   "/app/myapp",
			Config: store.AppConfig{Data: data},
		},
	}
}

func TestEnsureVolumes(t *testing.T) {
	t.Run("no volumes section returns empty map", func(t *testing.T) {
		rt := &fakeRuntime{}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("services:\n  web:\n    image: nginx\n")
		got, err := EnsureVolumes(tk.Kraken, "myapp", appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %#v, want empty", got)
		}
	})

	t.Run("null volume is skipped (auto-created later)", func(t *testing.T) {
		rt := &fakeRuntime{}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n")
		got, err := EnsureVolumes(tk.Kraken, "myapp", appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %#v, want empty", got)
		}
	})

	t.Run("simple named volume without driver is skipped", func(t *testing.T) {
		rt := &fakeRuntime{}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    labels:\n      foo: bar\n")
		got, err := EnsureVolumes(tk.Kraken, "myapp", appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %#v, want empty", got)
		}
	})

	t.Run("external volume that exists is used as-is", func(t *testing.T) {
		var checked string
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				checked = name
				return true, nil
			},
		}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    external: true\n")
		got, err := EnsureVolumes(tk.Kraken, "myapp", appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if checked != "data" {
			t.Errorf("checked = %q, want data", checked)
		}
		want := map[string]string{"data": "data"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("external volume with explicit name", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return name == "actual-name", nil
			},
		}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    external:\n      name: actual-name\n")
		got, err := EnsureVolumes(tk.Kraken, "myapp", appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"data": "actual-name"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("external volume missing errors", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return false, nil
			},
		}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    external: true\n")
		if _, err := EnsureVolumes(tk.Kraken, "myapp", appEntry); err == nil {
			t.Fatal("expected error for missing external volume")
		}
	})

	t.Run("VolumeExists error propagates for external volume", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return false, context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    external: true\n")
		if _, err := EnsureVolumes(tk.Kraken, "myapp", appEntry); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("driver volume created when missing", func(t *testing.T) {
		var createdOpts runtime.VolumeCreateOptions
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return false, nil
			},
			VolumeCreateFn: func(ctx context.Context, opts runtime.VolumeCreateOptions) error {
				createdOpts = opts
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    driver: local\n    driver_opts:\n      type: nfs\n    labels:\n      team: infra\n")
		got, err := EnsureVolumes(tk.Kraken, "myapp", appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantName := "kraken_vol.myapp.data"
		if createdOpts.Name != wantName {
			t.Errorf("Name = %q, want %q", createdOpts.Name, wantName)
		}
		if createdOpts.Driver != "local" {
			t.Errorf("Driver = %q, want local", createdOpts.Driver)
		}
		if createdOpts.DriverOpts["type"] != "nfs" {
			t.Errorf("DriverOpts = %#v", createdOpts.DriverOpts)
		}
		if createdOpts.Labels[LabelApp] != "myapp" || createdOpts.Labels["kraken.target"] != "data" || createdOpts.Labels["team"] != "infra" {
			t.Errorf("Labels = %#v", createdOpts.Labels)
		}
		want := map[string]string{"data": wantName}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("driver volume not recreated when it already exists", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return true, nil
			},
			VolumeCreateFn: func(ctx context.Context, opts runtime.VolumeCreateOptions) error {
				t.Fatal("VolumeCreate should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    driver: local\n")
		got, err := EnsureVolumes(tk.Kraken, "myapp", appEntry)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"data": "kraken_vol.myapp.data"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("VolumeCreate error propagates", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return false, nil
			},
			VolumeCreateFn: func(ctx context.Context, opts runtime.VolumeCreateOptions) error {
				return context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)
		appEntry := appEntryWithConfig("volumes:\n  data:\n    driver: local\n")
		if _, err := EnsureVolumes(tk.Kraken, "myapp", appEntry); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestEnsureServiceVolumes(t *testing.T) {
	t.Run("adds managed volumes for uncovered image VOLUME paths", func(t *testing.T) {
		var createdNames []string
		rt := &fakeRuntime{
			ImageVolumesFn: func(ctx context.Context, image string) ([]string, error) {
				return []string{"/data", "/covered"}, nil
			},
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return false, nil
			},
			VolumeCreateFn: func(ctx context.Context, opts runtime.VolumeCreateOptions) error {
				createdNames = append(createdNames, opts.Name)
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{
			Image: "myimage",
			Volumes: []runtime.VolumeMount{
				{Source: ServiceVolumeName("myapp", "web", "/covered"), Target: "/covered"},
			},
		}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Volumes) != 2 {
			t.Fatalf("Volumes = %#v, want 2 entries", cfg.Volumes)
		}
		found := false
		for _, vm := range cfg.Volumes {
			if vm.Target == "/data" {
				found = true
				if vm.Source != ServiceVolumeName("myapp", "web", "/data") {
					t.Errorf("Source = %q", vm.Source)
				}
			}
		}
		if !found {
			t.Error("expected /data volume mount to be added")
		}
		sort.Strings(createdNames)
		want := []string{ServiceVolumeName("myapp", "web", "/covered"), ServiceVolumeName("myapp", "web", "/data")}
		sort.Strings(want)
		if !reflect.DeepEqual(createdNames, want) {
			t.Errorf("createdNames = %v, want %v", createdNames, want)
		}
	})

	t.Run("skips ImageVolumes lookup when no image set", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return true, nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("creates bind-mount host directory", func(t *testing.T) {
		rt := &fakeRuntime{}
		tk := newTestKraken(t, rt)
		hostDir := filepath.Join(t.TempDir(), "nested", "bind")
		cfg := &runtime.ContainerOptions{
			Volumes: []runtime.VolumeMount{
				{Source: hostDir, Target: "/data"},
			},
		}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		info, err := os.Stat(hostDir)
		if err != nil {
			t.Fatalf("expected bind-mount dir to be created: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected a directory")
		}
	})

	t.Run("bind-mounting an existing file is not an error", func(t *testing.T) {
		rt := &fakeRuntime{}
		tk := newTestKraken(t, rt)
		file := filepath.Join(t.TempDir(), ".dotfile")
		if err := os.WriteFile(file, []byte("alias l=ls\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := &runtime.ContainerOptions{
			Volumes: []runtime.VolumeMount{{Source: file, Target: "/container/.dotfile"}},
		}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info, err := os.Stat(file); err != nil || info.IsDir() {
			t.Errorf("file should be untouched, err=%v", err)
		}
	})

	t.Run("skips anonymous volumes", func(t *testing.T) {
		rt := &fakeRuntime{}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{
			Volumes: []runtime.VolumeMount{
				{Source: "", Target: "/data"},
			},
		}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("does not recreate an existing named volume", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return true, nil
			},
			VolumeCreateFn: func(ctx context.Context, opts runtime.VolumeCreateOptions) error {
				t.Fatal("VolumeCreate should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{
			Volumes: []runtime.VolumeMount{
				{Source: "kraken_vol.myapp.web.abc", Target: "/data"},
			},
		}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("creates missing named volume with labels", func(t *testing.T) {
		var createdOpts runtime.VolumeCreateOptions
		rt := &fakeRuntime{
			VolumeExistsFn: func(ctx context.Context, name string) (bool, error) {
				return false, nil
			},
			VolumeCreateFn: func(ctx context.Context, opts runtime.VolumeCreateOptions) error {
				createdOpts = opts
				return nil
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{
			Volumes: []runtime.VolumeMount{
				{Source: "kraken_vol.myapp.web.abc", Target: "/data"},
			},
		}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if createdOpts.Name != "kraken_vol.myapp.web.abc" {
			t.Errorf("Name = %q", createdOpts.Name)
		}
		if createdOpts.Labels[LabelApp] != "myapp" || createdOpts.Labels[LabelService] != "web" || createdOpts.Labels["kraken.target"] != "/data" {
			t.Errorf("Labels = %#v", createdOpts.Labels)
		}
	})

	t.Run("ImageVolumes error propagates", func(t *testing.T) {
		rt := &fakeRuntime{
			ImageVolumesFn: func(ctx context.Context, image string) ([]string, error) {
				return nil, context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)
		cfg := &runtime.ContainerOptions{Image: "myimage"}
		if err := EnsureServiceVolumes(tk.Kraken, "myapp", "web", cfg); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestPurgeApp(t *testing.T) {
	var listedPrefix string
	var removed []string
	rt := &fakeRuntime{
		VolumeListFn: func(ctx context.Context, namePrefix string) ([]string, error) {
			listedPrefix = namePrefix
			return []string{"kraken_vol.myapp.data", "kraken_vol.myapp.web.abc"}, nil
		},
		VolumeRemoveFn: func(ctx context.Context, name string, force bool) error {
			removed = append(removed, name)
			return nil
		},
	}
	tk := newTestKraken(t, rt)
	if err := PurgeApp(tk.Kraken, "myapp", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if listedPrefix != "kraken_vol.myapp." {
		t.Errorf("listedPrefix = %q, want kraken_vol.myapp.", listedPrefix)
	}
	sort.Strings(removed)
	want := []string{"kraken_vol.myapp.data", "kraken_vol.myapp.web.abc"}
	if !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
}

func TestPurgeService(t *testing.T) {
	var listedPrefix string
	rt := &fakeRuntime{
		VolumeListFn: func(ctx context.Context, namePrefix string) ([]string, error) {
			listedPrefix = namePrefix
			return nil, nil
		},
	}
	tk := newTestKraken(t, rt)
	if err := PurgeService(tk.Kraken, "myapp", "web"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if listedPrefix != "kraken_vol.myapp.web." {
		t.Errorf("listedPrefix = %q, want kraken_vol.myapp.web.", listedPrefix)
	}
}

func TestRemoveVolumesByPrefixErrors(t *testing.T) {
	t.Run("VolumeList error propagates", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeListFn: func(ctx context.Context, namePrefix string) ([]string, error) {
				return nil, context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)
		if err := PurgeApp(tk.Kraken, "myapp", nil); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("VolumeRemove error is logged but not fatal", func(t *testing.T) {
		rt := &fakeRuntime{
			VolumeListFn: func(ctx context.Context, namePrefix string) ([]string, error) {
				return []string{"kraken_vol.myapp.data"}, nil
			},
			VolumeRemoveFn: func(ctx context.Context, name string, force bool) error {
				return context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)
		if err := PurgeApp(tk.Kraken, "myapp", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
