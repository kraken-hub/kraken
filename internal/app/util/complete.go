package util

import (
	"github.com/spf13/cobra"

	"github.com/kraken-hub/kraken/internal/store"
)

// CompleteApps returns registered app names for shell completion.
func CompleteApps(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	apps, err := store.ListApps()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(apps))
	for _, a := range apps {
		names = append(names, a.Key)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// CompleteRefs returns service.app completions for all registered apps.
// Also includes bare app names for commands that accept [service.]app.
func CompleteRefs(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	apps, err := store.ListApps()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var results []string
	for _, a := range apps {
		results = append(results, a.Key)
		services, err := LoadServices(a)
		if err != nil {
			continue
		}
		for _, svc := range services {
			results = append(results, svc.Name+"."+a.Key)
		}
	}
	return results, cobra.ShellCompDirectiveNoFileComp
}
