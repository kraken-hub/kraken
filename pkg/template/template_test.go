package template

import (
	"os"
	"path/filepath"
	"testing"
)

func envFrom(vals map[string]string) func(string) string {
	return func(k string) string {
		return vals[k]
	}
}

func TestRenderTemplate(t *testing.T) {
	env := envFrom(map[string]string{"FOO": "bar", "EMPTY": ""})

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple var", "${FOO}", "bar"},
		{"missing var", "${MISSING}", ""},
		{"missing var with default", "${MISSING:-default}", "default"},
		{"empty var with colon-default", "${EMPTY:-default}", "default"},
		{"empty var with dash-default", "${EMPTY-default}", "default"},
		{"embedded in text", "prefix-${FOO}-suffix", "prefix-bar-suffix"},
		{"no substitution", "plain text", "plain text"},
		{"multiple vars", "${FOO}-${FOO}", "bar-bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderTemplate(tt.in, env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderTemplateBacktickEscaped(t *testing.T) {
	env := envFrom(map[string]string{})
	in := "literal `backtick` text"
	got, err := RenderTemplate(in, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != in {
		t.Errorf("expected backticks to remain literal, got %q", got)
	}
}

func TestRenderTemplateMalformedSyntax(t *testing.T) {
	_, err := RenderTemplate("${FOO", envFrom(nil))
	if err == nil {
		t.Errorf("expected error for unterminated ${, got nil")
	}
}

func TestRenderTemplateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tpl.txt")
	if err := os.WriteFile(path, []byte("hello ${NAME}!"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := RenderTemplateFile(path, envFrom(map[string]string{"NAME": "world"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world!" {
		t.Errorf("got %q", got)
	}
}

func TestRenderTemplateFileMissing(t *testing.T) {
	_, err := RenderTemplateFile(filepath.Join(t.TempDir(), "nope.txt"), envFrom(nil))
	if err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
}

func TestRenderTemplateFileMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(path, []byte("${UNCLOSED"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := RenderTemplateFile(path, envFrom(nil))
	if err == nil {
		t.Errorf("expected error for malformed template content, got nil")
	}
}
