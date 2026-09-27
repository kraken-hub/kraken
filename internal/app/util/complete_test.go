package util

import (
	"reflect"
	"sort"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/store"
)

func TestCompleteApps(t *testing.T) {
	t.Run("no apps registered", func(t *testing.T) {
		t.Setenv("KRAKEN_HOME", t.TempDir())
		names, directive := CompleteApps(nil, nil, "")
		if len(names) != 0 {
			t.Errorf("names = %v, want empty", names)
		}
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v", directive)
		}
	})

	t.Run("returns registered app names", func(t *testing.T) {
		t.Setenv("KRAKEN_HOME", t.TempDir())
		if err := store.SaveApp(&store.App{Key: "app1", Value: store.AppValue{Path: "/tmp/app1"}}); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveApp(&store.App{Key: "app2", Value: store.AppValue{Path: "/tmp/app2"}}); err != nil {
			t.Fatal(err)
		}
		names, _ := CompleteApps(nil, nil, "")
		sort.Strings(names)
		want := []string{"app1", "app2"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})
}

func TestCompleteRefs(t *testing.T) {
	t.Run("no apps registered", func(t *testing.T) {
		t.Setenv("KRAKEN_HOME", t.TempDir())
		results, directive := CompleteRefs(nil, nil, "")
		if len(results) != 0 {
			t.Errorf("results = %v, want empty", results)
		}
		if directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("directive = %v", directive)
		}
	})

	t.Run("includes app names and service refs", func(t *testing.T) {
		t.Setenv("KRAKEN_HOME", t.TempDir())
		configData := "services:\n  web:\n    image: nginx\n  db:\n    image: postgres\n"
		if err := store.SaveApp(&store.App{
			Key: "myapp",
			Value: store.AppValue{
				Path:   "/tmp/myapp",
				Config: store.AppConfig{Data: configData},
			},
		}); err != nil {
			t.Fatal(err)
		}
		results, _ := CompleteRefs(nil, nil, "")
		sort.Strings(results)
		want := []string{"db.myapp", "myapp", "web.myapp"}
		if !reflect.DeepEqual(results, want) {
			t.Errorf("results = %v, want %v", results, want)
		}
	})

	t.Run("app with no config snapshot still lists app name", func(t *testing.T) {
		t.Setenv("KRAKEN_HOME", t.TempDir())
		if err := store.SaveApp(&store.App{Key: "bareapp", Value: store.AppValue{Path: "/tmp/bareapp"}}); err != nil {
			t.Fatal(err)
		}
		results, _ := CompleteRefs(nil, nil, "")
		want := []string{"bareapp"}
		if !reflect.DeepEqual(results, want) {
			t.Errorf("results = %v, want %v", results, want)
		}
	})
}
