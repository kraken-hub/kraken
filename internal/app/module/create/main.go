package create

import (
	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
)

func doInit(kraken *app.Kraken) {
	var (
		flagProfiles []string
	)

	cmd := &cobra.Command{
		Use:   "create [[service.]app ...]",
		Short: "Create application container(s) without starting them",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := util.CreateOptions{Profiles: util.SplitList(flagProfiles...)}
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				return util.CreateContainers(kraken, appName, serviceFilter, opts)
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs

	cmd.Flags().StringArrayVar(&flagProfiles, "profile", nil, "Enable services of a profile, \"*\" for all, comma-separated (default: app profile)")

	kraken.RootCmd().AddCommand(cmd)
}
