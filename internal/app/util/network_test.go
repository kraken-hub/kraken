package util

import (
	"context"
	"testing"
)

func TestEnsureNetworks(t *testing.T) {
	t.Run("creates both networks when neither exists", func(t *testing.T) {
		var created []string
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", nil
			},
			NetworkCreateFn: func(ctx context.Context, name string, labels map[string]string) (string, error) {
				created = append(created, name)
				return "id-" + name, nil
			},
		}
		tk := newTestKraken(t, rt)

		globalID, appID, err := EnsureNetworks(tk.Kraken, "myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if globalID != "id-kraken_net" {
			t.Errorf("globalID = %q, want id-kraken_net", globalID)
		}
		if appID != "id-kraken_net.myapp" {
			t.Errorf("appID = %q, want id-kraken_net.myapp", appID)
		}
		want := []string{"kraken_net", "kraken_net.myapp"}
		if len(created) != 2 || created[0] != want[0] || created[1] != want[1] {
			t.Errorf("created = %v, want %v", created, want)
		}
	})

	t.Run("skips creation when networks already exist", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return true, "existing-" + name, nil
			},
			NetworkCreateFn: func(ctx context.Context, name string, labels map[string]string) (string, error) {
				t.Fatalf("NetworkCreate should not be called for %q", name)
				return "", nil
			},
		}
		tk := newTestKraken(t, rt)

		globalID, appID, err := EnsureNetworks(tk.Kraken, "myapp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if globalID != "existing-kraken_net" || appID != "existing-kraken_net.myapp" {
			t.Errorf("got (%q,%q)", globalID, appID)
		}
	})

	t.Run("propagates NetworkExists error", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)

		if _, _, err := EnsureNetworks(tk.Kraken, "myapp"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("propagates NetworkCreate error", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", nil
			},
			NetworkCreateFn: func(ctx context.Context, name string, labels map[string]string) (string, error) {
				return "", context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)

		if _, _, err := EnsureNetworks(tk.Kraken, "myapp"); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRemoveNetwork(t *testing.T) {
	t.Run("removes network when it exists", func(t *testing.T) {
		var removedID string
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				if name == "kraken_net.myapp" {
					return true, "net-id-123", nil
				}
				return false, "", nil
			},
			NetworkRemoveFn: func(ctx context.Context, id string) error {
				removedID = id
				return nil
			},
		}
		tk := newTestKraken(t, rt)

		RemoveNetwork(tk.Kraken, "myapp")

		if removedID != "net-id-123" {
			t.Errorf("removedID = %q, want net-id-123", removedID)
		}
	})

	t.Run("does nothing when network does not exist", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", nil
			},
			NetworkRemoveFn: func(ctx context.Context, id string) error {
				t.Fatal("NetworkRemove should not be called")
				return nil
			},
		}
		tk := newTestKraken(t, rt)

		RemoveNetwork(tk.Kraken, "myapp")
	})

	t.Run("does nothing on NetworkExists error", func(t *testing.T) {
		rt := &fakeRuntime{
			NetworkExistsFn: func(ctx context.Context, name string) (bool, string, error) {
				return false, "", context.DeadlineExceeded
			},
		}
		tk := newTestKraken(t, rt)

		RemoveNetwork(tk.Kraken, "myapp")
	})
}
