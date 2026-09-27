package restart

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
)

func doInit(kraken *app.Kraken) {
	var flagTimeout int

	cmd := &cobra.Command{
		Use:   "restart [[service.]app ...]",
		Short: "Restart container(s)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var timeout *time.Duration
			if flagTimeout > 0 {
				duration := time.Duration(flagTimeout) * time.Second
				timeout = &duration
			}
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				if err := util.StopContainers(kraken, appName, serviceFilter, timeout); err != nil {
					return err
				}
				return util.StartContainers(kraken, appName, serviceFilter)
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs
	cmd.Flags().IntVar(&flagTimeout, "timeout", 0, "Seconds to wait before killing (0 = daemon default)")

	kraken.RootCmd().AddCommand(cmd)
}
