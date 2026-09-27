package yaml

import (
	"encoding/json"
	"testing"

	"github.com/goccy/go-yaml/ast"
)

func convertSrc(t *testing.T, src string) string {
	t.Helper()
	node, err := Load([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error loading %q: %v", src, err)
	}
	ctx := NewCtx()
	ctx.Index(node)
	return ConvertNodeToJson(ctx, node)
}

func TestConvertNodeToJsonScalars(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"int", "a: 1\n", `{"a":1}`},
		{"float", "a: 1.5\n", `{"a":1.5}`},
		{"string", "a: hello\n", `{"a":"hello"}`},
		{"bool true", "a: true\n", `{"a":true}`},
		{"bool false", "a: false\n", `{"a":false}`},
		{"null tilde", "a: ~\n", `{"a":null}`},
		{"null keyword", "a: null\n", `{"a":null}`},
		{"quoted string with special chars", "a: \"he said \\\"hi\\\"\"\n", `{"a":"he said \"hi\""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSrc(t, tt.src)
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestConvertNodeToJsonSequence(t *testing.T) {
	got := convertSrc(t, "a:\n  - 1\n  - 2\n  - 3\n")
	want := `{"a":[1,2,3]}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestConvertNodeToJsonEmptySequence(t *testing.T) {
	got := convertSrc(t, "a: []\n")
	want := `{"a":[]}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestConvertNodeToJsonEmptyMapping(t *testing.T) {
	got := convertSrc(t, "a: {}\n")
	want := `{"a":{}}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestConvertNodeToJsonNestedMapping(t *testing.T) {
	got := convertSrc(t, "a:\n  b:\n    c: 1\n")
	want := `{"a":{"b":{"c":1}}}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestConvertNodeToJsonAlias(t *testing.T) {
	got := convertSrc(t, "a: &x 1\nb: *x\n")
	want := `{"a":1,"b":1}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestConvertNodeToJsonMergeKey(t *testing.T) {
	src := "base: &base\n  a: 1\n  b: 2\nderived:\n  <<: *base\n  b: 3\n"
	node, err := Load([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := &NodeManager{Node: node}
	derived := m.Get("derived")
	if derived == nil {
		t.Fatalf("expected to find 'derived' key")
	}
	got := ConvertNodeToJson(m.ctx(), derived.Node)
	want := `{"a":1,"b":3}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestConvertNodeToJsonNullKey(t *testing.T) {
	got := convertSrc(t, "~: hello\n")
	want := `{"":"hello"}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

// goccy recognizes a fixed set of Infinity/NaN spellings at scan time
// (.inf/.Inf/.INF, -.inf/-.Inf/-.INF, .nan/.NaN/.NAN) as dedicated
// InfinityNode/NanNode types rather than resolving arbitrary spellings to a
// float64 the way yaml.v3 did; any of those recognized spellings quote to
// their own source text since JSON has no Infinity/NaN literal.
func TestConvertNodeToJsonInfAndNan(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"positive inf", "a: .inf\n", `{"a":".inf"}`},
		{"negative inf", "a: -.inf\n", `{"a":"-.inf"}`},
		{"canonical nan", "a: .NaN\n", `{"a":".NaN"}`},
		{"lowercase nan", "a: .nan\n", `{"a":".nan"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSrc(t, tt.src)
			if got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestConvertNodeToJsonDocumentWrapper(t *testing.T) {
	node, err := Load([]byte("a: 1\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	doc, ok := node.(*ast.DocumentNode)
	if !ok {
		t.Fatalf("expected DocumentNode, got %T", node)
	}
	if _, ok := doc.Body.(*ast.MappingNode); !ok {
		t.Fatalf("expected document node to wrap a distinct mapping node")
	}
	got := ConvertNodeToJson(NewCtx(), node)
	want := `{"a":1}`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestNodeToJSONMarshalJSON(t *testing.T) {
	node, err := Load([]byte("a: 1\nb: two\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wrapper := NodeToJSON{ctx: NewCtx(), n: node}
	b, err := wrapper.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"a":1,"b":"two"}`
	if string(b) != want {
		t.Fatalf("expected %s, got %s", want, string(b))
	}
}

func TestNodeToJSONViaStdlibMarshal(t *testing.T) {
	node, err := Load([]byte("a: 1\nb: two\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := json.Marshal(NodeToJSON{ctx: NewCtx(), n: node})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"a":1,"b":"two"}`
	if string(b) != want {
		t.Fatalf("expected %s, got %s", want, string(b))
	}
}
