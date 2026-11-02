package config

import (
	"fmt"
	"strings"

	"github.com/kraken-hub/kraken/pkg/yaml"
)

func prepareEnvNode(ctx *yaml.Ctx, node yaml.Node, env string) {
	resolved := yaml.Resolve(ctx, node)

	if m, ok := resolved.(*yaml.MappingNode); ok {
		for {
			index, mv := findEnvNode(ctx, m)
			if index == -1 {
				break
			}

			envs := strings.Split(env, ",")
			parts := envKeyParts(yaml.ScalarText(mv.Key))

			envVal := mv.Value
			yaml.RemovePair(m, index)

			doAppend := false
			for _, e := range envs {
				for _, p := range parts {
					if e == p {
						doAppend = true
						break
					}
				}
			}

			if doAppend {
				appendNode(ctx, m, envVal, index)
			} else {
				rescueAnchors(ctx, m, envVal)
			}
		}

		for _, mv := range m.Values {
			prepareEnvNode(ctx, mv.Key, env)
			prepareEnvNode(ctx, mv.Value, env)
		}
		return
	}

	if s, ok := resolved.(*yaml.SequenceNode); ok {
		for _, item := range s.Values {
			prepareEnvNode(ctx, item, env)
		}
	}
}

// rescueAnchors moves every anchor defined anywhere under dropped (a section
// about to be removed: a non-matching ~env section or an x- key) onto its
// first alias under node. Anchors are visited in reverse document order, so
// an anchor whose only alias sits inside a later dropped anchor's value is
// rescued after that value has itself been moved under node.
func rescueAnchors(ctx *yaml.Ctx, node, dropped yaml.Node) {
	anchors := yaml.Anchors(dropped)
	for i := len(anchors) - 1; i >= 0; i-- {
		replaceAnchorUsageNode(ctx, node, anchors[i])
	}
}

// removeExtensionKeys drops every top-level key starting with "x-" (anchor-only
// blocks), rescuing their anchors first.
// Dropped values are rescued in reverse order, so an x- block aliasing an
// earlier x- block works the same as anchors nested within one block.
func removeExtensionKeys(ctx *yaml.Ctx, root *yaml.MappingNode) {
	var dropped []yaml.Node
	for i := 0; i < len(root.Values); {
		if strings.HasPrefix(yaml.ScalarText(root.Values[i].Key), "x-") {
			dropped = append(dropped, root.Values[i].Value)
			yaml.RemovePair(root, i)
			continue
		}
		i++
	}
	for i := len(dropped) - 1; i >= 0; i-- {
		rescueAnchors(ctx, root, dropped[i])
	}
}

// replaceAnchorUsageNode replaces, in place, the first alias reference to
// anchor (an *yaml.AnchorNode wrapper, or anything Resolve()-equivalent to
// one) found anywhere under node with the anchor itself, so any later alias
// to it stays valid - used to rescue an anchor before the section defining
// it is dropped (a non-matching ~env section, an x- key, or a consumed
// extend/extends directive).
func replaceAnchorUsageNode(ctx *yaml.Ctx, node yaml.Node, anchor yaml.Node) bool {
	target := yaml.Resolve(ctx, anchor)
	a, isAnchor := anchor.(*yaml.AnchorNode)
	switch t := node.(type) {
	case *yaml.AnchorNode:
		return replaceAnchorUsageNode(ctx, t.Value, anchor)
	case *yaml.MappingNode:
		for i, mv := range t.Values {
			if isAliasTo(ctx, mv.Value, target) {
				if isAnchor {
					ctx.Anchors[yaml.AnchorName(a)] = yaml.MoveAnchorToPair(t, i, a)
				} else {
					mv.Value = anchor
				}
				return true
			}
			if replaceAnchorUsageNode(ctx, mv.Value, anchor) {
				return true
			}
		}
	case *yaml.SequenceNode:
		for i, item := range t.Values {
			if isAliasTo(ctx, item, target) {
				if isAnchor {
					ctx.Anchors[yaml.AnchorName(a)] = yaml.MoveAnchorToItem(t, i, a)
				} else {
					t.Values[i] = anchor
				}
				return true
			}
			if replaceAnchorUsageNode(ctx, item, anchor) {
				return true
			}
		}
	}
	return false
}

func isAliasTo(ctx *yaml.Ctx, n yaml.Node, anchor yaml.Node) bool {
	if !yaml.IsAlias(n) {
		return false
	}
	return yaml.Resolve(ctx, n) == anchor
}

// envKeyParts returns the comma-separated environment names from a `~env`
// key, e.g. "~prod,test" -> ["prod", "test"]. Anything from a "#" onward is
// a disambiguator, dropped before splitting - it lets multiple `~env`
// sections targeting the same environment(s) coexist as distinct YAML
// mapping keys (YAML disallows duplicate keys at the same level) without
// that suffix itself being treated as another environment name to match.
func envKeyParts(key string) []string {
	if idx := strings.IndexByte(key, '#'); idx != -1 {
		key = key[:idx]
	}
	return strings.Split(string([]rune(key)[1:]), ",")
}

func findEnvNode(ctx *yaml.Ctx, node yaml.Node) (int, *yaml.MappingValueNode) {
	m, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode)
	if !ok {
		return -1, nil
	}
	for i, mv := range m.Values {
		text := yaml.ScalarText(mv.Key)
		if len(text) > 1 && text[0] == '~' {
			return i, mv
		}
	}
	return -1, nil
}

// extendNode merges node (local overrides) into rootNode (base). For keys
// present in node, the value in rootNode is replaced (the pre-existing key
// node is left untouched); for keys absent in rootNode, the key+value pair
// from node is appended.
func extendNode(ctx *yaml.Ctx, rootNode *yaml.MappingNode, node yaml.Node) {
	m, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode)
	if !ok {
		return
	}
	for _, mv := range m.Values {
		val := yaml.Resolve(ctx, mv.Value)
		key := yaml.ScalarText(mv.Key)
		if idx, existing := yaml.FindPair(ctx, rootNode, key); existing != nil {
			yaml.ReplacePairValue(rootNode, idx, val)
		} else {
			yaml.AppendPair(rootNode, yaml.NewPair(mv.Key, val))
		}
	}
}

// appendNode merges node's key/value pairs into root. Existing keys whose
// current value is itself a mapping are merged into recursively; existing
// keys whose value is anything else (a scalar, sequence, ...) are
// overwritten in place. New keys are inserted starting at index (or
// appended if index is -1), preserving their relative order.
//
// index is root's own insertion point (where node's source section sat
// before removal, or -1 if there's no such position). When recursing into an
// existing nested mapping, that same position is compared against the
// matched key's position in root: if node's section preceded the key in the
// source, new nested keys are inserted at the front of the nested mapping
// (so they keep reading before it); otherwise they're appended, keeping the
// merge's on-page order intuitive instead of always dumping merged keys at
// the very end regardless of where the source section was written.
func appendNode(ctx *yaml.Ctx, root *yaml.MappingNode, node yaml.Node, index int) {
	m, ok := yaml.Resolve(ctx, node).(*yaml.MappingNode)
	if !ok {
		return
	}

	for _, mv := range m.Values {
		key := yaml.ScalarText(mv.Key)
		idx, existing := yaml.FindValue(ctx, root, key)
		if existing == nil {
			newPair := yaml.NewPair(mv.Key, yaml.Resolve(ctx, mv.Value))
			if index == -1 {
				yaml.AppendPair(root, newPair)
			} else {
				yaml.InsertPairAt(root, newPair, index)
				index++
			}
			continue
		}

		if existingMap, ok := yaml.Resolve(ctx, existing).(*yaml.MappingNode); ok {
			nestedIndex := -1
			if index != -1 && index <= idx {
				nestedIndex = 0
			}
			appendNode(ctx, existingMap, mv.Value, nestedIndex)
		} else {
			yaml.ReplacePairValue(root, idx, yaml.Resolve(ctx, mv.Value))
		}
	}
}

func convertSequenceToMapping(ctx *yaml.Ctx, node yaml.Node, separator string) *yaml.MappingNode {
	result := yaml.NewMapping()
	for _, n := range yaml.SeqItems(ctx, node) {
		if yaml.IsAlias(n) {
			sub := convertSequenceToMapping(ctx, n, separator)
			result.Values = append(result.Values, sub.Values...)
			continue
		}
		values := strings.SplitN(yaml.ScalarText(n), separator, 2)
		var valueNode yaml.Node
		if len(values) > 1 {
			valueNode = yaml.NewString(values[1])
		} else {
			valueNode = yaml.NewNull()
		}
		result.Values = append(result.Values, yaml.NewPair(yaml.NewString(values[0]), valueNode))
	}
	return result
}

func convertMappingToSequence(ctx *yaml.Ctx, node yaml.Node, separator string) *yaml.SequenceNode {
	result := yaml.NewSequence()
	for _, mv := range yaml.MapPairs(ctx, node) {
		if yaml.IsAlias(mv.Value) {
			sub := convertMappingToSequence(ctx, mv.Value, separator)
			result.Values = append(result.Values, sub.Values...)
			continue
		}
		str := yaml.ScalarText(mv.Key)
		if v := yaml.ScalarText(mv.Value); v != "" {
			str = fmt.Sprintf("%v%s%v", str, separator, v)
		}
		result.Values = append(result.Values, yaml.NewString(str))
	}
	return result
}

// sequenceNodeToStrings converts a sequence YAML node to a []string.
func sequenceNodeToStrings(ctx *yaml.Ctx, node yaml.Node) []string {
	items := yaml.SeqItems(ctx, node)
	result := make([]string, 0, len(items))
	for _, n := range items {
		result = append(result, yaml.ScalarText(n))
	}
	return result
}

// filterNodeByKeys returns a copy of a mapping node containing only the
// specified keys (includeKeys) or all keys except excludeKeys. Non-mapping
// nodes are returned unchanged.
func filterNodeByKeys(ctx *yaml.Ctx, node yaml.Node, includeKeys, excludeKeys []string) yaml.Node {
	resolved := yaml.Resolve(ctx, node)
	m, ok := resolved.(*yaml.MappingNode)
	if !ok {
		return resolved
	}

	filtered := yaml.NewMapping()
	for _, mv := range m.Values {
		key := yaml.ScalarText(mv.Key)
		include := len(includeKeys) == 0
		if len(includeKeys) > 0 {
			for _, k := range includeKeys {
				if k == key {
					include = true
					break
				}
			}
		}
		if include && len(excludeKeys) > 0 {
			for _, k := range excludeKeys {
				if k == key {
					include = false
					break
				}
			}
		}
		if include {
			// Wrap in a fresh pair rather than reusing mv itself: mv.Value
			// stays shared with the base service (needed for addAnchors to
			// detect and compact the sharing as &anchor/*alias), but the
			// *pair* must be independent, or addAnchors mutating this
			// pair's Value in place - to wrap it in an anchor - would mutate
			// the base service's own view of the exact same pair object too,
			// producing a duplicated anchor definition instead of one
			// anchor plus one alias.
			filtered.Values = append(filtered.Values, yaml.NewPair(mv.Key, mv.Value))
		}
	}
	return filtered
}

func isAnchorCandidate(n yaml.Node) bool {
	return n != nil && !yaml.IsScalar(n) && !yaml.IsAlias(n)
}

// addAnchors walks the document tree and, for any non-scalar value node
// that appears more than once (by pointer), anchors the first occurrence
// and replaces subsequent occurrences with alias nodes. This produces
// proper YAML anchor/alias output (e.g. volumes: *ref_0) for nodes shared
// by pointer as a result of scale expansion or extend/extends merging.
func addAnchors(root yaml.Node) {
	counts := map[yaml.Node]int{}
	var countPass func(yaml.Node)
	countPass = func(n yaml.Node) {
		switch t := n.(type) {
		case *yaml.DocumentNode:
			countPass(t.Body)
		case *yaml.AnchorNode:
			countPass(t.Value)
		case *yaml.MappingNode:
			for _, mv := range t.Values {
				if isAnchorCandidate(mv.Value) {
					counts[mv.Value]++
				}
				countPass(mv.Value)
			}
		case *yaml.SequenceNode:
			for _, item := range t.Values {
				if isAnchorCandidate(item) {
					counts[item]++
				}
				countPass(item)
			}
		}
	}
	countPass(root)

	anchorSeq := 0
	anchored := map[yaml.Node]*yaml.AnchorNode{}
	var replacePass func(yaml.Node)
	replacePass = func(n yaml.Node) {
		switch t := n.(type) {
		case *yaml.DocumentNode:
			replacePass(t.Body)
		case *yaml.AnchorNode:
			replacePass(t.Value)
		case *yaml.MappingNode:
			for _, mv := range t.Values {
				child := mv.Value
				if isAnchorCandidate(child) && counts[child] > 1 {
					if anc, ok := anchored[child]; ok {
						mv.Value = yaml.NewAliasTo(anc)
						continue
					}
					anc, ok := child.(*yaml.AnchorNode)
					if !ok {
						anc = yaml.NewAnchor(child, fmt.Sprintf("ref_%d", anchorSeq))
						anchorSeq++
						mv.Value = anc
					}
					anchored[child] = anc
				}
				replacePass(child)
			}
		case *yaml.SequenceNode:
			for i, item := range t.Values {
				if isAnchorCandidate(item) && counts[item] > 1 {
					if anc, ok := anchored[item]; ok {
						t.Values[i] = yaml.NewAliasTo(anc)
						continue
					}
					anc, ok := item.(*yaml.AnchorNode)
					if !ok {
						anc = yaml.NewAnchor(item, fmt.Sprintf("ref_%d", anchorSeq))
						anchorSeq++
						t.Values[i] = anc
					}
					anchored[item] = anc
				}
				replacePass(item)
			}
		}
	}
	replacePass(root)
}
