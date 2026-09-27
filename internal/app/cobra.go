package app

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(use, version string, fn func(c *cobra.Command, args []string) error) *cobra.Command {
	var rootCmd *cobra.Command

	rootCmd = &cobra.Command{
		Use:                   use,
		Version:               version,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		RunE: func(c *cobra.Command, args []string) error {
			return fn(c, args)
		},
	}

	rootCmd.Flags().BoolP("version", "V", false, "Show version")
	rootCmd.SetVersionTemplate("{{printf \"%s\" .Version}}\n")
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	setHelpFunc(rootCmd)

	return rootCmd
}

func ApplyDefaultFlags(rootCmd *cobra.Command) {
	rootCmd.Flags().SortFlags = false
	rootCmd.Flags().BoolP("help", "h", false, "Show help")
}
