package start

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
)

func doInit(kraken *app.Kraken) {
	cmd := &cobra.Command{
		Use:   "start [[service.]app ...]",
		Short: "Create and start application container(s)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("init") {
				appName, err := util.ResolveUniqueApp(args)
				if err != nil {
					kraken.Logger().Fatal("--init: %s", err)
					os.Exit(2)
				}
				initVal, _ := cmd.Flags().GetString("init")
				if err := maybeInit(kraken, appName, initVal); err != nil {
					return err
				}
			}
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				return util.CreateContainers(kraken, appName, serviceFilter, true)
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs

	cmd.Flags().String("init", "", "Initialize application if not exists (format: [config]~[env]; use \"\" for defaults)")

	kraken.RootCmd().AddCommand(cmd)
}

// maybeInit initialises the app if it is not already registered.
// initVal format: "[configPath]~[env]" — both parts optional; empty string means all defaults.
func maybeInit(kraken *app.Kraken, appName, initVal string) error {
	var configFile, env string
	if initVal != "" {
		parts := strings.SplitN(initVal, "~", 2)
		configFile = parts[0]
		if len(parts) == 2 {
			env = parts[1]
		}
	}

	appEntry, err := util.InitApp(appName, "", configFile, env)
	if err != nil {
		return err
	}
	kraken.Logger().Info("Application %q initialised (path: %s, env: %s)", appName, appEntry.Value.Path, appEntry.Value.Env)
	return nil
}
