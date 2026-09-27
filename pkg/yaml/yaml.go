package yaml

import (
	"bytes"
	"encoding/json"

	goccyyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// Marshal encodes v as YAML source text.
func Marshal(v interface{}) ([]byte, error) {
	return goccyyaml.Marshal(v)
}

// Unmarshal decodes YAML source text into v.
func Unmarshal(data []byte, v interface{}) error {
	return goccyyaml.Unmarshal(data, v)
}

// Load parses YAML source into an ast.Node. Returns a *ast.DocumentNode
// wrapping the parsed content, or nil for empty input.
func Load(data []byte) (ast.Node, error) {
	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return nil, nil
	}
	return file.Docs[0], nil
}

// Dump renders n back to YAML source text.
func Dump(n ast.Node) ([]byte, error) {
	if n == nil {
		return nil, nil
	}
	return []byte(n.String() + "\n"), nil
}

// NodeManager provides yaml.v3-style Keys()/Get() navigation over a
// resolved ast.Node tree, transparently following aliases.
type NodeManager struct {
	Node ast.Node
	Ctx  *Ctx
}

func (m *NodeManager) ctx() *Ctx {
	if m.Ctx == nil {
		m.Ctx = NewCtx()
		m.Ctx.Index(m.Node)
	}
	return m.Ctx
}

func (m *NodeManager) Keys() []string {
	if _, ok := Resolve(m.ctx(), m.Node).(*ast.MappingNode); !ok {
		return nil
	}
	var keys []string
	for _, mv := range ExpandMergeNode(m.ctx(), m.Node).Values {
		keys = append(keys, ScalarText(mv.Key))
	}
	return keys
}

func (m *NodeManager) Get(k string) *NodeManager {
	if _, ok := Resolve(m.ctx(), m.Node).(*ast.MappingNode); !ok {
		return nil
	}
	_, v := FindValue(m.ctx(), ExpandMergeNode(m.ctx(), m.Node), k)
	if v == nil {
		return nil
	}
	return &NodeManager{Node: v, Ctx: m.ctx()}
}

func (m *NodeManager) JSON() ([]byte, error) {
	return json.Marshal(NodeToJSON{m.ctx(), m.Node})
}

func (m *NodeManager) JsonPrettyPrint() ([]byte, error) {
	b, err := m.JSON()
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	err = json.Indent(&out, b, "", "    ")
	return out.Bytes(), err
}
