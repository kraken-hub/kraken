package rebuild

import (
	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
)

func doInit(kraken *app.Kraken) {
	cmd := &cobra.Command{
		Use:   "rebuild [[service.]app ...]",
		Short: "Remove and recreate container(s)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				if err := util.RemoveContainers(kraken, appName, serviceFilter, true); err != nil {
					return err
				}
				return util.CreateContainers(kraken, appName, serviceFilter, true)
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs

	kraken.RootCmd().AddCommand(cmd)
}
