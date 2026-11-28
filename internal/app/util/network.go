package util

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
	"github.com/kraken-hub/kraken/pkg/yaml"
)

// UsesKrakenNetworks reports whether a container with the given network mode is
// attached to the kraken networks. Only the default ("" / "bridge") mode is: host,
// none and container: modes share or disable the network stack and can't join
// other networks.
func UsesKrakenNetworks(mode string) bool {
	return mode == "" || mode == "bridge"
}

// PrepareNetworkMode validates cfg.NetworkMode and resolves `service:<name>` to the
// container of service <name> in appName. In host and none mode published ports
// are dropped with a warning. In container mode, settings owned by the shared
// network stack (ports, hostname, dns, extra_hosts) are an error.
func PrepareNetworkMode(kraken *app.Kraken, appName string, cfg *runtime.ContainerOptions) error {
	droppedPorts, err := prepareNetworkMode(appName, cfg)
	if err != nil {
		return err
	}
	if droppedPorts {
		kraken.Logger().Warn("Ports of `%s` are ignored with network_mode %q.", cfg.Name, cfg.NetworkMode)
	}
	return nil
}

func prepareNetworkMode(appName string, cfg *runtime.ContainerOptions) (droppedPorts bool, err error) {
	mode := cfg.NetworkMode
	if UsesKrakenNetworks(mode) {
		return false, nil
	}
	if len(cfg.Networks) > 0 {
		return false, fmt.Errorf("network_mode %q of `%s` can't be combined with networks", mode, cfg.Name)
	}
	switch {
	case mode == "host" || mode == "none":
		droppedPorts = len(cfg.Ports) > 0
		cfg.Ports = nil
		return droppedPorts, nil
	}
	name, ok := ResolveContainerRef(appName, mode)
	if !ok {
		return false, fmt.Errorf("network_mode %q of `%s`: unsupported (use bridge, host, none, container:<name> or service:<name>)", mode, cfg.Name)
	}
	if _, target, _ := strings.Cut(mode, ":"); target == "" {
		return false, fmt.Errorf("network_mode %q of `%s`: missing name", mode, cfg.Name)
	}
	var conflicts []string
	if len(cfg.Ports) > 0 {
		conflicts = append(conflicts, "ports")
	}
	if cfg.Hostname != "" {
		conflicts = append(conflicts, "hostname")
	}
	if len(cfg.DNS) > 0 {
		conflicts = append(conflicts, "dns")
	}
	if len(cfg.DNSSearch) > 0 {
		conflicts = append(conflicts, "dns_search")
	}
	if len(cfg.DNSOptions) > 0 {
		conflicts = append(conflicts, "dns_opt")
	}
	if len(cfg.ExtraHosts) > 0 {
		conflicts = append(conflicts, "extra_hosts")
	}
	if len(conflicts) > 0 {
		return false, fmt.Errorf("network_mode %q of `%s` can't be combined with %s (the network stack is shared)",
			mode, cfg.Name, strings.Join(conflicts, ", "))
	}
	cfg.NetworkMode = "container:" + name
	return false, nil
}

// RemoveNetworks removes the networks owned by an app: kraken_net.{appName} and
// the networks declared in its top-level networks: section (both labelled with
// kraken.app). External networks are never labelled and are left alone.
func RemoveNetworks(kraken *app.Kraken, appName string) {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()

	nets, err := rt.NetworkList(ctx, AppLabels(appName))
	if err != nil {
		log.Warn("Warning listing networks: %v", err)
		nets = nil
	}
	// The app network is also looked up by name, in case it predates labelling.
	if exists, id, err := rt.NetworkExists(ctx, AppNetwork(appName)); err == nil && exists {
		nets = append(nets, runtime.NetworkInfo{ID: id, Name: AppNetwork(appName)})
	}

	seen := map[string]bool{}
	for _, n := range nets {
		if seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		log.Info("Removing network `%s`.", n.Name)
		if err := rt.NetworkRemove(ctx, n.ID); err != nil {
			log.Warn("Warning removing network `%s`: %v", n.Name, err)
		}
	}
}

// EnsureAppNetworks ensures the networks declared in the top-level networks:
// section exist. App networks are named kraken_net.{appName}.{name} and created
// when missing; external networks must already exist and keep their name.
// It returns a map of YAML network name → actual network name.
func EnsureAppNetworks(kraken *app.Kraken, appName string, appEntry *store.App) (map[string]string, error) {
	networks, nctx, err := LoadNetworksSection(appEntry)
	if err != nil {
		return nil, err
	}

	rt := kraken.Runtime()
	ctx := kraken.Context()
	result := map[string]string{}

	for netName, netNode := range networks {
		if netName == "default" || netName == GlobalNetwork {
			return nil, fmt.Errorf("network name %q is reserved", netName)
		}
		resolved := yaml.Resolve(nctx, netNode)
		var settings yaml.Node
		if resolved != nil && resolved.Type() != yaml.NullType {
			settings = resolved
		}

		if settings != nil && isExternalNetwork(nctx, settings) {
			actualName := netName
			if nameNode := findMappingKey(nctx, settings, "name"); nameNode != nil {
				actualName = yaml.ScalarText(nameNode)
			} else if nameNode := findMappingKey(nctx, findMappingKey(nctx, settings, "external"), "name"); nameNode != nil {
				actualName = yaml.ScalarText(nameNode)
			}
			exists, _, err := rt.NetworkExists(ctx, actualName)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, fmt.Errorf("external network %q does not exist", actualName)
			}
			result[netName] = actualName
			continue
		}

		opts := runtime.NetworkCreateOptions{
			Name:   AppNamedNetwork(appName, netName),
			Labels: map[string]string{LabelApp: appName, LabelNetwork: netName},
		}
		if settings != nil {
			if driverNode := findMappingKey(nctx, settings, "driver"); driverNode != nil {
				opts.Driver = yaml.ScalarText(driverNode)
			}
			if optsNode := findMappingKey(nctx, settings, "driver_opts"); optsNode != nil {
				opts.DriverOpts = mappingNodeToMap(nctx, optsNode)
			}
			if internalNode := findMappingKey(nctx, settings, "internal"); internalNode != nil {
				opts.Internal = yaml.ScalarText(internalNode) == "true"
			}
			if labelsNode := findMappingKey(nctx, settings, "labels"); labelsNode != nil {
				for k, v := range labelsNodeToMap(nctx, labelsNode) {
					if _, reserved := opts.Labels[k]; !reserved {
						opts.Labels[k] = v
					}
				}
			}
		}
		exists, _, err := rt.NetworkExists(ctx, opts.Name)
		if err != nil {
			return nil, err
		}
		if !exists {
			if _, err := rt.NetworkCreate(ctx, opts); err != nil {
				return nil, fmt.Errorf("create network %q: %w", opts.Name, err)
			}
		}
		result[netName] = opts.Name
	}
	return result, nil
}

// isExternalNetwork reports whether a network definition has `external: true`
// (or the `external: {name: ...}` mapping form).
func isExternalNetwork(ctx *yaml.Ctx, settings yaml.Node) bool {
	externalNode := findMappingKey(ctx, settings, "external")
	if externalNode == nil {
		return false
	}
	switch v := yaml.Resolve(ctx, externalNode).(type) {
	case *yaml.MappingNode:
		return true
	default:
		return yaml.ScalarText(v) == "true"
	}
}

// ValidateExtraNetworks checks the network names given on the command line: each
// must be the full name of an existing network (as listed by `docker network ls`).
// Network mode values (bridge, host, none, container:, service:) and the networks
// every container joins by default (kraken_net, kraken_net.{appName}) are rejected.
func ValidateExtraNetworks(kraken *app.Kraken, appName string, names []string) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	for _, name := range names {
		if _, isRef := ResolveContainerRef(appName, name); isRef || name == "bridge" || name == "host" || name == "none" {
			return fmt.Errorf("%q is a network mode, use --network-mode %s", name, name)
		}
		if name == GlobalNetwork || name == AppNetwork(appName) {
			return fmt.Errorf("network %q is joined by default", name)
		}
		exists, _, err := rt.NetworkExists(ctx, name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("network %q not found", name)
		}
	}
	return nil
}

// AppendExtraNetworks appends networks given by full name (see
// ValidateExtraNetworks) to cfg.Networks, which must already be resolved by
// ResolveNetworkAttachments. Networks the container already joins are skipped.
func AppendExtraNetworks(cfg *runtime.ContainerOptions, names []string) {
	seen := map[string]bool{}
	for _, att := range cfg.Networks {
		seen[att.Name] = true
	}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		cfg.Networks = append(cfg.Networks, runtime.NetworkAttachment{Name: name})
	}
}

// ApplyRunIPv4Address clears the service's ipv4_address values, which belong to
// the service container (a one-off container next to it would clash), and gives
// the container the static IP ip on kraken_net instead, when ip is not empty.
func ApplyRunIPv4Address(cfg *runtime.ContainerOptions, ip string) {
	hasGlobal := false
	for i := range cfg.Networks {
		cfg.Networks[i].IPv4Address = ""
		if cfg.Networks[i].Name == GlobalNetwork {
			cfg.Networks[i].IPv4Address = ip
			hasGlobal = true
		}
	}
	if ip != "" && !hasGlobal {
		cfg.Networks = append(cfg.Networks, runtime.NetworkAttachment{Name: GlobalNetwork, IPv4Address: ip})
	}
}

// ResolveNetworkAttachments replaces the YAML network names in cfg.Networks with
// actual network names from appNetworks (see EnsureAppNetworks), dropping
// duplicates. A network not declared in appNetworks is an error, except
// kraken_net, which every container joins and which needs no declaration.
// A malformed ipv4_address is an error too, so it is caught before the container
// is created.
func ResolveNetworkAttachments(cfg *runtime.ContainerOptions, appNetworks map[string]string) error {
	if len(cfg.Networks) == 0 {
		return nil
	}
	resolved := make([]runtime.NetworkAttachment, 0, len(cfg.Networks))
	seen := map[string]bool{}
	for _, att := range cfg.Networks {
		actual, ok := appNetworks[att.Name]
		if att.Name == GlobalNetwork {
			actual, ok = GlobalNetwork, true
		}
		if !ok {
			return fmt.Errorf("network %q of `%s` is not declared in the top-level networks section", att.Name, cfg.Name)
		}
		if att.IPv4Address != "" && !IsIPv4(att.IPv4Address) {
			return fmt.Errorf("network %q of `%s`: ipv4_address %q is not an IPv4 address", att.Name, cfg.Name, att.IPv4Address)
		}
		if seen[actual] {
			continue
		}
		seen[actual] = true
		resolved = append(resolved, runtime.NetworkAttachment{Name: actual, Aliases: att.Aliases, IPv4Address: att.IPv4Address})
	}
	cfg.Networks = resolved
	return nil
}

// IsIPv4 reports whether s is an IPv4 address.
func IsIPv4(s string) bool {
	addr, err := netip.ParseAddr(s)
	return err == nil && addr.Is4()
}

// ConnectNetworks attaches a created container to the global and per-app kraken
// networks and to the extra networks in cfg.Networks (already resolved by
// ResolveNetworkAttachments). A kraken_net entry in cfg.Networks adds its aliases
// and static IP to the global network connection. Containers whose network mode
// doesn't use the kraken networks are left alone. When a connection fails (e.g. a
// static IP already in use), the container is removed, so a later start doesn't
// pick up a container that misses its networks.
func ConnectNetworks(kraken *app.Kraken, appName, serviceName, containerID string, cfg *runtime.ContainerOptions, globalNetworkID, appNetworkID string) error {
	if !UsesKrakenNetworks(cfg.NetworkMode) {
		return nil
	}
	err := connectNetworks(kraken, appName, serviceName, containerID, cfg, globalNetworkID, appNetworkID)
	if err != nil {
		if rmErr := kraken.Runtime().ContainerRemove(kraken.Context(), containerID, true, true); rmErr != nil {
			kraken.Logger().Warn("Warning removing container `%s`: %v", cfg.Name, rmErr)
		}
	}
	return err
}

func connectNetworks(kraken *app.Kraken, appName, serviceName, containerID string, cfg *runtime.ContainerOptions, globalNetworkID, appNetworkID string) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	global := runtime.NetworkConnectOptions{Aliases: []string{cfg.Name}}
	for _, att := range cfg.Networks {
		if att.Name == GlobalNetwork {
			global.Aliases = append(global.Aliases, att.Aliases...)
			global.IPv4Address = att.IPv4Address
		}
	}
	if err := rt.NetworkConnect(ctx, globalNetworkID, containerID, global); err != nil {
		return fmt.Errorf("network connect %q to %s: %w", cfg.Name, GlobalNetwork, err)
	}
	if err := rt.NetworkConnect(ctx, appNetworkID, containerID, runtime.NetworkConnectOptions{Aliases: []string{serviceName}}); err != nil {
		return fmt.Errorf("network connect %q to %s: %w", cfg.Name, AppNetwork(appName), err)
	}
	for _, att := range cfg.Networks {
		if att.Name == GlobalNetwork {
			continue
		}
		if err := rt.NetworkConnect(ctx, att.Name, containerID, runtime.NetworkConnectOptions{Aliases: att.Aliases, IPv4Address: att.IPv4Address}); err != nil {
			return fmt.Errorf("network connect %q to %s: %w", cfg.Name, att.Name, err)
		}
	}
	return nil
}

// EnsureNetworks creates the global kraken_net and per-app kraken_net.{appName} networks
// if they do not already exist. kraken_net is created with the subnet and dynamic IP
// range from KRAKEN_NET_SUBNET and KRAKEN_NET_IP_RANGE (an existing network is left
// as is). Returns (globalNetworkID, appNetworkID, error).
func EnsureNetworks(kraken *app.Kraken, appName string) (string, string, error) {
	rt := kraken.Runtime()
	ctx := kraken.Context()

	ensureOne := func(opts runtime.NetworkCreateOptions) (string, error) {
		exists, id, err := rt.NetworkExists(ctx, opts.Name)
		if err != nil {
			return "", err
		}
		if exists {
			return id, nil
		}
		if opts.IPRange != "" && opts.Subnet == "" {
			return "", fmt.Errorf("create network %q: KRAKEN_NET_IP_RANGE needs KRAKEN_NET_SUBNET", opts.Name)
		}
		id, err = rt.NetworkCreate(ctx, opts)
		if err != nil {
			return "", fmt.Errorf("create network %q: %w", opts.Name, err)
		}
		return id, nil
	}

	globalID, err := ensureOne(runtime.NetworkCreateOptions{
		Name:    GlobalNetwork,
		Labels:  map[string]string{},
		Subnet:  os.Getenv("KRAKEN_NET_SUBNET"),
		IPRange: os.Getenv("KRAKEN_NET_IP_RANGE"),
	})
	if err != nil {
		return "", "", err
	}
	appID, err := ensureOne(runtime.NetworkCreateOptions{Name: AppNetwork(appName), Labels: map[string]string{LabelApp: appName}})
	if err != nil {
		return "", "", err
	}
	return globalID, appID, nil
}
