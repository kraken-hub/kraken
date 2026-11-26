package rebuild

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
		Use:   "rebuild [[service.]app ...]",
		Short: "Remove and recreate container(s)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			profiles := util.SplitList(flagProfiles...)
			return util.ForEachRef(args, func(appName, serviceFilter string) error {
				// Optional services that have a container now are recreated when no
				// profile is active (neither --profile nor the app's stored profile).
				existing, err := util.ExistingServices(kraken, appName)
				if err != nil {
					return err
				}
				opts := util.CreateOptions{Start: true, Profiles: profiles, Existing: existing}
				if err := util.RemoveContainers(kraken, appName, serviceFilter, true); err != nil {
					return err
				}
				return util.CreateContainers(kraken, appName, serviceFilter, opts)
			})
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs

	cmd.Flags().StringArrayVar(&flagProfiles, "profile", nil, "Enable services of a profile, \"*\" for all, comma-separated (default: app profile)")

	kraken.RootCmd().AddCommand(cmd)
}
