package config

import (
	"reflect"
	"testing"

	"github.com/kraken-hub/kraken/pkg/yaml"
)

func str(v string) *yaml.StringNode {
	return yaml.NewString(v)
}

func nullNode() *yaml.NullNode {
	return yaml.NewNull()
}

func pair(key yaml.MapKeyNode, value yaml.Node) *yaml.MappingValueNode {
	return yaml.NewPair(key, value)
}

func mapping(pairs ...*yaml.MappingValueNode) *yaml.MappingNode {
	m := yaml.NewMapping()
	m.Values = pairs
	return m
}

func strPair(k, v string) *yaml.MappingValueNode {
	return pair(str(k), str(v))
}

func sequence(items ...yaml.Node) *yaml.SequenceNode {
	s := yaml.NewSequence()
	s.Values = items
	return s
}

func newAnchor(name string, value yaml.Node) *yaml.AnchorNode {
	return yaml.NewAnchor(value, name)
}

func newAliasNode(name string) *yaml.AliasNode {
	return yaml.NewAliasNode(name)
}

// --- prepareEnvNode / findEnvNode / replaceAnchorUsageNode ---

func TestPrepareEnvNode_MatchingSectionMerged(t *testing.T) {
	doc := `
app:
  image: myapp
  ~dev,test:
    env:
      DEBUG: "1"
  ~prod:
    restart: always
`
	node := mustLoadNode(t, doc)
	ctx := yaml.NewCtx()
	ctx.Index(node)
	prepareEnvNode(ctx, node, "dev")

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"app": map[string]interface{}{
			"image": "myapp",
			"env": map[string]interface{}{
				"DEBUG": "1",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareEnvNode_SecondEnvInCommaListMatches(t *testing.T) {
	doc := `
app:
  image: myapp
  ~dev,test:
    env:
      DEBUG: "1"
`
	node := mustLoadNode(t, doc)
	ctx := yaml.NewCtx()
	ctx.Index(node)
	prepareEnvNode(ctx, node, "test")

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"app": map[string]interface{}{
			"image": "myapp",
			"env": map[string]interface{}{
				"DEBUG": "1",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareEnvNode_NoMatchDropsSection(t *testing.T) {
	doc := `
app:
  image: myapp
  ~dev,test:
    env:
      DEBUG: "1"
  ~prod:
    restart: always
`
	node := mustLoadNode(t, doc)
	ctx := yaml.NewCtx()
	ctx.Index(node)
	prepareEnvNode(ctx, node, "staging")

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"app": map[string]interface{}{
			"image": "myapp",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareEnvNode_ActiveEnvCommaListMatchesAnyPart(t *testing.T) {
	doc := `
app:
  image: myapp
  ~dev,test:
    env:
      DEBUG: "1"
`
	node := mustLoadNode(t, doc)
	ctx := yaml.NewCtx()
	ctx.Index(node)
	prepareEnvNode(ctx, node, "qa,test")

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"app": map[string]interface{}{
			"image": "myapp",
			"env": map[string]interface{}{
				"DEBUG": "1",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareEnvNode_MergesIntoExistingKey(t *testing.T) {
	doc := `
app:
  env:
    FOO: bar
  ~prod:
    env:
      BAZ: qux
`
	node := mustLoadNode(t, doc)
	ctx := yaml.NewCtx()
	ctx.Index(node)
	prepareEnvNode(ctx, node, "prod")

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"app": map[string]interface{}{
			"env": map[string]interface{}{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareEnvNode_MergeOverridesExistingScalar(t *testing.T) {
	doc := `
app:
  env:
    FOO: bar
  ~prod:
    env:
      FOO: prodval
`
	node := mustLoadNode(t, doc)
	ctx := yaml.NewCtx()
	ctx.Index(node)
	prepareEnvNode(ctx, node, "prod")

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"app": map[string]interface{}{
			"env": map[string]interface{}{
				"FOO": "prodval",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestPrepareEnvNode_DroppedSectionAnchorInlined(t *testing.T) {
	doc := `
~prod:
  base: &base
    image: redis
app:
  from: *base
`
	node := mustLoadNode(t, doc)
	ctx := yaml.NewCtx()
	ctx.Index(node)
	prepareEnvNode(ctx, node, "dev")

	got := dumpToValue(t, node)
	want := map[string]interface{}{
		"app": map[string]interface{}{
			"from": map[string]interface{}{
				"image": "redis",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestFindEnvNode_SingleTildeCharIsNotEnvKey(t *testing.T) {
	node := mapping(pair(nullNode(), str("value")), strPair("plain", "x"))
	index, envNode := findEnvNode(yaml.NewCtx(), node)
	if index != -1 || envNode != nil {
		t.Fatalf("expected no env node found for bare '~' key, got index=%d node=%v", index, envNode)
	}
}

func TestFindEnvNode_NonMappingReturnsNotFound(t *testing.T) {
	index, envNode := findEnvNode(yaml.NewCtx(), sequence(str("a")))
	if index != -1 || envNode != nil {
		t.Fatalf("expected not found on sequence node, got index=%d node=%v", index, envNode)
	}
}

func TestReplaceAnchorUsageNode(t *testing.T) {
	target := mapping(strPair("k", "v"))
	anchor := newAnchor("a", target)
	ctx := yaml.NewCtx()
	ctx.Anchors["a"] = target
	alias := newAliasNode("a")
	root := mapping(pair(str("ref"), alias))

	replaced := replaceAnchorUsageNode(ctx, root, target)
	if !replaced {
		t.Fatalf("expected replacement to succeed")
	}
	if root.Values[0].Value != target {
		t.Fatalf("expected alias to be replaced with the anchor's value node")
	}
	_ = anchor
}

func TestReplaceAnchorUsageNode_NotFound(t *testing.T) {
	target := mapping(strPair("k", "v"))
	root := mapping(strPair("ref", "plain"))

	replaced := replaceAnchorUsageNode(yaml.NewCtx(), root, target)
	if replaced {
		t.Fatalf("expected no replacement when anchor is not referenced")
	}
}

// --- convertSequenceToMapping / convertMappingToSequence ---

func TestConvertSequenceToMapping(t *testing.T) {
	seq := sequence(str("A=1"), str("B=2"), str("C"))
	got := convertSequenceToMapping(yaml.NewCtx(), seq, "=")

	want := []string{"A", "1", "B", "2", "C", ""}
	if len(got.Values)*2 != len(want) {
		t.Fatalf("got %d pairs, want %d values: %#v", len(got.Values), len(want), got.Values)
	}
	for i, mv := range got.Values {
		if yaml.ScalarText(mv.Key) != want[i*2] || yaml.ScalarText(mv.Value) != want[i*2+1] {
			t.Fatalf("pair[%d] = (%q,%q), want (%q,%q)", i, yaml.ScalarText(mv.Key), yaml.ScalarText(mv.Value), want[i*2], want[i*2+1])
		}
	}
	if got.Values[2].Value.Type() != yaml.NullType {
		t.Fatalf("expected null value for valueless entry, got %v", got.Values[2].Value.Type())
	}
}

func TestConvertSequenceToMapping_FlattensAliasElement(t *testing.T) {
	aliasTarget := sequence(str("X=1"))
	ctx := yaml.NewCtx()
	ctx.Anchors["extra"] = aliasTarget
	aliasNode := newAliasNode("extra")
	seq := sequence(aliasNode, str("Y=2"))

	got := convertSequenceToMapping(ctx, seq, "=")

	want := []string{"X", "1", "Y", "2"}
	if len(got.Values)*2 != len(want) {
		t.Fatalf("got %d pairs, want %d values: %#v", len(got.Values), len(want), got.Values)
	}
	for i, mv := range got.Values {
		if yaml.ScalarText(mv.Key) != want[i*2] || yaml.ScalarText(mv.Value) != want[i*2+1] {
			t.Fatalf("pair[%d] = (%q,%q), want (%q,%q)", i, yaml.ScalarText(mv.Key), yaml.ScalarText(mv.Value), want[i*2], want[i*2+1])
		}
	}
}

func TestConvertMappingToSequence(t *testing.T) {
	m := mapping(strPair("A", "1"), strPair("B", "2"), pair(str("C"), nullNode()))
	got := convertMappingToSequence(yaml.NewCtx(), m, "=")

	want := []string{"A=1", "B=2", "C"}
	if len(got.Values) != len(want) {
		t.Fatalf("got %d items, want %d: %#v", len(got.Values), len(want), got.Values)
	}
	for i, n := range got.Values {
		if yaml.ScalarText(n) != want[i] {
			t.Fatalf("content[%d] = %q, want %q", i, yaml.ScalarText(n), want[i])
		}
	}
}

func TestConvertMappingToSequence_FlattensAliasElement(t *testing.T) {
	// convertMappingToSequence checks whether the VALUE of a pair is an
	// alias (not the key); when it is, the alias's target mapping is
	// recursively flattened and its items spliced in, rather than emitting
	// a single "key=<alias>" entry.
	aliasTarget := mapping(strPair("X", "1"))
	ctx := yaml.NewCtx()
	ctx.Anchors["extra"] = aliasTarget
	aliasNode := newAliasNode("extra")
	m := mapping(pair(str("extra"), aliasNode), strPair("Y", "2"))
	got := convertMappingToSequence(ctx, m, "=")

	want := []string{"X=1", "Y=2"}
	if len(got.Values) != len(want) {
		t.Fatalf("got %d items, want %d: %#v", len(got.Values), len(want), got.Values)
	}
	for i, n := range got.Values {
		if yaml.ScalarText(n) != want[i] {
			t.Fatalf("content[%d] = %q, want %q", i, yaml.ScalarText(n), want[i])
		}
	}
}

// --- sequenceNodeToStrings ---

func TestSequenceNodeToStrings(t *testing.T) {
	seq := sequence(str("x"), str("y"), str("z"))
	got := sequenceNodeToStrings(yaml.NewCtx(), seq)
	want := []string{"x", "y", "z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestSequenceNodeToStrings_Empty(t *testing.T) {
	got := sequenceNodeToStrings(yaml.NewCtx(), sequence())
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %#v", got)
	}
}

// --- filterNodeByKeys ---

func TestFilterNodeByKeys_IncludeOnly(t *testing.T) {
	m := mapping(strPair("a", "1"), strPair("b", "2"), strPair("c", "3"))
	got := filterNodeByKeys(yaml.NewCtx(), m, []string{"a", "c"}, nil)
	if keys := mapKeys(got); !reflect.DeepEqual(keys, []string{"a", "c"}) {
		t.Fatalf("got keys %#v", keys)
	}
}

func TestFilterNodeByKeys_ExcludeOnly(t *testing.T) {
	m := mapping(strPair("a", "1"), strPair("b", "2"), strPair("c", "3"))
	got := filterNodeByKeys(yaml.NewCtx(), m, nil, []string{"b"})
	if keys := mapKeys(got); !reflect.DeepEqual(keys, []string{"a", "c"}) {
		t.Fatalf("got keys %#v", keys)
	}
}

func TestFilterNodeByKeys_IncludeThenExclude(t *testing.T) {
	m := mapping(strPair("a", "1"), strPair("b", "2"), strPair("c", "3"))
	got := filterNodeByKeys(yaml.NewCtx(), m, []string{"a", "b"}, []string{"b"})
	if keys := mapKeys(got); !reflect.DeepEqual(keys, []string{"a"}) {
		t.Fatalf("got keys %#v", keys)
	}
}

func TestFilterNodeByKeys_ResolvesAlias(t *testing.T) {
	target := mapping(strPair("a", "1"))
	ctx := yaml.NewCtx()
	ctx.Anchors["x"] = target
	alias := newAliasNode("x")
	got := filterNodeByKeys(ctx, alias, nil, nil)
	if keys := mapKeys(got); !reflect.DeepEqual(keys, []string{"a"}) {
		t.Fatalf("got keys %#v", keys)
	}
}

func TestFilterNodeByKeys_NonMappingPassthrough(t *testing.T) {
	scalar := str("hello")
	got := filterNodeByKeys(yaml.NewCtx(), scalar, []string{"a"}, nil)
	if got != yaml.Node(scalar) {
		t.Fatalf("expected scalar node returned unchanged")
	}
}

// --- extendNode ---

func TestExtendNode_OverridesAndAppends(t *testing.T) {
	root := mapping(strPair("a", "1"), strPair("b", "2"))
	local := mapping(strPair("b", "20"), strPair("c", "30"))

	extendNode(yaml.NewCtx(), root, local)

	got := map[string]string{}
	for _, mv := range root.Values {
		got[yaml.ScalarText(mv.Key)] = yaml.ScalarText(mv.Value)
	}
	want := map[string]string{"a": "1", "b": "20", "c": "30"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestExtendNode_WholeValueReplacementNotDeepMerge(t *testing.T) {
	root := mapping(pair(str("env"), mapping(strPair("A", "1"), strPair("B", "2"))))
	local := mapping(pair(str("env"), mapping(strPair("A", "3"))))

	extendNode(yaml.NewCtx(), root, local)

	_, envNode := findKeyValue(root, "env")
	envMap := envNode.(*yaml.MappingNode)
	if len(envMap.Values) != 1 {
		t.Fatalf("expected nested env value fully replaced (1 key), got %#v", envMap.Values)
	}
	if yaml.ScalarText(envMap.Values[0].Key) != "A" || yaml.ScalarText(envMap.Values[0].Value) != "3" {
		t.Fatalf("expected env to be exactly {A: 3} after whole-value replace, got %#v", envMap.Values)
	}
}

func TestExtendNode_ResolvesAliasNode(t *testing.T) {
	localTarget := mapping(strPair("c", "30"))
	ctx := yaml.NewCtx()
	ctx.Anchors["l"] = localTarget
	localAlias := newAliasNode("l")

	root := mapping(strPair("a", "1"))
	extendNode(ctx, root, localAlias)

	_, cNode := findKeyValue(root, "c")
	if cNode == nil || yaml.ScalarText(cNode) != "30" {
		t.Fatalf("expected key c=30 merged via alias-resolved node, got %#v", cNode)
	}
}

func TestExtendNode_ResolvesAliasValue(t *testing.T) {
	valueTarget := str("99")
	ctx := yaml.NewCtx()
	ctx.Anchors["v"] = valueTarget
	valueAlias := newAliasNode("v")
	local := mapping(pair(str("d"), valueAlias))

	root := mapping(strPair("a", "1"))
	extendNode(ctx, root, local)

	_, dNode := findKeyValue(root, "d")
	if dNode != yaml.Node(valueTarget) {
		t.Fatalf("expected value node replaced with the alias target pointer")
	}
}

func TestExtendNode_NonMappingNodeIsNoop(t *testing.T) {
	root := mapping(strPair("a", "1"))
	before := len(root.Values)
	extendNode(yaml.NewCtx(), root, str("scalar"))
	if len(root.Values) != before {
		t.Fatalf("expected no change when node is not a mapping")
	}
}

func findKeyValue(node *yaml.MappingNode, key string) (int, yaml.Node) {
	for i, mv := range node.Values {
		if yaml.ScalarText(mv.Key) == key {
			return i, mv.Value
		}
	}
	return -1, nil
}

// --- appendNode ---

func TestAppendNode_AppendsNewKeyAtEnd(t *testing.T) {
	root := mapping(strPair("a", "1"))
	toAppend := mapping(strPair("b", "2"))

	appendNode(yaml.NewCtx(), root, toAppend, -1)

	if keys := mapKeys(root); !reflect.DeepEqual(keys, []string{"a", "b"}) {
		t.Fatalf("got keys %#v", keys)
	}
}

func TestAppendNode_InsertsAtIndex(t *testing.T) {
	root := mapping(strPair("a", "1"), strPair("c", "3"))
	toInsert := mapping(strPair("b", "2"))

	appendNode(yaml.NewCtx(), root, toInsert, 1)

	if keys := mapKeys(root); !reflect.DeepEqual(keys, []string{"a", "b", "c"}) {
		t.Fatalf("got keys %#v", keys)
	}
}

func TestAppendNode_MergesIntoExistingNestedKey(t *testing.T) {
	root := mapping(pair(str("env"), mapping(strPair("FOO", "bar"))))
	toAppend := mapping(pair(str("env"), mapping(strPair("BAZ", "qux"))))

	appendNode(yaml.NewCtx(), root, toAppend, -1)

	_, envNode := findKeyValue(root, "env")
	envMap := envNode.(*yaml.MappingNode)
	got := map[string]string{}
	for _, mv := range envMap.Values {
		got[yaml.ScalarText(mv.Key)] = yaml.ScalarText(mv.Value)
	}
	want := map[string]string{"FOO": "bar", "BAZ": "qux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestAppendNode_OverwritesExistingScalarInPlace(t *testing.T) {
	root := mapping(pair(str("env"), mapping(strPair("FOO", "bar"))))
	toAppend := mapping(pair(str("env"), mapping(strPair("FOO", "newval"))))

	appendNode(yaml.NewCtx(), root, toAppend, -1)

	_, envNode := findKeyValue(root, "env")
	envMap := envNode.(*yaml.MappingNode)
	_, fooNode := findKeyValue(envMap, "FOO")
	if yaml.ScalarText(fooNode) != "newval" {
		t.Fatalf("expected FOO overwritten to 'newval', got %q", yaml.ScalarText(fooNode))
	}
}

func TestAppendNode_ResolvesAlias(t *testing.T) {
	target := mapping(strPair("b", "2"))
	ctx := yaml.NewCtx()
	ctx.Anchors["x"] = target
	alias := newAliasNode("x")
	root := mapping(strPair("a", "1"))

	appendNode(ctx, root, alias, -1)

	if keys := mapKeys(root); !reflect.DeepEqual(keys, []string{"a", "b"}) {
		t.Fatalf("got keys %#v", keys)
	}
}

// --- addAnchors ---

func TestAddAnchors_SharedPointerGetsAnchorAndAlias(t *testing.T) {
	shared := mapping(strPair("x", "1"))
	root := mapping(
		pair(str("svcA"), shared),
		pair(str("svcB"), shared),
	)

	addAnchors(root)

	first := root.Values[0].Value
	second := root.Values[1].Value

	anc, ok := first.(*yaml.AnchorNode)
	if !ok {
		t.Fatalf("expected first occurrence wrapped in an anchor node, got %T", first)
	}
	if anc.Value != yaml.Node(shared) {
		t.Fatalf("expected anchor to wrap the original shared node")
	}
	if yaml.AnchorName(anc) == "" {
		t.Fatalf("expected anchor name set on first occurrence")
	}

	al, ok := second.(*yaml.AliasNode)
	if !ok {
		t.Fatalf("expected second occurrence replaced with an alias node, got %T", second)
	}
	if al.Value.GetToken().Value != yaml.AnchorName(anc) {
		t.Fatalf("expected alias name %q to match anchor %q", al.Value.GetToken().Value, yaml.AnchorName(anc))
	}
}

func TestAddAnchors_UniqueNodesUntouched(t *testing.T) {
	root := mapping(
		pair(str("svcA"), mapping(strPair("x", "1"))),
		pair(str("svcB"), mapping(strPair("y", "2"))),
	)

	addAnchors(root)

	if _, ok := root.Values[0].Value.(*yaml.AnchorNode); ok {
		t.Fatalf("expected no anchor set for non-shared node svcA")
	}
	if _, ok := root.Values[1].Value.(*yaml.AnchorNode); ok {
		t.Fatalf("expected no anchor set for non-shared node svcB")
	}
}
