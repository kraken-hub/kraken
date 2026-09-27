package create

import (
	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
)

func doInit(kraken *app.Kraken) {
	cmd := &cobra.Command{
		Use:   "create [[service.]app ...]",
		Short: "Create application container(s) without starting them",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				return util.CreateContainers(kraken, appName, serviceFilter, false)
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs

	kraken.RootCmd().AddCommand(cmd)
}
