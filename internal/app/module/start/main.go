package start

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/store"
)

func doInit(kraken *app.Kraken) {
	var (
		flagProfiles []string
	)

	cmd := &cobra.Command{
		Use:   "start [[service.]app ...]",
		Short: "Create and start application container(s)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			profiles := util.SplitList(flagProfiles...)
			if cmd.Flags().Changed("init") {
				appName, err := util.ResolveUniqueApp(args)
				if err != nil {
					kraken.Logger().Fatal("--init: %s", err)
					os.Exit(2)
				}
				initVal, _ := cmd.Flags().GetString("init")
				if err := maybeInit(kraken, appName, initVal, profiles); err != nil {
					return err
				}
			}
			opts := util.CreateOptions{Start: true, Profiles: profiles}
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				return util.CreateContainers(kraken, appName, serviceFilter, opts)
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs

	cmd.Flags().String("init", "", "Initialize application if not exists (format: [config]~[env]; use \"\" for defaults)")
	cmd.Flags().StringArrayVar(&flagProfiles, "profile", nil, "Enable services of a profile, \"*\" for all, comma-separated (default: app profile)")

	kraken.RootCmd().AddCommand(cmd)
}

// maybeInit initialises the app if it is not already registered.
// initVal format: "[configPath]~[env]" — both parts optional; empty string means all defaults.
// profiles are stored as the app's default profile only when it gets registered here.
func maybeInit(kraken *app.Kraken, appName, initVal string, profiles []string) error {
	exists, err := store.AppExists(appName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	var configFile, env string
	if initVal != "" {
		parts := strings.SplitN(initVal, "~", 2)
		configFile = parts[0]
		if len(parts) == 2 {
			env = parts[1]
		}
	}

	appEntry, err := util.InitApp(appName, "", configFile, env, strings.Join(profiles, ","))
	if err != nil {
		return err
	}
	kraken.Logger().Info("Application %q initialised (path: %s, env: %s, profile: %s)", appName, appEntry.Value.Path, appEntry.Value.Env, appEntry.Value.Profile)
	return nil
}
