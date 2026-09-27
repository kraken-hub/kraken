package yaml

import (
	"testing"

	"github.com/goccy/go-yaml/ast"
)

func TestRemoveAllComments(t *testing.T) {
	src := []byte("# head comment\na: 1 # line comment\nb: 2\n")
	node, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	RemoveAllComments(node)

	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		if n == nil {
			return
		}
		if n.GetComment() != nil {
			t.Fatalf("expected no comments left, found on node %+v", n)
		}
		switch v := n.(type) {
		case *ast.DocumentNode:
			walk(v.Body)
		case *ast.MappingNode:
			if v.FootComment != nil {
				t.Fatalf("expected mapping FootComment cleared")
			}
			for _, mv := range v.Values {
				walk(mv)
			}
		case *ast.MappingValueNode:
			if v.FootComment != nil {
				t.Fatalf("expected mapping value FootComment cleared")
			}
			walk(v.Key)
			walk(v.Value)
		case *ast.SequenceNode:
			if v.FootComment != nil {
				t.Fatalf("expected sequence FootComment cleared")
			}
			for _, item := range v.Values {
				walk(item)
			}
		}
	}
	walk(node)
}

func buildSimpleMapping() (*ast.MappingNode, *ast.MappingValueNode, *ast.MappingValueNode) {
	pairA := NewPair(NewString("a"), NewString("1"))
	pairB := NewPair(NewString("b"), NewString("2"))
	root := NewMapping()
	root.Values = append(root.Values, pairA, pairB)
	return root, pairA, pairB
}

func TestRemovePair(t *testing.T) {
	root, pairA, pairB := buildSimpleMapping()

	RemovePair(root, 0)
	if len(root.Values) != 1 {
		t.Fatalf("expected 1 remaining pair, got %d", len(root.Values))
	}
	if root.Values[0] != pairB {
		t.Fatalf("expected pairB to remain, got %+v", root.Values[0])
	}
	_ = pairA
}

func TestRemovePairLastElement(t *testing.T) {
	root, pairA, _ := buildSimpleMapping()

	RemovePair(root, 1)
	if len(root.Values) != 1 {
		t.Fatalf("expected 1 remaining pair, got %d", len(root.Values))
	}
	if root.Values[0] != pairA {
		t.Fatalf("expected pairA to remain, got %+v", root.Values[0])
	}
}

func TestFindPairFound(t *testing.T) {
	root, pairA, pairB := buildSimpleMapping()
	ctx := NewCtx()

	idx, mv := FindPair(ctx, root, "a")
	if idx != 0 || mv != pairA {
		t.Fatalf("expected (0, pairA), got (%d, %+v)", idx, mv)
	}

	idx, mv = FindPair(ctx, root, "b")
	if idx != 1 || mv != pairB {
		t.Fatalf("expected (1, pairB), got (%d, %+v)", idx, mv)
	}
}

func TestFindPairNotFound(t *testing.T) {
	root, _, _ := buildSimpleMapping()
	ctx := NewCtx()
	idx, mv := FindPair(ctx, root, "missing")
	if idx != -1 || mv != nil {
		t.Fatalf("expected (-1, nil), got (%d, %+v)", idx, mv)
	}
}

func TestFindPairNonMapping(t *testing.T) {
	seq := NewSequence()
	seq.Values = append(seq.Values, NewString("a"))
	ctx := NewCtx()
	idx, mv := FindPair(ctx, seq, "a")
	if idx != -1 || mv != nil {
		t.Fatalf("expected (-1, nil) for non-mapping node, got (%d, %+v)", idx, mv)
	}
}

func TestFindValueFound(t *testing.T) {
	root, _, pairB := buildSimpleMapping()
	ctx := NewCtx()

	idx, v := FindValue(ctx, root, "b")
	if idx != 1 || v != pairB.Value {
		t.Fatalf("expected (1, pairB.Value), got (%d, %+v)", idx, v)
	}
}

func TestFindValueNotFound(t *testing.T) {
	root, _, _ := buildSimpleMapping()
	ctx := NewCtx()
	idx, v := FindValue(ctx, root, "missing")
	if idx != -1 || v != nil {
		t.Fatalf("expected (-1, nil), got (%d, %+v)", idx, v)
	}
}

func loadMappingContent(t *testing.T, src string, key string) ast.Node {
	t.Helper()
	node, err := Load([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	target := m.Get(key)
	if target == nil {
		t.Fatalf("expected to find key %q", key)
	}
	return target.Node
}

func mappingPairs(t *testing.T, n ast.Node) map[string]string {
	t.Helper()
	m, ok := n.(*ast.MappingNode)
	if !ok {
		t.Fatalf("expected mapping node, got %T", n)
	}
	result := map[string]string{}
	for _, mv := range m.Values {
		result[ScalarText(mv.Key)] = ScalarText(mv.Value)
	}
	return result
}

func mappingOrder(n *ast.MappingNode) []string {
	var order []string
	for _, mv := range n.Values {
		order = append(order, ScalarText(mv.Key))
	}
	return order
}

func TestExpandMergeNodeNoMerge(t *testing.T) {
	ctx := NewCtx()
	target := loadMappingContent(t, "derived:\n  a: 1\n  b: 2\n", "derived")
	expanded := ExpandMergeNode(ctx, target)
	pairs := mappingPairs(t, expanded)
	if pairs["a"] != "1" || pairs["b"] != "2" {
		t.Fatalf("expected {a:1 b:2}, got %+v", pairs)
	}
}

func TestExpandMergeNodeSimpleOverride(t *testing.T) {
	src := "base: &base\n  a: 1\n  b: 2\nderived:\n  <<: *base\n  b: 3\n  c: 4\n"
	node, err := Load([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := NewCtx()
	ctx.Index(node)
	target := loadMappingContent(t, src, "derived")
	expanded := ExpandMergeNode(ctx, target)
	pairs := mappingPairs(t, expanded)

	expected := map[string]string{"a": "1", "b": "3", "c": "4"}
	if len(pairs) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, pairs)
	}
	for k, v := range expected {
		if pairs[k] != v {
			t.Fatalf("expected %s=%s, got %v", k, v, pairs)
		}
	}

	order := mappingOrder(expanded)
	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Fatalf("expected order [a b c], got %v", order)
	}
}

func TestExpandMergeNodeSequenceMerge(t *testing.T) {
	src := "a: &a\n  x: 1\nb: &b\n  y: 2\nderived:\n  <<: [*a, *b]\n  z: 3\n"
	node, err := Load([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := NewCtx()
	ctx.Index(node)
	target := loadMappingContent(t, src, "derived")
	expanded := ExpandMergeNode(ctx, target)
	pairs := mappingPairs(t, expanded)

	expected := map[string]string{"x": "1", "y": "2", "z": "3"}
	if len(pairs) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, pairs)
	}
	for k, v := range expected {
		if pairs[k] != v {
			t.Fatalf("expected %s=%s, got %v", k, v, pairs)
		}
	}
}

// This exercises a two-level merge chain (derived <<: *mid <<: *base) where an
// intermediate level (mid) both merges in a base and overrides one of the
// base's keys itself. Own-key-overrides-merged-value only works correctly at
// the top-level node passed directly to ExpandMergeNode (it does a remove +
// reinsert-with-overridden-value pass). Nested merges reached through
// MergeNodeExpander's generic per-key loop (mid's "<<" comes before mid's own
// "b: 20" in source order) do not get that treatment: the merged-in value is
// inserted first and mid's own "b: 20" is then skipped by the plain
// "already present" check, so the base's value silently wins instead of
// mid's override. This looks like a bug (only the outermost mapping's own
// keys reliably win over merge-supplied values), documented here rather
// than fixed. Ported from the yaml.v3 implementation, same behavior.
func TestExpandMergeNodeChainedMerge(t *testing.T) {
	src := "base: &base\n  a: 1\n  b: 2\nmid: &mid\n  <<: *base\n  b: 20\n  c: 3\nderived:\n  <<: *mid\n  c: 30\n  d: 4\n"
	node, err := Load([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := NewCtx()
	ctx.Index(node)
	target := loadMappingContent(t, src, "derived")
	expanded := ExpandMergeNode(ctx, target)
	pairs := mappingPairs(t, expanded)

	expected := map[string]string{"a": "1", "b": "2", "c": "30", "d": "4"}
	if len(pairs) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, pairs)
	}
	for k, v := range expected {
		if pairs[k] != v {
			t.Fatalf("expected %s=%s, got %v", k, v, pairs)
		}
	}
}

func TestMergeNodeExpanderAppend(t *testing.T) {
	root := NewMapping()
	src := NewMapping()
	src.Values = append(src.Values,
		NewPair(NewString("a"), NewString("1")),
		NewPair(NewString("b"), NewString("2")),
	)

	MergeNodeExpander(NewCtx(), root, src, -1)

	pairs := mappingPairs(t, root)
	if pairs["a"] != "1" || pairs["b"] != "2" {
		t.Fatalf("expected {a:1 b:2}, got %+v", pairs)
	}
}

func TestMergeNodeExpanderSkipsExistingKeys(t *testing.T) {
	root := NewMapping()
	root.Values = append(root.Values, NewPair(NewString("a"), NewString("existing")))

	src := NewMapping()
	src.Values = append(src.Values,
		NewPair(NewString("a"), NewString("from-merge")),
		NewPair(NewString("b"), NewString("2")),
	)

	MergeNodeExpander(NewCtx(), root, src, -1)

	pairs := mappingPairs(t, root)
	if pairs["a"] != "existing" {
		t.Fatalf("expected existing key to win, got %+v", pairs)
	}
	if pairs["b"] != "2" {
		t.Fatalf("expected new key to be merged in, got %+v", pairs)
	}
}

func TestMergeNodeExpanderInsertAtIndex(t *testing.T) {
	root := NewMapping()
	root.Values = append(root.Values, NewPair(NewString("z"), NewString("26")))

	src := NewMapping()
	src.Values = append(src.Values,
		NewPair(NewString("a"), NewString("1")),
		NewPair(NewString("b"), NewString("2")),
	)

	MergeNodeExpander(NewCtx(), root, src, 0)

	order := mappingOrder(root)
	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "z" {
		t.Fatalf("expected order [a b z], got %v", order)
	}
}

func TestMergeNodeExpanderEmptySource(t *testing.T) {
	root := NewMapping()
	root.Values = append(root.Values, NewPair(NewString("a"), NewString("1")))

	src := NewMapping()

	MergeNodeExpander(NewCtx(), root, src, -1)

	if len(root.Values) != 1 {
		t.Fatalf("expected root to remain unchanged, got %+v", root.Values)
	}
}
