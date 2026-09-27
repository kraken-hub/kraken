package plugins

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/jsplugin"
	"github.com/kraken-hub/kraken/internal/store"
)

func doInit(kraken *app.Kraken) {
	loader := jsplugin.NewLoader(kraken)
	if err := loader.LoadDir(store.PluginsDir()); err != nil {
		kraken.Logger().Warn("failed to load plugins: %v", err)
	}

	var flagFormat string

	// plugins — list
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "List plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manifests, err := listPlugins()
			if err != nil {
				return err
			}
			if flagFormat == "json" {
				return util.EncodeJSON(kraken.Out(), manifests)
			}
			headers := []string{"Name", "Version"}
			widths := []int{len(headers[0]), len(headers[1])}
			for _, m := range manifests {
				if len(m.Name) > widths[0] {
					widths[0] = len(m.Name)
				}
				if len(m.Version) > widths[1] {
					widths[1] = len(m.Version)
				}
			}
			sep := util.BuildSeparator(widths)
			fmt.Fprint(kraken.Out(), sep)
			fmt.Fprint(kraken.Out(), util.BuildHeaderRow(headers, widths))
			fmt.Fprint(kraken.Out(), sep)
			for _, m := range manifests {
				fmt.Fprint(kraken.Out(), util.BuildRow([]string{m.Name, m.Version}, widths))
				fmt.Fprint(kraken.Out(), sep)
			}
			return nil
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.Flags().StringVarP(&flagFormat, "format", "f", "txt", "Output format: txt|json")

	// plugins:add
	installCmd := &cobra.Command{
		Use:   "plugins:add <path|url>",
		Short: "Add a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]

			if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
				kraken.Logger().Info("Downloading...")
				data, err := util.FetchURL(src)
				if err != nil {
					return fmt.Errorf("download plugin: %w", err)
				}
				manifest, err := installFromZip(data, store.PluginsDir())
				if err != nil {
					return err
				}
				kraken.Logger().Info("Plugin %q (%s) added.", manifest.Name, manifest.Version)
				return nil
			}

			abs, err := filepath.Abs(src)
			if err != nil {
				return err
			}

			if strings.HasSuffix(strings.ToLower(abs), ".zip") {
				data, err := os.ReadFile(abs)
				if err != nil {
					return err
				}
				manifest, err := installFromZip(data, store.PluginsDir())
				if err != nil {
					return err
				}
				kraken.Logger().Info("Plugin %q (%s) added.", manifest.Name, manifest.Version)
				return nil
			}

			info, err := os.Stat(abs)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				kraken.Logger().Fatal("plugin source must be a directory or .zip file")
				os.Exit(2)
			}
			manifest, err := jsplugin.ReadManifest(abs)
			if err != nil {
				kraken.Logger().Fatal("read kraken.json: %s", err)
				os.Exit(2)
			}
			if manifest.Name == "" {
				kraken.Logger().Fatal("kraken.json: missing name")
				os.Exit(2)
			}
			dest := filepath.Join(store.PluginsDir(), manifest.Name)
			if err := copyDir(abs, dest); err != nil {
				return err
			}
			kraken.Logger().Info("Plugin %q (%s) added.", manifest.Name, manifest.Version)
			return nil
		},
	}
	app.ApplyDefaultFlags(installCmd)

	// plugins:remove
	removeCmd := &cobra.Command{
		Use:   "plugins:remove <name>",
		Short: "Remove a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dest := filepath.Join(store.PluginsDir(), name)
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				kraken.Logger().Fatal("plugin %q is not installed", name)
				os.Exit(2)
			}
			if err := os.RemoveAll(dest); err != nil {
				return err
			}
			kraken.Logger().Info("Plugin %q removed.", name)
			return nil
		},
	}
	app.ApplyDefaultFlags(removeCmd)

	kraken.RootCmd().AddCommand(cmd, installCmd, removeCmd)
}

func listPlugins() ([]*jsplugin.Manifest, error) {
	entries, err := os.ReadDir(store.PluginsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var manifests []*jsplugin.Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := jsplugin.ReadManifest(filepath.Join(store.PluginsDir(), e.Name()))
		if err != nil {
			continue
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

// installFromZip reads the manifest from a zip in memory, then extracts the zip
// to destBase/<plugin-name>/ preserving file permissions.
func installFromZip(data []byte, destBase string) (*jsplugin.Manifest, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	manifest, err := readManifestFromZip(r.File)
	if err != nil {
		return nil, err
	}
	if manifest.Name == "" {
		return nil, fmt.Errorf("kraken.json: missing name")
	}
	dest := filepath.Join(destBase, manifest.Name)
	if err := util.ExtractZip(data, dest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// readManifestFromZip finds kraken.json at the root or one level deep inside a zip.
func readManifestFromZip(files []*zip.File) (*jsplugin.Manifest, error) {
	for _, f := range files {
		parts := strings.Split(strings.TrimSuffix(filepath.ToSlash(f.Name), "/"), "/")
		if len(parts) > 2 || parts[len(parts)-1] != "kraken.json" || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return jsplugin.ParseManifest(data)
	}
	return nil, fmt.Errorf("kraken.json not found in zip")
}

func copyDir(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
