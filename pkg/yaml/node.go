package yaml

import (
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
)

// Type aliases over goccy's ast node types, so that callers outside this
// package never need to import github.com/goccy/go-yaml/ast (or its
// sibling token/parser packages) directly - every YAML node type they need
// is reachable through this package alone. These are Go type aliases
// (`type X = ast.X`), not new types: identity, method sets, and type
// assertions/switches against the underlying ast types all behave exactly
// as if ast.X had been spelled directly.
type (
	Node             = ast.Node
	MapKeyNode       = ast.MapKeyNode
	MappingNode      = ast.MappingNode
	MappingValueNode = ast.MappingValueNode
	SequenceNode     = ast.SequenceNode
	DocumentNode     = ast.DocumentNode
	StringNode       = ast.StringNode
	NullNode         = ast.NullNode
	AnchorNode       = ast.AnchorNode
	AliasNode        = ast.AliasNode
	TagNode          = ast.TagNode
)

// NullType mirrors ast.NullType, for `node.Type() == yaml.NullType` checks
// without importing ast directly.
const NullType = ast.NullType

// Ctx carries anchor-name -> target-node resolution state for a single
// parsed document. goccy's ast package does not resolve *alias references
// back to the node their anchor annotates (that map is private to its
// decoder), so callers build and carry one themselves via Ctx.
type Ctx struct {
	Anchors map[string]ast.Node
}

// NewCtx returns an empty Ctx.
func NewCtx() *Ctx {
	return &Ctx{Anchors: map[string]ast.Node{}}
}

// Index walks root and records every anchor name -> value-node mapping
// found anywhere in the tree (including nested anchors).
func (c *Ctx) Index(root ast.Node) {
	if root == nil {
		return
	}
	for _, n := range ast.Filter(ast.AnchorType, root) {
		a := n.(*ast.AnchorNode)
		c.Anchors[a.Name.GetToken().Value] = a.Value
	}
}

// Resolve unwraps DocumentNode/AnchorNode/AliasNode wrapper nodes down to
// the underlying value node. Returns the alias node itself if its anchor
// name isn't known (unresolved reference).
func Resolve(ctx *Ctx, n ast.Node) ast.Node {
	for n != nil {
		switch t := n.(type) {
		case *ast.DocumentNode:
			n = t.Body
		case *ast.AnchorNode:
			n = t.Value
		case *ast.AliasNode:
			name := t.Value.GetToken().Value
			target, ok := ctx.Anchors[name]
			if !ok {
				return n
			}
			n = target
		default:
			return n
		}
	}
	return n
}

// IsAlias reports whether n is an (unresolved) alias reference node.
func IsAlias(n ast.Node) bool {
	_, ok := n.(*ast.AliasNode)
	return ok
}

// AnchorName returns the anchor name if n is directly an AnchorNode
// wrapper, or "" otherwise.
func AnchorName(n ast.Node) string {
	if a, ok := n.(*ast.AnchorNode); ok {
		return a.Name.GetToken().Value
	}
	return ""
}

// IsMergeKey reports whether n is a `<<` merge-key node.
func IsMergeKey(n ast.Node) bool {
	return n != nil && n.Type() == ast.MergeKeyType
}

// IsScalar reports whether n is one of the plain scalar node types
// (string/int/float/bool/null/literal/infinity/nan).
func IsScalar(n ast.Node) bool {
	switch n.(type) {
	case *ast.StringNode, *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode,
		*ast.NullNode, *ast.LiteralNode, *ast.InfinityNode, *ast.NanNode:
		return true
	}
	return false
}

// ScalarText returns the literal text of a scalar-like node - the
// analogue of yaml.v3's Node.Value field for scalars. Transparent to a
// TagNode wrapper (e.g. `!!js/function`).
func ScalarText(n ast.Node) string {
	switch t := n.(type) {
	case nil:
		return ""
	case *ast.StringNode:
		return t.Value
	case *ast.LiteralNode:
		return t.Value.Value
	case *ast.NullNode:
		return ""
	case *ast.TagNode:
		return ScalarText(t.Value)
	default:
		if tok := t.GetToken(); tok != nil {
			return tok.Value
		}
		return ""
	}
}

// ShortTag mirrors yaml.v3's Node.ShortTag(): the resolved !!type tag for
// built-in kinds, or the literal custom tag (e.g. !!js/function) if n is a
// TagNode.
func ShortTag(n ast.Node) string {
	switch t := n.(type) {
	case *ast.TagNode:
		return t.Start.Value
	case *ast.StringNode:
		return "!!str"
	case *ast.IntegerNode:
		return "!!int"
	case *ast.FloatNode:
		return "!!float"
	case *ast.BoolNode:
		return "!!bool"
	case *ast.NullNode:
		return "!!null"
	case *ast.MappingNode:
		return "!!map"
	case *ast.SequenceNode:
		return "!!seq"
	case *ast.MergeKeyNode:
		return "!!merge"
	}
	return ""
}

// MapPairs resolves n and returns its mapping pairs, or nil if n does not
// resolve to a mapping node.
func MapPairs(ctx *Ctx, n ast.Node) []*ast.MappingValueNode {
	m, ok := Resolve(ctx, n).(*ast.MappingNode)
	if !ok {
		return nil
	}
	return m.Values
}

// SeqItems resolves n and returns its sequence entries, or nil if n does
// not resolve to a sequence node.
func SeqItems(ctx *Ctx, n ast.Node) []ast.Node {
	s, ok := Resolve(ctx, n).(*ast.SequenceNode)
	if !ok {
		return nil
	}
	return s.Values
}

// FindPair returns the index and pair for the given key within a mapping
// node (after resolving aliases/anchors), or (-1, nil) if not found.
func FindPair(ctx *Ctx, n ast.Node, key string) (int, *ast.MappingValueNode) {
	for i, v := range MapPairs(ctx, n) {
		if ScalarText(v.Key) == key {
			return i, v
		}
	}
	return -1, nil
}

// FindValue returns the index and value node for the given key.
func FindValue(ctx *Ctx, n ast.Node, key string) (int, ast.Node) {
	i, mv := FindPair(ctx, n, key)
	if mv == nil {
		return -1, nil
	}
	return i, mv.Value
}

// RemovePair removes the mapping pair at index i from m.
func RemovePair(m *ast.MappingNode, i int) {
	m.Values = append(m.Values[:i], m.Values[i+1:]...)
}

// NewString builds a new, standalone string scalar node with the given
// literal text, suitable for splicing into a mapping/sequence.
func NewString(v string) *ast.StringNode {
	return ast.String(token.String(v, v, &token.Position{Line: 1, Column: 1}))
}

// NewNull builds a new, standalone null scalar node.
func NewNull() *ast.NullNode {
	return ast.Null(token.New("null", "null", &token.Position{Line: 1, Column: 1}))
}

// NewMapping builds a new, empty block-style mapping node.
func NewMapping() *ast.MappingNode {
	return ast.Mapping(&token.Token{Type: token.MappingStartType, Position: &token.Position{Line: 1, Column: 1}}, false)
}

// NewSequence builds a new, empty block-style sequence node.
func NewSequence() *ast.SequenceNode {
	return ast.Sequence(&token.Token{Type: token.SequenceStartType, Position: &token.Position{Line: 1, Column: 1}}, false)
}

// NewPair builds a new mapping-value pair from a key/value node.
func NewPair(key ast.MapKeyNode, value ast.Node) *ast.MappingValueNode {
	return ast.MappingValue(&token.Token{Type: token.MappingValueType, Position: &token.Position{Line: 1, Column: 1}}, key, value)
}

// NewAnchor wraps child in a new anchor node named name (e.g. `&name`).
func NewAnchor(child ast.Node, name string) *ast.AnchorNode {
	pos := &token.Position{Line: 1, Column: 1}
	a := ast.Anchor(&token.Token{Type: token.AnchorType, Value: "&" + name, Position: pos})
	a.Name = NewString(name)
	a.Value = child
	return a
}

// NewAliasNode builds a standalone alias node referencing name (e.g.
// `*name`), without requiring the anchor node itself.
func NewAliasNode(name string) *ast.AliasNode {
	pos := &token.Position{Line: 1, Column: 1}
	al := ast.Alias(&token.Token{Type: token.AliasType, Value: "*" + name, Position: pos})
	al.Value = NewString(name)
	return al
}

// NewAliasTo builds an alias node referencing anc.
func NewAliasTo(anc *ast.AnchorNode) *ast.AliasNode {
	return NewAliasNode(anc.Name.(*ast.StringNode).Value)
}

// NewTag wraps value in a new tag node carrying the given YAML tag (e.g.
// "!!js/function").
func NewTag(value ast.Node, tag string) *ast.TagNode {
	tn := ast.Tag(&token.Token{Type: token.TagType, Value: tag, Position: &token.Position{Line: 1, Column: 1}})
	tn.Value = value
	return tn
}

// AlignColumn returns n repositioned to render at column col. If n already
// renders at col, n is returned unchanged - this is the common case (e.g.
// two services at the same nesting depth, one extending the other) and it
// matters: n may be intentionally shared with another live part of the
// tree (filterNodeByKeys shallow-copies a base service's pairs so
// addAnchors can detect and compact the sharing as &anchor/*alias in the
// output), and mutating a shared node's token positions would corrupt
// wherever else it's still rendered from. When a real shift is needed, a
// deep clone is aligned and returned instead of mutating n in place, for
// exactly that reason - block-style YAML output is column-driven (each
// node renders its own leading whitespace from its own token position), so
// any node built or relocated outside of a real parse needs realigning to
// its new position or the resulting text is not valid YAML, but that must
// never happen to the original if it's still referenced elsewhere.
func AlignColumn(n ast.Node, col int) ast.Node {
	if n == nil {
		return nil
	}
	if Column(n) == col {
		return n
	}
	clone := CloneNode(n)
	if delta := col - Column(clone); delta != 0 {
		clone.AddColumn(delta)
	}
	return clone
}

// Column returns the rendered column of n. A mapping node's block-style
// rendering indents each pair from that pair's own key column (not from
// the mapping's own GetToken(), which for a bare reparsed document can
// point at an unrelated position) - so for a non-empty mapping this
// returns its first pair's key column. Sequence block-style rendering does
// use the sequence's own token directly, so that case (and everything
// else) falls back to n's own token, or 1 if nothing is known.
func Column(n ast.Node) int {
	if m, ok := n.(*ast.MappingNode); ok && len(m.Values) > 0 {
		return Column(m.Values[0].Key)
	}
	if n == nil {
		return 1
	}
	tok := n.GetToken()
	if tok == nil || tok.Position == nil {
		return 1
	}
	return tok.Position.Column
}

// ReplacePairValue replaces the value at m.Values[idx] with newValue,
// aligning it to render as a properly nested child of the pair's
// (unchanged) key. This replaces the slice slot with a freshly built pair
// rather than mutating the existing *ast.MappingValueNode in place, since
// that pair object may be shared with another mapping (e.g. a shallow
// copy produced by filterNodeByKeys) that must not see the change.
func ReplacePairValue(m *ast.MappingNode, idx int, newValue ast.Node) {
	old := m.Values[idx]
	if needsColumnAlignment(newValue) {
		newValue = AlignColumn(newValue, Column(old.Key)+childIndentStep(m))
	}
	m.Values[idx] = NewPair(old.Key, newValue)
}

// needsColumnAlignment reports whether n's own rendering depends on its
// stored column (a non-empty mapping/sequence indents its children from
// it; a literal block scalar indents its content from it) - a plain
// scalar renders inline on the "key: value" line regardless of its
// column, so aligning (and, worse, cloning) it would be both pointless
// and needlessly break its identity if it happens to be shared elsewhere.
func needsColumnAlignment(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.MappingNode:
		return len(v.Values) > 0
	case *ast.SequenceNode:
		return len(v.Values) > 0
	case *ast.LiteralNode:
		return true
	}
	return false
}

// AppendPair appends newPair to m, aligning its key to match m's existing
// sibling keys (column 1 if m is currently empty) and its value one level
// deeper.
func AppendPair(m *ast.MappingNode, newPair *ast.MappingValueNode) {
	insertPairAt(m, newPair, siblingColumn(m))
	m.Values = append(m.Values, newPair)
}

// InsertPairAt inserts newPair into m at position index, aligning it like
// AppendPair and preserving the relative order of existing entries.
func InsertPairAt(m *ast.MappingNode, newPair *ast.MappingValueNode, index int) {
	insertPairAt(m, newPair, siblingColumn(m))
	m.Values = append(m.Values, nil)
	copy(m.Values[index+1:], m.Values[index:])
	m.Values[index] = newPair
}

func insertPairAt(m *ast.MappingNode, newPair *ast.MappingValueNode, col int) {
	newPair.Key = AlignColumn(newPair.Key, col).(ast.MapKeyNode)
	if needsColumnAlignment(newPair.Value) {
		newPair.Value = AlignColumn(newPair.Value, col+childIndentStep(m))
	}
}

func siblingColumn(m *ast.MappingNode) int {
	if len(m.Values) == 0 {
		return 1
	}
	return Column(m.Values[0].Key)
}

// childIndentStep returns the indentation step m's own document region
// actually uses (2 spaces, 4 spaces, ...), inferred from any existing
// sibling pair whose value is itself a non-empty mapping/sequence.
// Assuming a fixed step (e.g. always +2) breaks on documents indented
// differently - this reads the real convention from the tree instead of
// guessing at it.
func childIndentStep(m *ast.MappingNode) int {
	const defaultStep = 2
	for _, mv := range m.Values {
		switch v := mv.Value.(type) {
		case *ast.MappingNode:
			if len(v.Values) > 0 {
				if step := Column(v.Values[0].Key) - Column(mv.Key); step > 0 {
					return step
				}
			}
		case *ast.SequenceNode:
			if len(v.Values) > 0 {
				if step := Column(v.Values[0]) - Column(mv.Key); step > 0 {
					return step
				}
			}
		}
	}
	return defaultStep
}

// CloneNode returns a deep copy of n: every node and token in its subtree
// is a fresh object, so mutating the clone (e.g. via AddColumn, through
// AlignColumn) can never affect n or anything else that might still
// reference the same nodes.
func CloneNode(n ast.Node) ast.Node {
	if n == nil {
		return nil
	}
	switch t := n.(type) {
	case *ast.NullNode:
		return &ast.NullNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone()}
	case *ast.IntegerNode:
		return &ast.IntegerNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone(), Value: t.Value}
	case *ast.FloatNode:
		return &ast.FloatNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone(), Precision: t.Precision, Value: t.Value}
	case *ast.StringNode:
		return &ast.StringNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone(), Value: t.Value}
	case *ast.BoolNode:
		return &ast.BoolNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone(), Value: t.Value}
	case *ast.InfinityNode:
		return &ast.InfinityNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone(), Value: t.Value}
	case *ast.NanNode:
		return &ast.NanNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone()}
	case *ast.MergeKeyNode:
		return &ast.MergeKeyNode{BaseNode: cloneBase(t.BaseNode), Token: t.Token.Clone()}
	case *ast.LiteralNode:
		var val *ast.StringNode
		if t.Value != nil {
			val = CloneNode(t.Value).(*ast.StringNode)
		}
		return &ast.LiteralNode{BaseNode: cloneBase(t.BaseNode), Start: t.Start.Clone(), Value: val}
	case *ast.MappingNode:
		clone := &ast.MappingNode{
			BaseNode:    cloneBase(t.BaseNode),
			Start:       t.Start.Clone(),
			End:         t.End.Clone(),
			IsFlowStyle: t.IsFlowStyle,
			FootComment: t.FootComment,
		}
		for _, mv := range t.Values {
			clone.Values = append(clone.Values, CloneNode(mv).(*ast.MappingValueNode))
		}
		return clone
	case *ast.MappingValueNode:
		var key ast.MapKeyNode
		if t.Key != nil {
			key = CloneNode(t.Key).(ast.MapKeyNode)
		}
		var value ast.Node
		if t.Value != nil {
			value = CloneNode(t.Value)
		}
		return &ast.MappingValueNode{
			BaseNode:     cloneBase(t.BaseNode),
			Start:        t.Start.Clone(),
			CollectEntry: t.CollectEntry.Clone(),
			Key:          key,
			Value:        value,
			FootComment:  t.FootComment,
			IsFlowStyle:  t.IsFlowStyle,
		}
	case *ast.SequenceNode:
		clone := &ast.SequenceNode{
			BaseNode:          cloneBase(t.BaseNode),
			Start:             t.Start.Clone(),
			End:               t.End.Clone(),
			IsFlowStyle:       t.IsFlowStyle,
			ValueHeadComments: t.ValueHeadComments,
			FootComment:       t.FootComment,
		}
		for _, item := range t.Values {
			clone.Values = append(clone.Values, CloneNode(item))
		}
		return clone
	case *ast.AnchorNode:
		var name, value ast.Node
		if t.Name != nil {
			name = CloneNode(t.Name)
		}
		if t.Value != nil {
			value = CloneNode(t.Value)
		}
		return &ast.AnchorNode{BaseNode: cloneBase(t.BaseNode), Start: t.Start.Clone(), Name: name, Value: value}
	case *ast.AliasNode:
		var value ast.Node
		if t.Value != nil {
			value = CloneNode(t.Value)
		}
		return &ast.AliasNode{BaseNode: cloneBase(t.BaseNode), Start: t.Start.Clone(), Value: value}
	case *ast.TagNode:
		var value ast.Node
		if t.Value != nil {
			value = CloneNode(t.Value)
		}
		return &ast.TagNode{BaseNode: cloneBase(t.BaseNode), Directive: t.Directive, Start: t.Start.Clone(), Value: value}
	case *ast.DocumentNode:
		var body ast.Node
		if t.Body != nil {
			body = CloneNode(t.Body)
		}
		return &ast.DocumentNode{BaseNode: cloneBase(t.BaseNode), Start: t.Start.Clone(), End: t.End.Clone(), Body: body}
	}
	return n
}

func cloneBase(b *ast.BaseNode) *ast.BaseNode {
	if b == nil {
		return &ast.BaseNode{}
	}
	return &ast.BaseNode{Path: b.Path, Comment: b.Comment}
}
