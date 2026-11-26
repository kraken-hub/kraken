package util

import (
	"fmt"
	"strings"
	"time"

	"github.com/containerd/errdefs"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
)

// CreateOptions controls CreateContainers.
type CreateOptions struct {
	// Start starts the containers after creating them (existing ones are started too).
	Start bool
	// Profiles are the active profiles; "*" activates all of them. When empty, the
	// app's stored profile (init --profile, config:context) is used.
	Profiles []string
	// Existing names services whose containers existed before (rebuild). Only used
	// when no profiles are active.
	Existing map[string]bool
}

// CreateContainers creates (and optionally starts) containers for an app.
// serviceFilter limits the operation to a single service when non-empty; a named
// service is always included. Without a filter, services without profiles are
// always included; a service with profiles is included when one of its profiles
// is active — or, when no profiles are active at all (neither given nor stored),
// when its container exists (or is listed in opts.Existing).
func CreateContainers(kraken *app.Kraken, appName, serviceFilter string, opts CreateOptions) error {
	appEntry, err := store.LoadApp(appName)
	if err != nil {
		return fmt.Errorf("app %q not found (run 'init' first)", appName)
	}

	services, err := LoadServices(appEntry)
	if err != nil {
		return err
	}

	if len(opts.Profiles) == 0 {
		opts.Profiles = SplitList(appEntry.Value.Profile)
	}

	dockerVolumes, err := EnsureVolumes(kraken, appName, appEntry)
	if err != nil {
		return err
	}

	globalNetworkID, appNetworkID, err := EnsureNetworks(kraken, appName)
	if err != nil {
		return err
	}

	appNetworks, err := EnsureAppNetworks(kraken, appName, appEntry)
	if err != nil {
		return err
	}

	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()

	for _, svc := range services {
		serviceName, serviceNode := svc.Name, svc.Node
		if serviceFilter != "" && serviceName != serviceFilter {
			continue
		}
		if serviceFilter == "" && !ProfileActive(ServiceProfiles(svc.Ctx, serviceNode), opts.Profiles) {
			if len(opts.Profiles) > 0 {
				continue
			}
			if !opts.Existing[serviceName] {
				exists, err := rt.ContainerExists(ctx, ContainerName(appName, serviceName))
				if err != nil {
					return err
				}
				if !exists {
					continue
				}
			}
		}

		containerCfg := NodeToContainerOptions(appName, serviceName, serviceNode, svc.Ctx, appEntry.Value.Path, dockerVolumes)
		ApplyVars(containerCfg)

		{
			instanceCfg := *containerCfg

			if err := EnsureImage(kraken, &instanceCfg); err != nil {
				return err
			}

			if err := EnsureServiceVolumes(kraken, appName, serviceName, &instanceCfg); err != nil {
				return err
			}

			containerExists, err := rt.ContainerExists(ctx, instanceCfg.Name)
			if err != nil {
				return err
			}
			if containerExists {
				info, err := rt.ContainerInspect(ctx, instanceCfg.Name)
				if err != nil {
					return err
				}
				if info.Labels[LabelApp] != appName || info.Labels[LabelService] != serviceName {
					return fmt.Errorf("container `%s` already exists but is not managed by kraken (labels %s=%s, %s=%s missing), remove it manually",
						instanceCfg.Name, LabelApp, appName, LabelService, serviceName)
				}
				log.Warn("Container `%s` already exists, skipping create", instanceCfg.Name)
				if opts.Start {
					log.Info("Starting `%s` container.", instanceCfg.Name)
					if err := rt.ContainerStart(ctx, instanceCfg.Name); err != nil {
						log.Warn("Warning starting `%s`: %v", instanceCfg.Name, err)
					}
				}
				continue
			}

			payload := app.NewPayload("container:create")
			payload.Data = &instanceCfg
			if err := kraken.Emit(payload.ID(), payload); err != nil {
				return err
			}

			if err := PrepareNetworkMode(kraken, appName, &instanceCfg); err != nil {
				return err
			}
			if err := ResolveNetworkAttachments(&instanceCfg, appNetworks); err != nil {
				return err
			}

			instanceCfg.DNS = ResolveDNS(kraken, appName, instanceCfg.DNS)
			instanceCfg.ExtraHosts = ResolveExtraHosts(kraken, appName, instanceCfg.ExtraHosts)

			log.Info("Creating `%s` container.", instanceCfg.Name)
			containerID, err := rt.ContainerCreate(ctx, &instanceCfg)
			if err != nil {
				return fmt.Errorf("create %q: %w", instanceCfg.Name, err)
			}

			if err := ConnectNetworks(kraken, appName, serviceName, containerID, &instanceCfg, globalNetworkID, appNetworkID); err != nil {
				return err
			}

			log.Info("Container `%s` created.", instanceCfg.Name)

			if opts.Start {
				log.Info("Starting `%s` container.", instanceCfg.Name)
				if err := rt.ContainerStart(ctx, containerID); err != nil {
					return fmt.Errorf("start %q: %w", instanceCfg.Name, err)
				}
				if waitDuration := WaitDuration(svc.Ctx, serviceNode); waitDuration > 0 {
					if err := WaitWithDirective(kraken, containerID, instanceCfg.Name, waitDuration); err != nil {
						return err
					}
				}
				log.Info("Container `%s` started.", instanceCfg.Name)
			}
		}
	}
	return nil
}

// EnsureImage builds or pulls the image for a container config if needed.
func EnsureImage(kraken *app.Kraken, cfg *runtime.ContainerOptions) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()

	if cfg.Build != nil {
		b := *cfg.Build // copy so we can set derived fields without mutating the original

		imageExists, err := rt.ImageExists(ctx, cfg.Image)
		if err != nil {
			return err
		}
		if b.Policy == "always" || !imageExists {
			b.PullParent = cfg.PullPolicy == "always"
			b.Platform = cfg.Platform
			b.Tags = append([]string{cfg.Image}, cfg.Build.Tags...)
			log.Info("Building image `%s`.", cfg.Image)
			fmt.Fprintln(kraken.Out())
			if err := rt.ImageBuild(ctx, &b, kraken.Out()); err != nil {
				return fmt.Errorf("build %q: %w", cfg.Name, err)
			}
			fmt.Fprintln(kraken.Out())
		}
	} else if cfg.Image != "" {
		switch cfg.PullPolicy {
		case "always":
			log.Info("Pulling `%s`.", cfg.Image)
			fmt.Fprintln(kraken.Out())
			if err := rt.ImagePull(ctx, cfg.Image, cfg.Platform, kraken.Out()); err != nil {
				return fmt.Errorf("pull %q: %w", cfg.Image, err)
			}
			fmt.Fprintln(kraken.Out())
		case "never":
			// rely on what is locally available
		default: // missing
			imageExists, err := rt.ImageExists(ctx, cfg.Image)
			if err != nil {
				return err
			}
			if !imageExists {
				log.Info("Pulling `%s`.", cfg.Image)
				fmt.Fprintln(kraken.Out())
				if err := rt.ImagePull(ctx, cfg.Image, cfg.Platform, kraken.Out()); err != nil {
					return fmt.Errorf("pull %q: %w", cfg.Image, err)
				}
				fmt.Fprintln(kraken.Out())
			}
		}
	}
	return nil
}

// StartContainers starts stopped containers for an app/service.
func StartContainers(kraken *app.Kraken, appName, serviceFilter string) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	containers, err := rt.ContainerList(ctx, true, AppLabels(appName))
	if err != nil {
		return err
	}
	for _, container := range containers {
		if serviceFilter != "" && container.Labels[LabelService] != serviceFilter {
			continue
		}
		log.Info("Starting `%s` container.", container.Name)
		if err := rt.ContainerStart(ctx, container.ID); err != nil {
			log.Warn("Warning starting `%s`: %v", container.Name, err)
		}
	}
	return nil
}

// ExistingServices returns the services of an app that currently have a
// container (running or not), by the kraken.service label.
func ExistingServices(kraken *app.Kraken, appName string) (map[string]bool, error) {
	containers, err := kraken.Runtime().ContainerList(kraken.Context(), true, AppLabels(appName))
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, c := range containers {
		if svc := c.Labels[LabelService]; svc != "" {
			result[svc] = true
		}
	}
	return result, nil
}

// StopContainers stops running containers for an app/service.
func StopContainers(kraken *app.Kraken, appName, serviceFilter string, timeout *time.Duration) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	containers, err := rt.ContainerList(ctx, true, AppLabels(appName))
	if err != nil {
		return err
	}
	for _, container := range containers {
		if serviceFilter != "" && container.Labels[LabelService] != serviceFilter {
			continue
		}
		if container.State != "running" {
			continue
		}
		log.Info("Stopping `%s` container.", container.Name)
		if err := rt.ContainerStop(ctx, container.ID, timeout); err != nil {
			log.Warn("Warning stopping `%s`: %v", container.Name, err)
		}
	}
	return nil
}

// RemoveContainers removes containers for an app/service.
func RemoveContainers(kraken *app.Kraken, appName, serviceFilter string, force bool) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	containers, err := rt.ContainerList(ctx, true, AppLabels(appName))
	if err != nil {
		return err
	}
	for _, container := range containers {
		if serviceFilter != "" && container.Labels[LabelService] != serviceFilter {
			continue
		}
		log.Info("Removing `%s` container.", container.Name)
		if err := rt.ContainerRemove(ctx, container.ID, force, true); err != nil {
			log.Warn("Warning removing `%s`: %v", container.Name, err)
		}
	}
	return nil
}

// RemoveImages removes the resolved image for each service of an app (the
// explicit `image:` tag, or the auto-generated build tag when none is set).
// Removal is non-forced, so an image still referenced by a container of
// another app is left alone. Errors are logged, not returned.
func RemoveImages(kraken *app.Kraken, appName, serviceFilter string) error {
	appEntry, err := store.LoadApp(appName)
	if err != nil {
		return err
	}

	services, err := LoadServices(appEntry)
	if err != nil {
		return err
	}

	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()

	for _, svc := range services {
		if serviceFilter != "" && svc.Name != serviceFilter {
			continue
		}
		cfg := NodeToContainerOptions(appName, svc.Name, svc.Node, svc.Ctx, appEntry.Value.Path, nil)
		imageName := cfg.Image
		exists, err := rt.ImageExists(ctx, imageName)
		if err != nil || !exists {
			continue
		}
		if err := rt.ImageRemove(ctx, imageName, false); err != nil {
			if !errdefs.IsConflict(err) {
				log.Warn("Warning removing image `%s`: %v", imageName, err)
			}
		} else {
			log.Warn("Image `%s` removed.", imageName)
		}
	}
	return nil
}

// ResolveContainerRef resolves a container reference: `service:<name>` is the
// container of service <name> in appName, `container:<name>` is the literal
// container name. ok is false when ref is neither form.
func ResolveContainerRef(appName, ref string) (name string, ok bool) {
	kind, target, found := strings.Cut(ref, ":")
	if !found {
		return "", false
	}
	switch kind {
	case "service":
		return ContainerName(appName, target), true
	case "container":
		return target, true
	}
	return "", false
}

// ResolveDNS resolves "service:name" / "container:name" entries in a dns: list
// to their IP address. Static IP entries are passed through unchanged.
func ResolveDNS(kraken *app.Kraken, appName string, servers []string) []string {
	if len(servers) == 0 {
		return servers
	}
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	result := make([]string, 0, len(servers))
	for _, s := range servers {
		name, ok := ResolveContainerRef(appName, s)
		if !ok {
			result = append(result, s)
			continue
		}
		ip, err := rt.ContainerNetworkIP(ctx, name, GlobalNetwork)
		if err != nil || ip == "" {
			log.Warn("Warning: could not resolve dns container IP for %q: %v", name, err)
			continue
		}
		result = append(result, ip)
	}
	return result
}

// ResolveExtraHosts resolves "hostname:service:name" / "hostname:container:name"
// entries to "hostname:ip" by looking up the container's IP on the global kraken
// network. Static "hostname:ip" entries are passed through unchanged.
func ResolveExtraHosts(kraken *app.Kraken, appName string, hosts []string) []string {
	if len(hosts) == 0 {
		return hosts
	}
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	result := make([]string, 0, len(hosts))
	for _, h := range hosts {
		hostname, ref, _ := strings.Cut(h, ":")
		name, ok := ResolveContainerRef(appName, ref)
		if !ok {
			result = append(result, h)
			continue
		}
		ip, err := rt.ContainerNetworkIP(ctx, name, GlobalNetwork)
		if err != nil || ip == "" {
			log.Warn("Warning: could not resolve extra_host container IP for %q: %v", name, err)
			continue
		}
		result = append(result, hostname+":"+ip)
	}
	return result
}
