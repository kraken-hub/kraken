package config

import (
	"path/filepath"
	"strings"

	"github.com/kraken-hub/kraken/pkg/yaml"
)

// rebaseServicePaths makes the relative host paths of every service in a file
// loaded through extends - build context, env_file and bind-mount volume
// sources - relative to that file. Service paths are resolved against the app
// directory when containers are created, so each path is rewritten to the same
// location expressed relative to the app directory. Nothing changes when the
// file lives in the app directory itself.
func rebaseServicePaths(ctx *yaml.Ctx, services *yaml.MappingNode, fileDir, appDir string) {
	if services == nil || fileDir == "" || appDir == "" || filepath.Clean(fileDir) == filepath.Clean(appDir) {
		return
	}
	rebase := func(p string) string {
		return rebasePath(p, fileDir, appDir)
	}

	// Anchored nodes can be reached from several services; rebase each once.
	seen := map[yaml.Node]bool{}
	for _, svcPair := range services.Values {
		svcMap, ok := yaml.Resolve(ctx, svcPair.Value).(*yaml.MappingNode)
		if !ok || seen[svcMap] {
			continue
		}
		seen[svcMap] = true

		for i := range svcMap.Values {
			value := yaml.Resolve(ctx, svcMap.Values[i].Value)
			if seen[value] {
				continue
			}
			switch yaml.ScalarText(svcMap.Values[i].Key) {
			case "build":
				seen[value] = true
				rebaseBuild(ctx, svcMap, i, value, rebase)
			case "env_file":
				seen[value] = true
				rebaseEnvFile(ctx, svcMap, i, value, rebase)
			case "volumes":
				seen[value] = true
				rebaseVolumes(ctx, value, rebase)
			}
		}
	}
}

// rebasePath maps a path relative to fileDir to the same location relative to
// appDir. The result keeps a leading "./" or "../", so a volume source is still
// recognised as a bind mount rather than a volume name.
func rebasePath(p, fileDir, appDir string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	target := filepath.Join(fileDir, p)
	rel, err := filepath.Rel(appDir, target)
	if err != nil {
		return target
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return rel
	}
	return "./" + rel
}

// isBindSource reports whether a relative volume source is a host path, using
// the same rule as volume resolution: it contains "/" or starts with ".".
func isBindSource(source string) bool {
	return strings.Contains(source, "/") || strings.HasPrefix(source, ".")
}

func rebaseBuild(ctx *yaml.Ctx, svcMap *yaml.MappingNode, idx int, value yaml.Node, rebase func(string) string) {
	switch t := value.(type) {
	case *yaml.MappingNode:
		ci, contextNode := yaml.FindValue(ctx, t, "context")
		if contextNode == nil {
			// The context defaults to the app directory; for a service written
			// in another directory that default is the file's own directory.
			yaml.AppendPair(t, yaml.NewPair(yaml.NewString("context"), yaml.NewString(rebase("."))))
			return
		}
		switch c := yaml.Resolve(ctx, contextNode).(type) {
		case *yaml.SequenceNode:
			rebaseFirstScalar(ctx, c, rebase)
		default:
			if yaml.IsScalar(c) {
				yaml.ReplacePairValue(t, ci, yaml.NewString(rebase(yaml.ScalarText(c))))
			}
		}
	case *yaml.SequenceNode:
		// A list starting with mappings is an inline context, no path to move.
		rebaseFirstScalar(ctx, t, rebase)
	default:
		if yaml.IsScalar(t) {
			yaml.ReplacePairValue(svcMap, idx, yaml.NewString(rebase(yaml.ScalarText(t))))
		}
	}
}

func rebaseFirstScalar(ctx *yaml.Ctx, s *yaml.SequenceNode, rebase func(string) string) {
	if len(s.Values) == 0 {
		return
	}
	if first := yaml.Resolve(ctx, s.Values[0]); yaml.IsScalar(first) {
		s.Values[0] = yaml.AlignColumn(yaml.NewString(rebase(yaml.ScalarText(first))), yaml.Column(s.Values[0]))
	}
}

func rebaseEnvFile(ctx *yaml.Ctx, svcMap *yaml.MappingNode, idx int, value yaml.Node, rebase func(string) string) {
	switch t := value.(type) {
	case *yaml.SequenceNode:
		for i, item := range t.Values {
			if r := yaml.Resolve(ctx, item); yaml.IsScalar(r) {
				t.Values[i] = yaml.AlignColumn(yaml.NewString(rebase(yaml.ScalarText(r))), yaml.Column(item))
			}
		}
	default:
		if yaml.IsScalar(t) {
			yaml.ReplacePairValue(svcMap, idx, yaml.NewString(rebase(yaml.ScalarText(t))))
		}
	}
}

func rebaseVolumes(ctx *yaml.Ctx, value yaml.Node, rebase func(string) string) {
	switch t := value.(type) {
	case *yaml.MappingNode:
		for i, mv := range t.Values {
			target := yaml.Resolve(ctx, mv.Value)
			if target == nil || target.Type() == yaml.NullType || yaml.ScalarText(target) == "" {
				continue // a null target makes the key a container path
			}
			source := yaml.ScalarText(mv.Key)
			if filepath.IsAbs(source) || !isBindSource(source) {
				continue
			}
			key := yaml.AlignColumn(yaml.NewString(rebase(source)), yaml.Column(mv.Key)).(*yaml.StringNode)
			t.Values[i] = yaml.NewPair(key, mv.Value)
		}
	case *yaml.SequenceNode:
		for i, item := range t.Values {
			text := yaml.ScalarText(yaml.Resolve(ctx, item))
			idx := strings.Index(text, ":")
			if idx < 0 {
				continue // a single path is a container path
			}
			source := text[:idx]
			if filepath.IsAbs(source) || !isBindSource(source) {
				continue
			}
			t.Values[i] = yaml.AlignColumn(yaml.NewString(rebase(source)+text[idx:]), yaml.Column(item))
		}
	}
}
