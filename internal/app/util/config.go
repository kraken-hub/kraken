package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kraken-hub/kraken/internal/runtime"
	"github.com/kraken-hub/kraken/internal/store"
	"github.com/kraken-hub/kraken/pkg/dotenv"
	"github.com/kraken-hub/kraken/pkg/yaml"
)

// ServiceEntry holds a service name and its YAML node in definition order.
// Ctx resolves aliases anywhere in the document Node came from (shared
// across all entries returned by the same LoadServices call).
type ServiceEntry struct {
	Name string
	Node yaml.Node
	Ctx  *yaml.Ctx
}

// LoadServices parses the stored config snapshot into an ordered slice of services.
// Order matches the YAML definition order.
func LoadServices(appEntry *store.App) ([]ServiceEntry, error) {
	data := appEntry.Value.Config.Data
	if data == "" {
		return nil, fmt.Errorf("no config snapshot for %q — run 'config:update' first", appEntry.Key)
	}

	root, err := yaml.Load([]byte(data))
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("empty config")
	}

	ctx := yaml.NewCtx()
	ctx.Index(root)

	_, servicesVal := yaml.FindValue(ctx, root, "services")
	if servicesVal == nil {
		return nil, nil
	}

	pairs := yaml.MapPairs(ctx, servicesVal)
	result := make([]ServiceEntry, 0, len(pairs))
	for _, mv := range pairs {
		result = append(result, ServiceEntry{Name: yaml.ScalarText(mv.Key), Node: mv.Value, Ctx: ctx})
	}
	return result, nil
}

// LoadVolumesSection parses the top-level volumes: section from the config snapshot.
// Returns volumeName -> YAML node (may be nil for volumes with no config), plus the
// Ctx needed to resolve any aliases within those nodes.
func LoadVolumesSection(appEntry *store.App) (map[string]yaml.Node, *yaml.Ctx, error) {
	ctx := yaml.NewCtx()

	data := appEntry.Value.Config.Data
	if data == "" {
		return map[string]yaml.Node{}, ctx, nil
	}

	root, err := yaml.Load([]byte(data))
	if err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	if root == nil {
		return map[string]yaml.Node{}, ctx, nil
	}
	ctx.Index(root)

	_, volumesVal := yaml.FindValue(ctx, root, "volumes")
	if volumesVal == nil {
		return map[string]yaml.Node{}, ctx, nil
	}

	result := map[string]yaml.Node{}
	for _, mv := range yaml.MapPairs(ctx, volumesVal) {
		result[yaml.ScalarText(mv.Key)] = mv.Value
	}
	return result, ctx, nil
}

func findMappingKey(ctx *yaml.Ctx, node yaml.Node, key string) yaml.Node {
	_, v := yaml.FindValue(ctx, node, key)
	return v
}

// NodeToContainerOptions converts a YAML service node into a ContainerOptions.
// managedVolumes maps YAML volume names to their actual runtime volume names for
// volumes declared in the top-level volumes: section with driver/external config.
// All other volume references are resolved as managed volumes via ServiceVolumeName.
func NodeToContainerOptions(appName, serviceName string, node yaml.Node, ctx *yaml.Ctx, appPath string, managedVolumes map[string]string) *runtime.ContainerOptions {
	cfg := &runtime.ContainerOptions{
		Name: ContainerName(appName, serviceName),
		Labels: map[string]string{
			LabelApp:     appName,
			LabelService: serviceName,
		},
	}

	var envFileNode, envNode yaml.Node
	for _, mv := range yaml.MapPairs(ctx, node) {
		valueNode := mv.Value
		switch yaml.ScalarText(mv.Key) {
		case "image":
			cfg.Image = yaml.ScalarText(valueNode)
		case "pull_policy":
			cfg.PullPolicy = yaml.ScalarText(valueNode)
		case "build":
			parseBuild(ctx, cfg, valueNode, appPath, appName, serviceName)
		case "restart":
			parseRestart(ctx, cfg, valueNode)
		case "workdir":
			cfg.WorkDir = yaml.ScalarText(valueNode)
		case "hostname":
			cfg.Hostname = yaml.ScalarText(valueNode)
		case "entrypoint":
			cfg.Entrypoint = parseScript(ctx, valueNode)
		case "cmd":
			cfg.Cmd = parseScript(ctx, valueNode)
		case "env_file":
			envFileNode = valueNode
		case "env":
			envNode = valueNode
		case "volumes":
			cfg.Volumes = resolveVolumes(ctx, valueNode, appPath, appName, serviceName, managedVolumes)
		case "ports":
			cfg.Ports = parsePorts(ctx, valueNode)
		case "labels":
			cfg.UserLabels = labelsNodeToMap(ctx, valueNode)
		case "extra_hosts":
			cfg.ExtraHosts = expandExtraHosts(nodeToStringSlice(ctx, valueNode))
		case "dns":
			cfg.DNS = nodeToStringSlice(ctx, valueNode)
		case "dns_search":
			cfg.DNSSearch = nodeToStringSlice(ctx, valueNode)
		case "dns_opt":
			cfg.DNSOptions = nodeToStringSlice(ctx, valueNode)
		case "platform":
			cfg.Platform = yaml.ScalarText(valueNode)
		case "volumes_from":
			cfg.VolumesFrom = parseVolumesFrom(nodeToStringSlice(ctx, valueNode), appName)
		case "logging":
			cfg.Logging = parseLogging(ctx, valueNode)
		}
	}

	// env_file loaded first, then env overwrites — so explicit env: keys take precedence
	if envFileNode != nil {
		cfg.Env = parseEnvFiles(ctx, envFileNode, appPath)
	}
	if envNode != nil {
		for k, v := range envNodeToMap(ctx, envNode) {
			if cfg.Env == nil {
				cfg.Env = map[string]string{}
			}
			cfg.Env[k] = v
		}
	}

	// Inject identity vars — always present, not overridable from YAML
	if cfg.Env == nil {
		cfg.Env = map[string]string{}
	}
	cfg.Env["KRAKEN_APP"] = appName
	cfg.Env["KRAKEN_SERVICE"] = serviceName

	return cfg
}

// ApplyVars merges global store vars into cfg.Env. YAML-defined values take
// precedence — a var is only injected when the key is not already set.
func ApplyVars(cfg *runtime.ContainerOptions) {
	vars, err := store.Vars()
	if err != nil || len(vars) == 0 {
		return
	}
	for k, v := range vars {
		if _, exists := cfg.Env[k]; !exists {
			if cfg.Env == nil {
				cfg.Env = map[string]string{}
			}
			cfg.Env[k] = v
		}
	}
}

// WaitDuration returns the "wait" duration for a service node, or 0.
func WaitDuration(ctx *yaml.Ctx, node yaml.Node) time.Duration {
	waitNode := findMappingKey(ctx, node, "wait")
	if waitNode == nil {
		return 0
	}
	text := yaml.ScalarText(waitNode)
	if text == "" || text == "false" {
		return 0
	}
	duration, err := time.ParseDuration(text)
	if err != nil {
		return 0
	}
	return duration
}

// parseScript converts a scalar or sequence node into a command slice.
// Multiline scalar strings are wrapped in ["sh", "-c", script].
func parseScript(ctx *yaml.Ctx, node yaml.Node) []string {
	resolved := yaml.Resolve(ctx, node)
	if yaml.IsScalar(resolved) {
		text := yaml.ScalarText(resolved)
		if strings.Contains(text, "\n") {
			return []string{"/bin/sh", "-c", text}
		}
		return strings.Fields(text)
	}
	return nodeToStringSlice(ctx, node)
}

// parseBuild handles string, mapping, and sequence build: values.
func parseBuild(ctx *yaml.Ctx, cfg *runtime.ContainerOptions, node yaml.Node, appPath, appName, serviceName string) {
	b := &runtime.BuildOptions{}
	cfg.Build = b

	switch t := yaml.Resolve(ctx, node).(type) {
	case *yaml.MappingNode:
		contextNode := findMappingKey(ctx, t, "context")
		if contextNode != nil {
			switch c := yaml.Resolve(ctx, contextNode).(type) {
			case *yaml.SequenceNode:
				for _, item := range c.Values {
					if r := yaml.Resolve(ctx, item); yaml.IsScalar(r) && b.Context == "" {
						b.Context = filepath.Join(appPath, yaml.ScalarText(r))
					} else if m, ok := r.(*yaml.MappingNode); ok {
						parseBuildFileItems(b, m)
					}
				}
			default:
				if yaml.IsScalar(c) {
					b.Context = filepath.Join(appPath, yaml.ScalarText(c))
				}
			}
		} else {
			b.Context = appPath
		}

		// file and file_inline are complementary: file sets the path, file_inline sets the content.
		var fileVal string
		if fileNode := findMappingKey(ctx, t, "file"); fileNode != nil {
			fileVal = yaml.ScalarText(fileNode)
		}
		if inlineNode := findMappingKey(ctx, t, "file_inline"); inlineNode != nil {
			b.InlineFile = yaml.ScalarText(inlineNode)
			if fileVal != "" {
				b.File = fileVal
			}
		} else if fileVal != "" {
			b.File = fileVal
		}

		if argsNode := findMappingKey(ctx, t, "args"); argsNode != nil {
			b.Args = parseBuildArgs(ctx, argsNode)
		}
		if targetNode := findMappingKey(ctx, t, "target"); targetNode != nil {
			b.Target = yaml.ScalarText(targetNode)
		}
		if policyNode := findMappingKey(ctx, t, "policy"); policyNode != nil {
			b.Policy = yaml.ScalarText(policyNode)
		}
		if noCacheNode := findMappingKey(ctx, t, "no_cache"); noCacheNode != nil {
			b.NoCache = yaml.ScalarText(noCacheNode) == "true"
		}
		if tagsNode := findMappingKey(ctx, t, "tags"); tagsNode != nil {
			b.Tags = nodeToStringSlice(ctx, tagsNode)
		}
		if labelsNode := findMappingKey(ctx, t, "labels"); labelsNode != nil {
			b.Labels = mappingNodeToMap(ctx, labelsNode)
		}
		if cacheFromNode := findMappingKey(ctx, t, "cache_from"); cacheFromNode != nil {
			b.CacheFrom = nodeToStringSlice(ctx, cacheFromNode)
		}
		if shmNode := findMappingKey(ctx, t, "shm_size"); shmNode != nil {
			b.ShmSize = yaml.ScalarText(shmNode)
		}
		if netNode := findMappingKey(ctx, t, "network"); netNode != nil {
			b.Network = yaml.ScalarText(netNode)
		}
	case *yaml.SequenceNode:
		if len(t.Values) == 0 {
			break
		}
		first := yaml.Resolve(ctx, t.Values[0])
		if _, ok := first.(*yaml.MappingNode); ok {
			for _, item := range t.Values {
				if m, ok := yaml.Resolve(ctx, item).(*yaml.MappingNode); ok {
					parseBuildFileItems(b, m)
				}
			}
		} else if yaml.IsScalar(first) {
			b.Context = filepath.Join(appPath, yaml.ScalarText(first))
		}
	default:
		if yaml.IsScalar(t) {
			b.Context = filepath.Join(appPath, yaml.ScalarText(t))
		}
	}

	if cfg.Image == "" {
		cfg.Image = BuiltImageName(appName, serviceName)
	}
}

// parseRestart handles string and mapping restart: values.
func parseRestart(ctx *yaml.Ctx, cfg *runtime.ContainerOptions, node yaml.Node) {
	switch t := yaml.Resolve(ctx, node).(type) {
	case *yaml.MappingNode:
		if nameNode := findMappingKey(ctx, t, "name"); nameNode != nil {
			cfg.Restart = yaml.ScalarText(nameNode)
		}
		if retryNode := findMappingKey(ctx, t, "max_retry"); retryNode != nil {
			n, _ := strconv.Atoi(yaml.ScalarText(retryNode))
			cfg.RestartMaxRetry = n
		}
	default:
		if yaml.IsScalar(t) {
			cfg.Restart = yaml.ScalarText(t)
		}
	}
}

// parseLogging converts a logging: mapping node into a LoggingOptions.
func parseLogging(ctx *yaml.Ctx, node yaml.Node) runtime.LoggingOptions {
	lc := runtime.LoggingOptions{}
	m, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode)
	if !ok {
		return lc
	}
	if driverNode := findMappingKey(ctx, m, "driver"); driverNode != nil {
		lc.Driver = yaml.ScalarText(driverNode)
	}
	if optsNode := findMappingKey(ctx, m, "options"); optsNode != nil {
		lc.Options = mappingNodeToMap(ctx, optsNode)
	}
	return lc
}

// parseVolumesFrom translates service names in volumes_from to container names.
func parseVolumesFrom(items []string, appName string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		// "container:name" or "container:name:ro" → literal container name
		if strings.HasPrefix(item, "container:") {
			result = append(result, strings.TrimPrefix(item, "container:"))
			continue
		}
		// "service_name" or "service_name:ro"
		parts := strings.SplitN(item, ":", 2)
		name := ContainerName(appName, parts[0])
		if len(parts) == 2 {
			result = append(result, name+":"+parts[1])
		} else {
			result = append(result, name)
		}
	}
	return result
}

// expandExtraHosts expands brace expressions in each extra_hosts entry.
// "node{1..3}.app.lh:container:proxy" → three individual host entries.
func expandExtraHosts(hosts []string) []string {
	result := make([]string, 0, len(hosts))
	for _, h := range hosts {
		result = append(result, expandBraces(h)...)
	}
	return result
}

func nodeToStringSlice(ctx *yaml.Ctx, node yaml.Node) []string {
	resolved := yaml.Resolve(ctx, node)
	if yaml.IsScalar(resolved) {
		return strings.Fields(yaml.ScalarText(resolved))
	}
	items := yaml.SeqItems(ctx, resolved)
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, yaml.ScalarText(yaml.Resolve(ctx, item)))
	}
	return result
}

// parseBuildFileItems parses a {filename: content} or {filename: {mode,content}}
// mapping node and populates b.InlineFile / b.InlineFiles.
func parseBuildFileItems(b *runtime.BuildOptions, item *yaml.MappingNode) {
	emptyCtx := yaml.NewCtx()
	for _, mv := range item.Values {
		key := yaml.ScalarText(mv.Key)
		var bf runtime.BuildFile
		if valMap, ok := mv.Value.(*yaml.MappingNode); ok {
			if contentNode := findMappingKey(emptyCtx, valMap, "content"); contentNode != nil {
				bf.Content = yaml.ScalarText(contentNode)
			}
			if modeNode := findMappingKey(emptyCtx, valMap, "mode"); modeNode != nil {
				n, err := strconv.ParseInt(yaml.ScalarText(modeNode), 8, 64)
				if err == nil {
					bf.Mode = n
				}
			}
		} else {
			bf.Content = yaml.ScalarText(mv.Value)
		}
		if key == "Dockerfile" {
			b.InlineFile = bf.Content
		} else {
			if b.InlineFiles == nil {
				b.InlineFiles = map[string]runtime.BuildFile{}
			}
			b.InlineFiles[key] = bf
		}
	}
}

// parseBuildArgs parses build args from a mapping or sequence node.
func parseBuildArgs(ctx *yaml.Ctx, node yaml.Node) map[string]string {
	if m, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode); ok {
		return mappingNodeToMap(ctx, m)
	}
	result := map[string]string{}
	for _, item := range yaml.SeqItems(ctx, node) {
		text := yaml.ScalarText(item)
		k, v, ok := strings.Cut(text, "=")
		if ok {
			result[k] = v
		} else {
			result[text] = os.Getenv(text)
		}
	}
	return result
}

// labelsNodeToMap converts a labels mapping node to map[string]string,
// serializing nested mapping/sequence values as JSON.
func labelsNodeToMap(ctx *yaml.Ctx, node yaml.Node) map[string]string {
	result := map[string]string{}
	m, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode)
	if !ok {
		return result
	}
	for _, mv := range m.Values {
		key := yaml.ScalarText(mv.Key)
		val := yaml.Resolve(ctx, mv.Value)
		if yaml.IsScalar(val) {
			result[key] = yaml.ScalarText(val)
		} else {
			result[key] = yaml.ConvertNodeToJson(ctx, val)
		}
	}
	return result
}

// envNodeToMap converts an env mapping or sequence node to map[string]string.
// Null values (e.g. SESSION_SECRET with no value) are resolved from the host environment.
func envNodeToMap(ctx *yaml.Ctx, node yaml.Node) map[string]string {
	result := map[string]string{}
	switch t := yaml.Resolve(ctx, node).(type) {
	case *yaml.MappingNode:
		for _, mv := range t.Values {
			key := yaml.ScalarText(mv.Key)
			val := yaml.Resolve(ctx, mv.Value)
			if val.Type() == yaml.NullType {
				result[key] = os.Getenv(key)
			} else {
				result[key] = yaml.ScalarText(val)
			}
		}
	case *yaml.SequenceNode:
		for _, item := range t.Values {
			val := yaml.ScalarText(yaml.Resolve(ctx, item))
			if idx := strings.Index(val, "="); idx >= 0 {
				result[val[:idx]] = val[idx+1:]
			} else {
				result[val] = os.Getenv(val)
			}
		}
	}
	return result
}

func mappingNodeToMap(ctx *yaml.Ctx, node yaml.Node) map[string]string {
	result := map[string]string{}
	m, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode)
	if !ok {
		return result
	}
	for _, mv := range m.Values {
		result[yaml.ScalarText(mv.Key)] = yaml.ScalarText(mv.Value)
	}
	return result
}

// parseEnvFiles loads env variables from one or more shell env files.
// The node may be a scalar (single path) or a sequence (list of paths).
// Files are sourced in order; variables defined later overwrite earlier ones.
// Within a file, variable expansion is supported (BASE_URL=x; API=${BASE_URL}/api).
// Paths are resolved relative to appPath.
func parseEnvFiles(ctx *yaml.Ctx, node yaml.Node, appPath string) map[string]string {
	var paths []string
	switch t := yaml.Resolve(ctx, node).(type) {
	case *yaml.SequenceNode:
		for _, n := range t.Values {
			paths = append(paths, yaml.ScalarText(yaml.Resolve(ctx, n)))
		}
	default:
		if yaml.IsScalar(t) {
			paths = []string{yaml.ScalarText(t)}
		}
	}
	result := map[string]string{}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(appPath, p)
		}
		vars, err := dotenv.Load(p)
		if err != nil {
			continue
		}
		for k, v := range vars {
			result[k] = v
		}
	}
	return result
}

func resolveVolumes(ctx *yaml.Ctx, node yaml.Node, appPath, appName, serviceName string, managedVolumes map[string]string) []runtime.VolumeMount {
	var result []runtime.VolumeMount
	switch t := yaml.Resolve(ctx, node).(type) {
	case *yaml.MappingNode:
		for _, mv := range t.Values {
			source := yaml.ScalarText(mv.Key)
			val := yaml.Resolve(ctx, mv.Value)
			if val.Type() == yaml.NullType || yaml.ScalarText(val) == "" {
				// Null value: key is the container path — managed volume keyed by path
				result = append(result, runtime.VolumeMount{
					Source: ServiceVolumeName(appName, serviceName, source),
					Target: source,
				})
			} else {
				result = append(result, runtime.VolumeMount{
					Source: resolveVolumeSrc(source, appPath, appName, serviceName, managedVolumes),
					Target: yaml.ScalarText(val),
				})
			}
		}
	case *yaml.SequenceNode:
		for _, item := range t.Values {
			val := yaml.ScalarText(yaml.Resolve(ctx, item))
			if val == "" {
				continue
			}
			idx := strings.Index(val, ":")
			if idx < 0 {
				// Single path: managed volume keyed by container path
				result = append(result, runtime.VolumeMount{
					Source: ServiceVolumeName(appName, serviceName, val),
					Target: val,
				})
			} else {
				source := val[:idx]
				target := val[idx+1:]
				result = append(result, runtime.VolumeMount{
					Source: resolveVolumeSrc(source, appPath, appName, serviceName, managedVolumes),
					Target: target,
				})
			}
		}
	}
	return result
}

// resolveVolumeSrc resolves a volume source to a host path or managed volume name.
// Absolute and relative paths remain as bind mounts; named volume keys are resolved
// via managedVolumes (top-level volumes: section) or ServiceVolumeName.
func resolveVolumeSrc(source, appPath, appName, serviceName string, managedVolumes map[string]string) string {
	if source == "" {
		return ""
	}
	if filepath.IsAbs(source) {
		return source
	}
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || source == "." {
		return filepath.Join(appPath, source)
	}
	// Relative path with directory separator, or a dotfile/dotdir name (a leading
	// dot is never a valid volume name) — relative to appPath
	if strings.Contains(source, "/") || strings.HasPrefix(source, ".") {
		return filepath.Join(appPath, source)
	}
	// Named volume key: use top-level managed name if declared, otherwise service-scoped
	if name, ok := managedVolumes[source]; ok {
		return name
	}
	return ServiceVolumeName(appName, serviceName, source)
}

func parsePorts(ctx *yaml.Ctx, node yaml.Node) map[string]string {
	result := map[string]string{}
	switch t := yaml.Resolve(ctx, node).(type) {
	case *yaml.MappingNode:
		for _, mv := range t.Values {
			hostSpec := yaml.ScalarText(mv.Key)
			containerPort := yaml.ScalarText(yaml.Resolve(ctx, mv.Value))
			if !strings.Contains(containerPort, "/") {
				containerPort += "/tcp"
			}
			result[hostSpec] = containerPort
		}
	case *yaml.SequenceNode:
		for _, item := range t.Values {
			val := yaml.ScalarText(yaml.Resolve(ctx, item))
			proto := "tcp"
			if idx := strings.LastIndex(val, "/"); idx >= 0 {
				proto = val[idx+1:]
				val = val[:idx]
			}
			parts := strings.Split(val, ":")
			switch len(parts) {
			case 1:
				// containerPort only — expose but no host binding
				result["0:"+parts[0]] = parts[0] + "/" + proto
			case 2:
				result[parts[0]] = parts[1] + "/" + proto
			case 3:
				// ip:hostPort:containerPort
				result[parts[0]+":"+parts[1]] = parts[2] + "/" + proto
			}
		}
	}
	return result
}
