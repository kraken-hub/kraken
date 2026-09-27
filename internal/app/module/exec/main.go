package exec

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/runtime"
)

func doInit(kraken *app.Kraken) {
	var (
		flagEnv            []string
		flagNonInteractive bool
		flagWorkDir        string
	)

	cmd := &cobra.Command{
		Use:   "exec <service.[app]> [-- cmd...]",
		Short: "Execute a command in a service container",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, serviceName, err := util.ParseServiceRef(args)
			if err != nil {
				kraken.Logger().Fatal("%s", err)
				os.Exit(2)
			}

			var dashArgs []string
			if i := cmd.ArgsLenAtDash(); i >= 0 {
				dashArgs = args[i:]
			}

			cmdArgs := []string{"/bin/sh"}
			if len(dashArgs) > 0 {
				cmdArgs = dashArgs
			} else if v := os.Getenv("KRAKEN_SHELL"); v != "" {
				cmdArgs = strings.Fields(v)
			}

			tty := !flagNonInteractive && util.IsTerminal()

			execCfg := runtime.ExecOptions{
				Cmd:         cmdArgs,
				Tty:         tty,
				AttachStdin: tty,
				WorkDir:     flagWorkDir,
			}

			for _, e := range flagEnv {
				parts := strings.SplitN(e, "=", 2)
				if len(parts) == 2 {
					execCfg.Env = append(execCfg.Env, e)
				}
			}

			containerName := util.ContainerName(appName, serviceName)
			rt := kraken.Runtime()
			ctx := kraken.Context()

			var in = kraken.In()
			if !tty {
				in = nil
			}

			exitCode, err := rt.ContainerExec(ctx, containerName, execCfg, in, kraken.Out(), kraken.Err())
			if err != nil {
				return err
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
			return nil
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return util.CompleteRefs(cmd, args, toComplete)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cmd.Flags().StringArrayVarP(&flagEnv, "env", "e", nil, "Set environment variables")
	cmd.Flags().BoolVar(&flagNonInteractive, "non-interactive", false, "Non-interactive mode")
	cmd.Flags().StringVarP(&flagWorkDir, "workdir", "w", "", "Working directory inside the container")

	kraken.RootCmd().AddCommand(cmd)
}

