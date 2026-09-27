package remove

import (
	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/store"
)

func doInit(kraken *app.Kraken) {
	var flagPurge bool

	cmd := &cobra.Command{
		Use:   "remove [[service.]app ...]",
		Short: "Remove container(s)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				if err := util.RemoveContainers(kraken, appName, serviceFilter, true); err != nil {
					return err
				}

				if flagPurge {
					log := kraken.Logger()

					if err := util.RemoveImages(kraken, appName, serviceFilter); err != nil {
						log.Warn("Warning removing images: %v", err)
					}

					if serviceFilter == "" {
						appEntry, err := store.LoadApp(appName)
						if err == nil {
							if err := util.PurgeApp(kraken, appName, appEntry); err != nil {
								log.Warn("Warning: %v", err)
							}
						}
						util.RemoveNetwork(kraken, appName)
						if err := store.DeleteApp(appName); err != nil {
							return err
						}
						log.Warn("Application `%s` removed.", appName)
					} else {
						if err := util.PurgeService(kraken, appName, serviceFilter); err != nil {
							log.Warn("Warning: %v", err)
						}
						log.Warn("Service `%s` in app `%s` removed.", serviceFilter, appName)
					}
				}
				return nil
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs
	cmd.Flags().BoolVar(&flagPurge, "purge", false, "Also remove image(s) and persistent data")

	kraken.RootCmd().AddCommand(cmd)
}
