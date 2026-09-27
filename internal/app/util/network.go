package util

import (
	"fmt"

	"github.com/kraken-hub/kraken/internal/app"
)

// RemoveNetwork removes the per-app network if it exists.
func RemoveNetwork(kraken *app.Kraken, appName string) {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	name := AppNetwork(appName)
	exists, id, err := rt.NetworkExists(ctx, name)
	if err != nil || !exists {
		return
	}
	log.Info("Removing network `%s`.", name)
	_ = rt.NetworkRemove(ctx, id)
}

// EnsureNetworks creates the global kraken_net and per-app kraken_net.{appName} networks
// if they do not already exist. Returns (globalNetworkID, appNetworkID, error).
func EnsureNetworks(kraken *app.Kraken, appName string) (string, string, error) {
	rt := kraken.Runtime()
	ctx := kraken.Context()

	ensureOne := func(name string, labels map[string]string) (string, error) {
		exists, id, err := rt.NetworkExists(ctx, name)
		if err != nil {
			return "", err
		}
		if exists {
			return id, nil
		}
		id, err = rt.NetworkCreate(ctx, name, labels)
		if err != nil {
			return "", fmt.Errorf("create network %q: %w", name, err)
		}
		return id, nil
	}

	globalID, err := ensureOne(GlobalNetwork, map[string]string{})
	if err != nil {
		return "", "", err
	}
	appID, err := ensureOne(AppNetwork(appName), map[string]string{LabelApp: appName})
	if err != nil {
		return "", "", err
	}
	return globalID, appID, nil
}
