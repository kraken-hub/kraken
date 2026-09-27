package yaml

import (
	"encoding/json"
	"math"

	"github.com/goccy/go-yaml/ast"
)

type NodeToJSON struct {
	ctx *Ctx
	n   ast.Node
}

func (node NodeToJSON) MarshalJSON() ([]byte, error) {
	return []byte(ConvertNodeToJson(node.ctx, node.n)), nil
}

// ConvertNodeToJson renders n as a JSON text. Mapping keys are rendered as
// JSON strings (a null key renders as ""); merge keys are expanded inline.
func ConvertNodeToJson(ctx *Ctx, n ast.Node) string {
	resolved := Resolve(ctx, n)

	switch t := resolved.(type) {
	case *ast.SequenceNode:
		s := "["
		if len(t.Values) > 0 {
			for _, item := range t.Values {
				s = s + ConvertNodeToJson(ctx, item) + ","
			}
			s = s[:len(s)-1]
		}
		return s + "]"
	case *ast.MappingNode:
		s := "{"
		expanded := ExpandMergeNode(ctx, t)
		if len(expanded.Values) > 0 {
			for _, mv := range expanded.Values {
				key := `"` + ScalarText(mv.Key) + `"`
				if mv.Key.Type() == ast.NullType {
					key = `""`
				}
				s = s + key + ":" + ConvertNodeToJson(ctx, mv.Value) + ","
			}
			s = s[:len(s)-1]
		}
		return s + "}"
	case *ast.StringNode:
		return quoteJSON(t.Value, t.Value)
	case *ast.LiteralNode:
		text := ScalarText(t)
		return quoteJSON(text, text)
	case *ast.IntegerNode:
		return quoteJSON(t.Value, ScalarText(t))
	case *ast.FloatNode:
		// JSON has no NaN/Infinity literal.
		if math.IsNaN(t.Value) || math.IsInf(t.Value, 0) {
			return quoteJSON(nil, ScalarText(t))
		}
		return quoteJSON(t.Value, ScalarText(t))
	case *ast.BoolNode:
		if t.Value {
			return "true"
		}
		return "false"
	case *ast.NullNode:
		return "null"
	case *ast.InfinityNode:
		return `"` + ScalarText(t) + `"`
	case *ast.NanNode:
		return `"` + ScalarText(t) + `"`
	}
	return "null"
}

// quoteJSON marshals v as JSON, falling back to quoting fallback as a
// plain JSON string if v can't be marshaled (or forces a fallback, e.g.
// NaN/Infinity, by passing a nil v).
func quoteJSON(v interface{}, fallback string) string {
	if v != nil {
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	b, err := json.Marshal(fallback)
	if err != nil {
		return `"` + fallback + `"`
	}
	return string(b)
}
