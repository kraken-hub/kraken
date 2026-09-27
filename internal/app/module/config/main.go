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
		flagEnvAdd    []string
		flagEnvRemove []string
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
			if len(flagEnvAdd) == 0 && len(flagEnvRemove) == 0 {
				fmt.Fprintf(kraken.Out(), "env: %s\nsource: %s\n", appEntry.Value.Env, appEntry.Value.Config.File)
				return nil
			}
			envs := splitEnv(appEntry.Value.Env)
			for _, e := range flagEnvAdd {
				if !contains(envs, e) {
					envs = append(envs, e)
				}
			}
			kept := envs[:0]
			for _, e := range envs {
				if !contains(flagEnvRemove, e) {
					kept = append(kept, e)
				}
			}
			appEntry.Value.Env = strings.Join(kept, ",")
			if err := store.SaveApp(appEntry); err != nil {
				return err
			}
			kraken.Logger().Info("Env updated to %q.", appEntry.Value.Env)
			return nil
		},
	}
	app.ApplyDefaultFlags(contextCmd)
	contextCmd.ValidArgsFunction = util.CompleteApps
	contextCmd.Flags().StringArrayVar(&flagEnvAdd, "add-env", nil, "Add environment(s)")
	contextCmd.Flags().StringArrayVar(&flagEnvRemove, "remove-env", nil, "Remove environment(s)")

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

func splitEnv(env string) []string {
	var result []string
	for _, e := range strings.Split(env, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			result = append(result, e)
		}
	}
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
