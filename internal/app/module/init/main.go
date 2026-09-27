package appinit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
)

func doInit(kraken *app.Kraken) {
	var (
		flagEnv  string
		flagFile string
		flagPath string
	)

	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Register an application (defaults to current directory name)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			absPath := cwd
			if flagPath != "" {
				absPath, err = filepath.Abs(flagPath)
				if err != nil {
					return fmt.Errorf("resolve path: %w", err)
				}
			}

			name := filepath.Base(absPath)
			if len(args) >= 1 {
				name = args[0]
			}

			appEntry, err := util.InitApp(name, absPath, flagFile, flagEnv)
			if err != nil {
				return err
			}

			kraken.Logger().Info("Application %q initialised (path: %s, env: %s, file: %s)", name, appEntry.Value.Path, appEntry.Value.Env, appEntry.Value.Config.File)
			return nil
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.Flags().StringVarP(&flagEnv, "env", "e", "", "Initial environment(s), comma-separated (env: KRAKEN_ENV, default: dev)")
	cmd.Flags().StringVarP(&flagFile, "file", "f", "", "Config file to use (default: kraken.yaml)")
	cmd.Flags().StringVarP(&flagPath, "path", "p", "", "Application directory (default: current directory)")

	kraken.RootCmd().AddCommand(cmd)
}
