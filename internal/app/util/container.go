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

// CreateContainers creates (and optionally starts) containers for an app.
// serviceFilter limits the operation to a single service when non-empty.
func CreateContainers(kraken *app.Kraken, appName, serviceFilter string, startAfter bool) error {
	appEntry, err := store.LoadApp(appName)
	if err != nil {
		return fmt.Errorf("app %q not found (run 'init' first)", appName)
	}

	services, err := LoadServices(appEntry)
	if err != nil {
		return err
	}

	dockerVolumes, err := EnsureVolumes(kraken, appName, appEntry)
	if err != nil {
		return err
	}

	globalNetworkID, appNetworkID, err := EnsureNetworks(kraken, appName)
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
				if startAfter {
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

			instanceCfg.DNS = ResolveDNS(kraken, instanceCfg.DNS)
			instanceCfg.ExtraHosts = ResolveExtraHosts(kraken, instanceCfg.ExtraHosts)

			log.Info("Creating `%s` container.", instanceCfg.Name)
			containerID, err := rt.ContainerCreate(ctx, &instanceCfg)
			if err != nil {
				return fmt.Errorf("create %q: %w", instanceCfg.Name, err)
			}

			if err := rt.NetworkConnect(ctx, globalNetworkID, containerID, instanceCfg.Name); err != nil {
				return fmt.Errorf("network connect %q to %s: %w", instanceCfg.Name, GlobalNetwork, err)
			}
			if err := rt.NetworkConnect(ctx, appNetworkID, containerID, serviceName); err != nil {
				return fmt.Errorf("network connect %q to %s: %w", instanceCfg.Name, AppNetwork(appName), err)
			}

			log.Info("Container `%s` created.", instanceCfg.Name)

			if startAfter {
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

// ResolveDNS resolves "container:name" entries in a dns: list to their IP address.
// Static IP entries are passed through unchanged.
func ResolveDNS(kraken *app.Kraken, servers []string) []string {
	if len(servers) == 0 {
		return servers
	}
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	result := make([]string, 0, len(servers))
	for _, s := range servers {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) == 2 && parts[0] == "container" {
			ip, err := rt.ContainerNetworkIP(ctx, parts[1], GlobalNetwork)
			if err != nil || ip == "" {
				log.Warn("Warning: could not resolve dns container IP for %q: %v", parts[1], err)
				continue
			}
			result = append(result, ip)
		} else {
			result = append(result, s)
		}
	}
	return result
}

// ResolveExtraHosts resolves "hostname:container:name" entries to "hostname:ip"
// by looking up the container's IP on the global kraken network.
// Static "hostname:ip" entries are passed through unchanged.
func ResolveExtraHosts(kraken *app.Kraken, hosts []string) []string {
	if len(hosts) == 0 {
		return hosts
	}
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()
	result := make([]string, 0, len(hosts))
	for _, h := range hosts {
		parts := strings.SplitN(h, ":", 3)
		if len(parts) == 3 && parts[1] == "container" {
			ip, err := rt.ContainerNetworkIP(ctx, parts[2], GlobalNetwork)
			if err != nil || ip == "" {
				log.Warn("Warning: could not resolve extra_host container IP for %q: %v", parts[2], err)
				continue
			}
			result = append(result, parts[0]+":"+ip)
		} else {
			result = append(result, h)
		}
	}
	return result
}
