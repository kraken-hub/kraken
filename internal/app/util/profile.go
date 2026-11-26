package util

import (
	"slices"
	"strings"

	"github.com/kraken-hub/kraken/pkg/yaml"
)

// ServiceProfiles returns the profiles a service belongs to (its `profiles:`
// list; a single string is accepted too). No profiles means always enabled.
func ServiceProfiles(ctx *yaml.Ctx, node yaml.Node) []string {
	profilesNode := findMappingKey(ctx, node, "profiles")
	if profilesNode == nil {
		return nil
	}
	if items := yaml.SeqItems(ctx, profilesNode); items != nil {
		var result []string
		for _, item := range items {
			if p := yaml.ScalarText(yaml.Resolve(ctx, item)); p != "" {
				result = append(result, p)
			}
		}
		return result
	}
	if p := yaml.ScalarText(yaml.Resolve(ctx, profilesNode)); p != "" {
		return []string{p}
	}
	return nil
}

// ProfileActive reports whether a service with the given profiles is enabled
// when the active profiles are active: always for a service without profiles,
// otherwise when one of its profiles is active or "*" is.
func ProfileActive(serviceProfiles, active []string) bool {
	if len(serviceProfiles) == 0 || slices.Contains(active, "*") {
		return true
	}
	for _, p := range serviceProfiles {
		if slices.Contains(active, p) {
			return true
		}
	}
	return false
}

// SplitList splits comma-separated values (e.g. repeated --profile flags or a
// stored profile string) into a flat list, dropping empty entries.
func SplitList(values ...string) []string {
	var result []string
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				result = append(result, p)
			}
		}
	}
	return result
}
