package config

import (
	"testing"

	"github.com/kraken-hub/kraken/pkg/yaml"
)

// mustLoadNode parses a YAML document string into a yaml.Node tree.
func mustLoadNode(t *testing.T, doc string) yaml.Node {
	t.Helper()
	node, err := yaml.Load([]byte(doc))
	if err != nil {
		t.Fatalf("yaml.Load failed: %v", err)
	}
	return node
}

// dumpToValue dumps a node back to YAML and decodes it into a generic
// interface{} so tests can compare structure instead of exact formatting.
func dumpToValue(t *testing.T, node yaml.Node) interface{} {
	t.Helper()
	data, err := yaml.Dump(node)
	if err != nil {
		t.Fatalf("yaml.Dump failed: %v", err)
	}
	var v interface{}
	if err := yaml.Unmarshal(data, &v); err != nil {
		t.Fatalf("yaml.Unmarshal of dumped output failed: %v\noutput was:\n%s", err, data)
	}
	return v
}

// mapKeys extracts ordered top-level keys from a mapping node.
func mapKeys(node yaml.Node) []string {
	m, ok := node.(*yaml.MappingNode)
	if !ok {
		return nil
	}
	var keys []string
	for _, mv := range m.Values {
		keys = append(keys, yaml.ScalarText(mv.Key))
	}
	return keys
}
