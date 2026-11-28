package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/config"
	"github.com/kraken-hub/kraken/internal/store"
	"github.com/kraken-hub/kraken/pkg/diff"
)

const defaultConfigFile = "kraken.yaml"

func doInit(kraken *app.Kraken) {
	// config [app] — show saved config snapshot (read-only)
	cmd := &cobra.Command{
		Use:   "config [app]",
		Short: "Show saved application configuration",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, err := util.ResolveAppName(args)
			if err != nil {
				return err
			}
			appEntry, err := store.LoadApp(appName)
			if err != nil {
				kraken.Logger().Fatal("%s", err)
				os.Exit(2)
			}
			if appEntry.Value.Config.Data == "" {
				kraken.Logger().Warn("No config snapshot saved. Run 'config:update %s' first.", appName)
				return nil
			}
			fmt.Fprint(kraken.Out(), appEntry.Value.Config.Data)
			return nil
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteApps

	// config:context [app] — show or update application context
	var (
		flagEnvAdd        []string
		flagProfileAdd    []string
		flagEnvRemove     []string
		flagProfileRemove []string
	)
	contextCmd := &cobra.Command{
		Use:   "config:context [app]",
		Short: "Show or update application context",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, err := util.ResolveAppName(args)
			if err != nil {
				return err
			}
			appEntry, err := store.LoadApp(appName)
			if err != nil {
				kraken.Logger().Fatal("%s", err)
				os.Exit(2)
			}
			envChanged := len(flagEnvAdd) > 0 || len(flagEnvRemove) > 0
			profileChanged := len(flagProfileAdd) > 0 || len(flagProfileRemove) > 0
			if !envChanged && !profileChanged {
				fmt.Fprint(kraken.Out(), formatContext(appEntry.Value.Env, appEntry.Value.Profile, appEntry.Value.Config.File))
				return nil
			}
			if envChanged {
				appEntry.Value.Env = updateList(appEntry.Value.Env, flagEnvAdd, flagEnvRemove)
			}
			if profileChanged {
				appEntry.Value.Profile = updateList(appEntry.Value.Profile, flagProfileAdd, flagProfileRemove)
			}
			if err := store.SaveApp(appEntry); err != nil {
				return err
			}
			if envChanged {
				kraken.Logger().Info("Env updated to %q.", appEntry.Value.Env)
			}
			if profileChanged {
				kraken.Logger().Info("Profile updated to %q.", appEntry.Value.Profile)
			}
			return nil
		},
	}
	app.ApplyDefaultFlags(contextCmd)
	contextCmd.ValidArgsFunction = util.CompleteApps
	contextCmd.Flags().StringArrayVar(&flagEnvAdd, "add-env", nil, "Add environment(s)")
	contextCmd.Flags().StringArrayVar(&flagProfileAdd, "add-profile", nil, "Add default profile(s)")
	contextCmd.Flags().StringArrayVar(&flagEnvRemove, "remove-env", nil, "Remove environment(s)")
	contextCmd.Flags().StringArrayVar(&flagProfileRemove, "remove-profile", nil, "Remove default profile(s)")

	// config:diff [app] — diff stored snapshot vs local kraken.yaml
	diffCmd := &cobra.Command{
		Use:   "config:diff [app]",
		Short: "Diff config snapshot against current config",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, err := util.ResolveAppName(args)
			if err != nil {
				return err
			}
			appEntry, err := store.LoadApp(appName)
			if err != nil {
				kraken.Logger().Fatal("%s", err)
				os.Exit(2)
			}
			fresh, err := processConfig(appEntry)
			if err != nil {
				return err
			}
			saved := appEntry.Value.Config.Data
			if saved == fresh {
				kraken.Logger().Info("No differences.")
				return nil
			}
			fmt.Fprintln(kraken.Out(), diff.Diff(saved, fresh))
			return nil
		},
	}
	app.ApplyDefaultFlags(diffCmd)
	diffCmd.ValidArgsFunction = util.CompleteApps

	// config:update [app] — process local kraken.yaml and save snapshot
	updateCmd := &cobra.Command{
		Use:   "config:update [app]",
		Short: "Update the config snapshot",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, err := util.ResolveAppName(args)
			if err != nil {
				return err
			}
			appEntry, err := store.LoadApp(appName)
			if err != nil {
				kraken.Logger().Fatal("%s", err)
				os.Exit(2)
			}
			fresh, err := processConfig(appEntry)
			if err != nil {
				return err
			}
			appEntry.Value.Config.Data = fresh
			if err := store.SaveApp(appEntry); err != nil {
				return err
			}
			kraken.Logger().Info("Config snapshot updated for %q.", appName)
			return nil
		},
	}
	app.ApplyDefaultFlags(updateCmd)
	updateCmd.ValidArgsFunction = util.CompleteApps

	kraken.RootCmd().AddCommand(cmd, contextCmd, diffCmd, updateCmd)
}

// processConfig loads and processes the app's kraken.yaml with the correct env function.
func processConfig(appEntry *store.App) (string, error) {
	cfgFile := appEntry.Value.Config.File
	if cfgFile == "" {
		cfgFile = appEntry.Value.Path + "/" + defaultConfigFile
	}
	return config.LoadYamlFile(cfgFile, util.EnvFunc(appEntry))
}

// updateList adds and removes items in a comma-separated list, keeping order.
// formatContext renders the app context; an empty env or profile is left out.
func formatContext(env, profile, file string) string {
	var b strings.Builder
	if env != "" {
		fmt.Fprintf(&b, "env: %s\n", env)
	}
	if profile != "" {
		fmt.Fprintf(&b, "profile: %s\n", profile)
	}
	fmt.Fprintf(&b, "source: %s\n", file)
	return b.String()
}

func updateList(current string, add, remove []string) string {
	items := util.SplitList(current)
	for _, e := range util.SplitList(add...) {
		if !contains(items, e) {
			items = append(items, e)
		}
	}
	removed := util.SplitList(remove...)
	kept := items[:0]
	for _, e := range items {
		if !contains(removed, e) {
			kept = append(kept, e)
		}
	}
	return strings.Join(kept, ",")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
