package util

import (
	"crypto/md5"
	"fmt"
	"os"

	"github.com/kraken-hub/kraken/internal/app"
	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
	"github.com/kraken-hub/kraken/pkg/yaml"
)

// AppVolumeName returns the managed volume name for a top-level named volume.
// Format: kraken_vol.{appName}.{volName}
func AppVolumeName(appName, volName string) string {
	return "kraken_vol." + appName + "." + volName
}

// ServiceVolumeName returns the managed volume name for a service container-path volume.
// Format: kraken_vol.{appName}.{serviceName}.{md5(path)}
func ServiceVolumeName(appName, serviceName, path string) string {
	sum := md5.Sum([]byte(path))
	return fmt.Sprintf("kraken_vol.%s.%s.%x", appName, serviceName, sum)
}

// EnsureVolumes ensures all named volumes from the top-level volumes: section
// exist. It returns a map of YAML volume name → actual volume name for volumes
// that are runtime-managed (external or driver/driver_opts). Simple null volumes
// are auto-created by the runtime via ServiceVolumeName at container-create time.
func EnsureVolumes(kraken *app.Kraken, appName string, appEntry *store.App) (map[string]string, error) {
	volumes, vctx, err := LoadVolumesSection(appEntry)
	if err != nil {
		return nil, err
	}

	rt := kraken.Runtime()
	ctx := kraken.Context()
	managedVolumes := map[string]string{}

	for volName, volNode := range volumes {
		resolved := yaml.Resolve(vctx, volNode)
		if resolved == nil || resolved.Type() == yaml.NullType {
			// Simple volume — auto-created by runtime via ServiceVolumeName
			continue
		}

		// External volume — verify it exists, use as-is
		externalNode := findMappingKey(vctx, resolved, "external")
		if externalNode != nil {
			actualName := volName
			if externalMap, ok := yaml.Resolve(vctx, externalNode).(*yaml.MappingNode); ok {
				if nameNode := findMappingKey(vctx, externalMap, "name"); nameNode != nil {
					actualName = yaml.ScalarText(nameNode)
				}
			}
			exists, err := rt.VolumeExists(ctx, actualName)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, fmt.Errorf("external volume %q does not exist", actualName)
			}
			managedVolumes[volName] = actualName
			continue
		}

		// Volume with driver/driver_opts — managed named volume
		hasDriverOpts := findMappingKey(vctx, resolved, "driver_opts") != nil
		hasDriver := findMappingKey(vctx, resolved, "driver") != nil
		if hasDriverOpts || hasDriver {
			volActualName := AppVolumeName(appName, volName)
			opts := runtime.VolumeCreateOptions{
				Name:   volActualName,
				Labels: map[string]string{LabelApp: appName, "kraken.target": volName},
			}
			if driverNode := findMappingKey(vctx, resolved, "driver"); driverNode != nil {
				opts.Driver = yaml.ScalarText(driverNode)
			}
			if optsNode := findMappingKey(vctx, resolved, "driver_opts"); optsNode != nil {
				opts.DriverOpts = mappingNodeToMap(vctx, optsNode)
			}
			if labelsNode := findMappingKey(vctx, resolved, "labels"); labelsNode != nil {
				for k, v := range labelsNodeToMap(vctx, labelsNode) {
					opts.Labels[k] = v
				}
			}
			exists, err := rt.VolumeExists(ctx, volActualName)
			if err != nil {
				return nil, err
			}
			if !exists {
				if err := rt.VolumeCreate(ctx, opts); err != nil {
					return nil, fmt.Errorf("create volume %q: %w", volActualName, err)
				}
			}
			managedVolumes[volName] = volActualName
		}
		// Simple named volume without driver/driver_opts → auto-created via ServiceVolumeName
	}
	return managedVolumes, nil
}

// EnsureServiceVolumes pre-creates any service-path managed volumes in cfg.Volumes
// that do not yet exist, and also covers paths declared via VOLUME in the image
// so Docker does not fall back to anonymous volumes for them.
// Must be called after EnsureImage so the image is available for inspection.
func EnsureServiceVolumes(kraken *app.Kraken, appName, serviceName string, cfg *runtime.ContainerOptions) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()

	// Collect container paths already covered by explicit mounts.
	covered := map[string]bool{}
	for _, vm := range cfg.Volumes {
		covered[vm.Target] = true
	}

	// Add managed volumes for any image VOLUME paths not already covered.
	if cfg.Image != "" {
		imagePaths, err := rt.ImageVolumes(ctx, cfg.Image)
		if err != nil {
			return err
		}
		for _, p := range imagePaths {
			if covered[p] {
				continue
			}
			cfg.Volumes = append(cfg.Volumes, runtime.VolumeMount{
				Source: ServiceVolumeName(appName, serviceName, p),
				Target: p,
			})
			covered[p] = true
		}
	}

	// Pre-create all managed volumes; ensure bind-mount host directories exist.
	for _, vm := range cfg.Volumes {
		if vm.Source == "" {
			continue // anonymous volume
		}
		if vm.Source[0] == '/' {
			// Bind mount — ensure the host path exists; an existing file is fine.
			if _, err := os.Stat(vm.Source); err == nil {
				continue
			}
			if err := os.MkdirAll(vm.Source, 0o755); err != nil {
				return fmt.Errorf("create bind-mount directory %q: %w", vm.Source, err)
			}
			continue
		}
		exists, err := rt.VolumeExists(ctx, vm.Source)
		if err != nil {
			return err
		}
		if !exists {
			if err := rt.VolumeCreate(ctx, runtime.VolumeCreateOptions{
				Name: vm.Source,
				Labels: map[string]string{
					LabelApp:        appName,
					LabelService:    serviceName,
					"kraken.target": vm.Target,
				},
			}); err != nil {
				return fmt.Errorf("create volume %q: %w", vm.Source, err)
			}
		}
	}
	return nil
}

// PurgeApp removes all managed volumes for an app (prefix: kraken_vol.{appName}.).
func PurgeApp(kraken *app.Kraken, appName string, _ *store.App) error {
	return removeVolumesByPrefix(kraken, "kraken_vol."+appName+".")
}

// PurgeService removes managed volumes for a single service (prefix: kraken_vol.{appName}.{serviceName}.).
// App-level named volumes (kraken_vol.{appName}.{volName}) are not removed.
func PurgeService(kraken *app.Kraken, appName, serviceName string) error {
	return removeVolumesByPrefix(kraken, "kraken_vol."+appName+"."+serviceName+".")
}

func removeVolumesByPrefix(kraken *app.Kraken, prefix string) error {
	rt := kraken.Runtime()
	ctx := kraken.Context()
	log := kraken.Logger()

	names, err := rt.VolumeList(ctx, prefix)
	if err != nil {
		return fmt.Errorf("list volumes: %w", err)
	}
	for _, name := range names {
		if err := rt.VolumeRemove(ctx, name, false); err != nil {
			log.Warn("Warning removing volume `%s`: %v", name, err)
		} else {
			log.Info("Volume `%s` removed.", name)
		}
	}
	return nil
}
