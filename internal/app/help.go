package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/gookit/color.v1"
)

func initColors() {
	if os.Getenv("NO_COLOR") != "" {
		color.Enable = false
	}
}

func helpLabel(s string) string { return color.Yellow.Sprint(s) }
func helpTerm(s string) string  { return color.Green.Sprint(s) }
func helpDesc(s string) string  { return color.Cyan.Sprint(s) }

// usageLine builds the usage line for a command.
// full=true includes the full command path (for the Usage: section).
// full=false uses only the command's own Use string (for the Commands: list).
func usageLine(cmd *cobra.Command, full bool) string {
	hasFlags := false
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			hasFlags = true
		}
	})

	var base string
	if full {
		base = cmd.CommandPath()
	} else {
		base = cmd.Use
	}

	use := base
	if hasFlags {
		use += " [options]"
	}
	if full {
		// append positional args or [command] hint from Use
		name := cmd.Name()
		rest := strings.TrimPrefix(cmd.Use, name)
		if rest != "" {
			use += rest
		} else if cmd.HasAvailableSubCommands() {
			use += " [command]"
		}
	}
	return use
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func setHelpFunc(cmd *cobra.Command) {
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()

		// Usage
		fmt.Fprintf(out, "\n  %s\n\n", helpLabel("Usage:"))
		useLine := usageLine(cmd, true)
		fmt.Fprintf(out, "    %s\n", useLine)
		fmt.Fprintln(out)

		// Long or short description
		description := cmd.Long
		if description == "" {
			description = cmd.Short
		}
		if description != "" {
			fmt.Fprintf(out, "  %s\n\n", helpLabel("Description:"))
			for _, line := range strings.Split(strings.TrimRight(description, "\n\r "), "\n") {
				if line == "" {
					fmt.Fprintln(out)
				} else {
					fmt.Fprintf(out, "    %s\n", line)
				}
			}
			fmt.Fprintln(out)
		}

		// Collect flags
		type flagEntry struct {
			label string
			usage string
		}
		var flagEntries []flagEntry
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			short := "    "
			if f.Shorthand != "" {
				short = "-" + f.Shorthand + ", "
			}
			label := short + "--" + f.Name
			if f.Value.Type() != "bool" {
				label += " <" + f.Name + ">"
			}
			flagEntries = append(flagEntries, flagEntry{label: label, usage: f.Usage})
		})

		// Collect visible subcommands
		var visible []*cobra.Command
		for _, c := range cmd.Commands() {
			if !c.Hidden {
				visible = append(visible, c)
			}
		}

		// Root shows only command names; subcommands show full use line
		isRoot := !cmd.HasParent()
		cmdLabel := func(c *cobra.Command) string {
			if isRoot {
				return c.Name()
			}
			return usageLine(c, false)
		}

		// Global column width — align descriptions across both sections
		colWidth := 0
		for _, e := range flagEntries {
			if len(e.label) > colWidth {
				colWidth = len(e.label)
			}
		}
		for _, c := range visible {
			if l := cmdLabel(c); len(l) > colWidth {
				colWidth = len(l)
			}
		}

		if len(flagEntries) > 0 {
			fmt.Fprintf(out, "  %s\n\n", helpLabel("Options:"))
			for _, e := range flagEntries {
				fmt.Fprintf(out, "    %s  %s\n",
					helpTerm(padRight(e.label, colWidth)),
					helpDesc(e.usage),
				)
			}
			fmt.Fprintln(out)
		}

		if len(visible) > 0 {
			fmt.Fprintf(out, "  %s\n\n", helpLabel("Commands:"))
			for _, c := range visible {
				fmt.Fprintf(out, "    %s  %s\n",
					helpTerm(padRight(cmdLabel(c), colWidth)),
					helpDesc(c.Short),
				)
			}
			fmt.Fprintln(out)
		}
	})
}
