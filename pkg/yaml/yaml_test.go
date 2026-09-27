package yaml

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml/ast"
)

func TestLoadValid(t *testing.T) {
	node, err := Load([]byte("a: 1\nb: two\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doc, ok := node.(*ast.DocumentNode)
	if !ok {
		t.Fatalf("expected DocumentNode, got %T", node)
	}
	if _, ok := doc.Body.(*ast.MappingNode); !ok {
		t.Fatalf("expected mapping node as document body, got %T", doc.Body)
	}
}

func TestLoadInvalid(t *testing.T) {
	_, err := Load([]byte("a: [1, 2\n"))
	if err == nil {
		t.Fatalf("expected error for malformed yaml, got nil")
	}
}

func TestLoadEmpty(t *testing.T) {
	node, err := Load([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node != nil {
		t.Fatalf("expected nil node for empty input, got %+v", node)
	}
}

func TestDumpRoundTrip(t *testing.T) {
	src := []byte("a: 1\nb:\n  - x\n  - y\n")
	node, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, err := Dump(node)
	if err != nil {
		t.Fatalf("unexpected error dumping: %v", err)
	}

	node2, err := Load(out)
	if err != nil {
		t.Fatalf("unexpected error reloading dumped yaml: %v", err)
	}

	m := &NodeManager{Node: node2}
	keys := m.Keys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("expected keys [a b], got %v", keys)
	}
}

func TestNodeManagerKeysOnMapping(t *testing.T) {
	node, err := Load([]byte("a: 1\nb: 2\nc: 3\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	keys := m.Keys()
	expected := []string{"a", "b", "c"}
	if len(keys) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, keys)
	}
	for i, k := range expected {
		if keys[i] != k {
			t.Fatalf("expected %v, got %v", expected, keys)
		}
	}
}

func TestNodeManagerKeysOnSequence(t *testing.T) {
	node, err := Load([]byte("- 1\n- 2\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	if keys := m.Keys(); keys != nil {
		t.Fatalf("expected nil keys for sequence node, got %v", keys)
	}
}

func TestNodeManagerKeysOnScalar(t *testing.T) {
	node, err := Load([]byte("hello\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	if keys := m.Keys(); keys != nil {
		t.Fatalf("expected nil keys for scalar node, got %v", keys)
	}
}

func TestNodeManagerKeysViaAlias(t *testing.T) {
	node, err := Load([]byte("base: &base\n  a: 1\n  b: 2\nref: *base\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	ref := m.Get("ref")
	if ref == nil {
		t.Fatalf("expected to find 'ref' key")
	}
	keys := ref.Keys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("expected [a b] via alias resolution, got %v", keys)
	}
}

func TestNodeManagerGetFound(t *testing.T) {
	node, err := Load([]byte("a: 1\nb: two\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	b := m.Get("b")
	if b == nil {
		t.Fatalf("expected to find key 'b'")
	}
	if ScalarText(b.Node) != "two" {
		t.Fatalf("expected value 'two', got %q", ScalarText(b.Node))
	}
}

func TestNodeManagerGetNotFound(t *testing.T) {
	node, err := Load([]byte("a: 1\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	if got := m.Get("missing"); got != nil {
		t.Fatalf("expected nil for missing key, got %+v", got)
	}
}

func TestNodeManagerGetOnEmptyMapping(t *testing.T) {
	node, err := Load([]byte("a: {}\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	a := m.Get("a")
	if a == nil {
		t.Fatalf("expected to find key 'a'")
	}
	if got := a.Get("anything"); got != nil {
		t.Fatalf("expected nil Get on empty mapping, got %+v", got)
	}
}

func TestNodeManagerJSON(t *testing.T) {
	node, err := Load([]byte("a: 1\nb: two\nc:\n  - 1\n  - 2\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	b, err := m.JSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"a":1,"b":"two","c":[1,2]}`
	if string(b) != expected {
		t.Fatalf("expected %s, got %s", expected, string(b))
	}
}

func TestNodeManagerJsonPrettyPrint(t *testing.T) {
	node, err := Load([]byte("a: 1\nb: 2\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	b, err := m.JsonPrettyPrint()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "\n    \"a\": 1") {
		t.Fatalf("expected pretty-printed indentation, got %s", out)
	}
}
