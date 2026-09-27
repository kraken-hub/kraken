package list

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/store"
)

type appRow struct {
	Name   string `json:"name"`
	Env    string `json:"env"`
	Status string `json:"status"`
	Path   string `json:"path"`
}

func doInit(kraken *app.Kraken) {
	var flagFormat string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered applications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apps, err := store.ListApps()
			if err != nil {
				return err
			}

			rt := kraken.Runtime()
			ctx := kraken.Context()

			rows := make([]appRow, 0, len(apps))
			for _, appEntry := range apps {
				services, _ := util.LoadServices(appEntry)

				statusParts := make([]string, 0, len(services))
				for _, svc := range services {
					containerName := util.ContainerName(appEntry.Key, svc.Name)
					info, err := rt.ContainerInspect(ctx, containerName)
					if err != nil {
						statusParts = append(statusParts, "~")
					} else if info.State == "running" {
						statusParts = append(statusParts, "+")
					} else {
						statusParts = append(statusParts, "-")
					}
				}

				rows = append(rows, appRow{
					Name:   appEntry.Key,
					Env:    appEntry.Value.Env,
					Status: strings.Join(statusParts, "|"),
					Path:   appEntry.Value.Path,
				})
			}

			if flagFormat == "json" {
				return util.EncodeJSON(kraken.Out(), rows)
			}

			printBoxTable(kraken.Out(), rows)
			return nil
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.Flags().StringVarP(&flagFormat, "format", "f", "txt", "Output format: txt|json")

	kraken.RootCmd().AddCommand(cmd)
}

func printBoxTable(out io.Writer, rows []appRow) {
	headers := []string{"Application name", "Env", "Status", "Path"}
	widths := []int{len(headers[0]), len(headers[1]), len(headers[2]), len(headers[3])}

	for _, row := range rows {
		if len(row.Name) > widths[0] {
			widths[0] = len(row.Name)
		}
		if len(row.Env) > widths[1] {
			widths[1] = len(row.Env)
		}
		if len(row.Status) > widths[2] {
			widths[2] = len(row.Status)
		}
		if len(row.Path) > widths[3] {
			widths[3] = len(row.Path)
		}
	}

	sep := util.BuildSeparator(widths)
	fmt.Fprint(out, sep)
	fmt.Fprint(out, util.BuildHeaderRow(headers, widths))
	fmt.Fprint(out, sep)
	for _, row := range rows {
		cols := []string{row.Name, row.Env, row.Status, row.Path}
		fmt.Fprint(out, util.BuildRow(cols, widths))
		fmt.Fprint(out, sep)
	}
}
