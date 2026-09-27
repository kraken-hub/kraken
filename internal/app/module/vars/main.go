package vars

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/app/util"
	"github.com/kraken-hub/kraken/internal/store"
)

func doInit(kraken *app.Kraken) {
	var flagFormat string

	// vars — list all
	cmd := &cobra.Command{
		Use:   "vars",
		Short: "List shared variables",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := store.Vars()
			if err != nil {
				return err
			}
			if flagFormat == "json" {
				return util.EncodeJSON(kraken.Out(), m)
			}
			headers := []string{"Name", "Value"}
			widths := []int{len(headers[0]), len(headers[1])}
			for k, v := range m {
				if len(k) > widths[0] {
					widths[0] = len(k)
				}
				if len(v) > widths[1] {
					widths[1] = len(v)
				}
			}
			sep := util.BuildSeparator(widths)
			fmt.Fprint(kraken.Out(), sep)
			fmt.Fprint(kraken.Out(), util.BuildHeaderRow(headers, widths))
			fmt.Fprint(kraken.Out(), sep)
			for k, v := range m {
				fmt.Fprint(kraken.Out(), util.BuildRow([]string{k, v}, widths))
				fmt.Fprint(kraken.Out(), sep)
			}
			return nil
		},
	}
	app.ApplyDefaultFlags(cmd)
	cmd.Flags().StringVarP(&flagFormat, "format", "f", "txt", "Output format: txt|json")

	// vars:set
	setCmd := &cobra.Command{
		Use:   "vars:set <name> [value]",
		Short: "Set a shared variable",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			value := ""
			if len(args) == 2 {
				value = args[1]
			}
			if err := store.SetVar(args[0], value); err != nil {
				return err
			}
			kraken.Logger().Info("Set %s=%s", args[0], value)
			return nil
		},
	}
	app.ApplyDefaultFlags(setCmd)

	// vars:unset
	unsetCmd := &cobra.Command{
		Use:   "vars:unset <name>",
		Short: "Unset a shared variable",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.UnsetVar(args[0]); err != nil {
				return err
			}
			kraken.Logger().Info("Unset %s", args[0])
			return nil
		},
	}
	app.ApplyDefaultFlags(unsetCmd)

	kraken.RootCmd().AddCommand(cmd, setCmd, unsetCmd)
}

