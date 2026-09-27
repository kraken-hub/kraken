package logs

import (
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/runtime"
)

func doInit(kraken *app.Kraken) {
	var (
		flagSince     string
		flagTail      string
		flagFollow    bool
		flagTimestamp bool
	)

	cmd := &cobra.Command{
		Use:   "logs <service.[app]>",
		Short: "Show container logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, serviceName, err := util.ParseServiceRef(args)
			if err != nil {
				kraken.Logger().Fatal("%s", err)
				os.Exit(2)
			}

			rt := kraken.Runtime()
			ctx := kraken.Context()

			containerName := util.ContainerName(appName, serviceName)
			info, err := rt.ContainerInspect(ctx, containerName)
			if err != nil {
				kraken.Logger().Fatal("container %q not found", containerName)
				os.Exit(2)
			}

			tail := flagTail
			if tail == "" {
				tail = "all"
			}

			rc, err := rt.ContainerLogs(ctx, info.ID, runtime.LogsOptions{
				Follow:     flagFollow,
				Since:      flagSince,
				Tail:       tail,
				Timestamps: flagTimestamp,
				Stdout:     true,
				Stderr:     true,
			})
			if err != nil {
				return fmt.Errorf("logs %s: %w", containerName, err)
			}
			defer rc.Close()
			out := kraken.Out()
			if info.Tty {
				_, err = io.Copy(out, rc)
			} else {
				_, err = stdcopy.StdCopy(out, out, rc)
			}
			return err
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteRefs
	cmd.Flags().StringVar(&flagSince, "since", "", "Show logs since timestamp or duration (e.g. 42m)")
	cmd.Flags().StringVar(&flagTail, "tail", "all", "Number of lines to show from end")
	cmd.Flags().BoolVarP(&flagFollow, "follow", "f", false, "Follow log output")
	cmd.Flags().BoolVar(&flagTimestamp, "timestamps", false, "Show timestamps")

	kraken.RootCmd().AddCommand(cmd)
}
