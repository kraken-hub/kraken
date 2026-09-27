package run

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/store"
	"github.com/kraken-hub/kraken/pkg/yaml"
)

func doInit(kraken *app.Kraken) {
	var (
		flagEnv            []string
		flagDetach         bool
		flagEntrypoint     string
		flagNonInteractive bool
		flagWorkDir        string
	)

	cmd := &cobra.Command{
		Use:   "run <service.[app]> [-- cmd...]",
		Short: "Run a command in a new container",
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

			rt := kraken.Runtime()
			ctx := kraken.Context()

			// Load YAML config to get the clean env, image, and volumes definition.
			appEntry, err := store.LoadApp(appName)
			if err != nil {
				kraken.Logger().Fatal("%s", err)
				os.Exit(2)
			}
			services, err := util.LoadServices(appEntry)
			if err != nil {
				return err
			}
			var serviceNode yaml.Node
			var serviceCtx *yaml.Ctx
			for _, svc := range services {
				if svc.Name == serviceName {
					serviceNode = svc.Node
					serviceCtx = svc.Ctx
					break
				}
			}
			if serviceNode == nil {
				kraken.Logger().Fatal("service %q not found in config", serviceName)
				os.Exit(2)
			}

			dockerVolumes, err := util.EnsureVolumes(kraken, appName, appEntry)
			if err != nil {
				return err
			}
			containerCfg := util.NodeToContainerOptions(appName, serviceName, serviceNode, serviceCtx, appEntry.Value.Path, dockerVolumes)
			util.ApplyVars(containerCfg)

			imageExists, err := rt.ImageExists(ctx, containerCfg.Image)
			if err != nil {
				return err
			}
			if !imageExists {
				if err := util.EnsureImage(kraken, containerCfg); err != nil {
					return err
				}
			}

			if err := util.EnsureServiceVolumes(kraken, appName, serviceName, containerCfg); err != nil {
				return err
			}

			containerCfg.DNS = util.ResolveDNS(kraken, containerCfg.DNS)
			containerCfg.ExtraHosts = util.ResolveExtraHosts(kraken, containerCfg.ExtraHosts)
			containerCfg.Labels = map[string]string{}

			for _, e := range flagEnv {
				if k, v, ok := strings.Cut(e, "="); ok {
					if containerCfg.Env == nil {
						containerCfg.Env = map[string]string{}
					}
					containerCfg.Env[k] = v
				}
			}

			tty := !flagNonInteractive && util.IsTerminal()

			if flagDetach {
				containerCfg.Name = fmt.Sprintf("%s.%s_detach_%d", serviceName, appName, os.Getpid())
			} else {
				containerCfg.Name = fmt.Sprintf("%s.%s_pty_%d", serviceName, appName, os.Getpid())
			}

			containerCfg.Restart = ""
			containerCfg.AutoRemove = true
			containerCfg.Ports = nil
			containerCfg.Tty = tty
			containerCfg.OpenStdin = tty
			containerCfg.Cmd = cmdArgs

			if flagWorkDir != "" {
				containerCfg.WorkDir = flagWorkDir
			}

			if cmd.Flags().Changed("entrypoint") {
				if flagEntrypoint == "" {
					containerCfg.Entrypoint = []string{}
				} else {
					containerCfg.Entrypoint = []string{flagEntrypoint}
				}
			}

			payload := app.NewPayload("container:create")
			payload.Data = containerCfg
			if err := kraken.Emit(payload.ID(), payload); err != nil {
				return err
			}

			globalNetworkID, appNetworkID, err := util.EnsureNetworks(kraken, appName)
			if err != nil {
				return err
			}

			containerID, err := rt.ContainerCreate(ctx, containerCfg)
			if err != nil {
				return fmt.Errorf("create: %w", err)
			}

			if err := rt.NetworkConnect(ctx, globalNetworkID, containerID, containerCfg.Name); err != nil {
				return fmt.Errorf("network: %w", err)
			}
			if err := rt.NetworkConnect(ctx, appNetworkID, containerID, serviceName); err != nil {
				return fmt.Errorf("network: %w", err)
			}

			if flagDetach {
				if err := rt.ContainerStart(ctx, containerID); err != nil {
					return err
				}
				fmt.Fprintln(kraken.Out(), containerID)
				return nil
			}

			var in = kraken.In()
			if !tty {
				in = nil
			}

			exitCode, err := rt.ContainerRun(ctx, containerID, tty, in, kraken.Out(), kraken.Err())
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
	cmd.Flags().BoolVarP(&flagDetach, "detach", "d", false, "Run in background")
	cmd.Flags().StringVar(&flagEntrypoint, "entrypoint", "", "Override container entrypoint (\"\" to clear)")
	cmd.Flags().BoolVar(&flagNonInteractive, "non-interactive", false, "Non-interactive mode")
	cmd.Flags().StringVarP(&flagWorkDir, "workdir", "w", "", "Working directory inside the container")

	kraken.RootCmd().AddCommand(cmd)
}
