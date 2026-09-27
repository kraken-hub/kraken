package yaml

import (
	"github.com/goccy/go-yaml/ast"
)

type commentStripper struct{}

func (commentStripper) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	_ = n.SetComment(nil)
	switch t := n.(type) {
	case *ast.MappingNode:
		t.FootComment = nil
	case *ast.MappingValueNode:
		t.FootComment = nil
	case *ast.SequenceNode:
		t.FootComment = nil
	case *ast.SequenceEntryNode:
		t.HeadComment = nil
		t.LineComment = nil
	}
	return commentStripper{}
}

// RemoveAllComments strips every head/line/foot comment from n and all of
// its descendants.
func RemoveAllComments(n ast.Node) {
	if n == nil {
		return
	}
	ast.Walk(commentStripper{}, n)
}

func indexOfPair(m *ast.MappingNode, mv *ast.MappingValueNode) int {
	for i, v := range m.Values {
		if v == mv {
			return i
		}
	}
	return -1
}

// ExpandMergeNode returns a new mapping containing n's own pairs with any
// top-level `<<` merge keys expanded and flattened in. Keys present
// directly on n win over values supplied by a merge.
func ExpandMergeNode(ctx *Ctx, n ast.Node) *ast.MappingNode {
	result := NewMapping()
	if m, ok := Resolve(ctx, n).(*ast.MappingNode); ok {
		result.Values = append(result.Values, m.Values...)
	}

	for {
		doBreak := true
		for _, mv := range result.Values {
			if !IsMergeKey(mv.Key) {
				continue
			}

			tmp := NewMapping()
			MergeNodeExpander(ctx, tmp, mv.Value, -1)

			// Keys already present in result (i.e. declared directly on n,
			// not supplied by this merge) win over the merged-in value.
			for _, t := range tmp.Values {
				key := ScalarText(t.Key)
				if idx, existing := FindPair(ctx, result, key); existing != nil {
					RemovePair(result, idx)
					t.Value = existing.Value
				}
			}

			idx := indexOfPair(result, mv)
			RemovePair(result, idx)
			MergeNodeExpander(ctx, result, tmp, idx)

			doBreak = false
			break
		}
		if doBreak {
			break
		}
	}

	return result
}

// MergeNodeExpander merges node's key/value pairs into root, skipping any
// key already present in root. Merge keys (`<<`) nested in node are
// expanded recursively. If index is -1, new pairs are appended; otherwise
// they are inserted starting at that position, preserving their relative
// order.
func MergeNodeExpander(ctx *Ctx, root *ast.MappingNode, node ast.Node, index int) {
	switch t := Resolve(ctx, node).(type) {
	case *ast.SequenceNode:
		tmp := NewMapping()
		for _, item := range t.Values {
			MergeNodeExpander(ctx, tmp, item, -1)
		}
		MergeNodeExpander(ctx, root, tmp, index)
	case *ast.MappingNode:
		for _, mv := range t.Values {
			if IsMergeKey(mv.Key) {
				merged := ExpandMergeNode(ctx, mv.Value)
				MergeNodeExpander(ctx, root, merged, index)
				continue
			}

			key := ScalarText(mv.Key)
			if _, existing := FindPair(ctx, root, key); existing != nil {
				continue
			}
			if index == -1 {
				root.Values = append(root.Values, mv)
				continue
			}
			root.Values = append(root.Values, nil)
			copy(root.Values[index+1:], root.Values[index:])
			root.Values[index] = mv
			index++
		}
	}
}
