package status

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/store"
)

func doInit(kraken *app.Kraken) {
	var flagFormat string

	cmd := &cobra.Command{
		Use:   "status [app]",
		Short: "Show application status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, err := util.ResolveAppName(args)
			if err != nil {
				return err
			}

			appEntry, err := store.LoadApp(appName)
			if err != nil {
				return err
			}

			services, err := util.LoadServices(appEntry)
			if err != nil {
				return err
			}

			rt := kraken.Runtime()
			ctx := kraken.Context()

			type containerRow struct {
				Name   string `json:"name"`
				Status string `json:"status"`
				IP     string `json:"ip"`
			}

			rows := make([]containerRow, 0, len(services))
			for _, svc := range services {
				serviceName := svc.Name
				containerName := util.ContainerName(appName, serviceName)
				info, err := rt.ContainerInspect(ctx, containerName)
				if err != nil {
					rows = append(rows, containerRow{Name: containerName, Status: "NOT CREATED", IP: "-"})
					continue
				}
				status := "OFF"
				ip := "-"
				if info.State == "running" {
					status = "ON"
					if info.IP != "" {
						ip = info.IP
					}
				}
				rows = append(rows, containerRow{Name: containerName, Status: status, IP: ip})
			}

			if flagFormat == "json" {
				return util.EncodeJSON(kraken.Out(), rows)
			}

			headers := []string{"Container name", "Status", "IP"}
			widths := []int{len(headers[0]), len(headers[1]), len(headers[2])}
			for _, row := range rows {
				if len(row.Name) > widths[0] {
					widths[0] = len(row.Name)
				}
				if len(row.Status) > widths[1] {
					widths[1] = len(row.Status)
				}
				if len(row.IP) > widths[2] {
					widths[2] = len(row.IP)
				}
			}

			sep := util.BuildSeparator(widths)
			fmt.Fprint(kraken.Out(), sep)
			fmt.Fprint(kraken.Out(), util.BuildHeaderRow(headers, widths))
			fmt.Fprint(kraken.Out(), sep)
			for _, row := range rows {
				fmt.Fprint(kraken.Out(), util.BuildRow([]string{row.Name, row.Status, row.IP}, widths))
				fmt.Fprint(kraken.Out(), sep)
			}
			return nil
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.ValidArgsFunction = util.CompleteApps
	cmd.Flags().StringVarP(&flagFormat, "format", "f", "txt", "Output format: txt|json")

	kraken.RootCmd().AddCommand(cmd)
}
