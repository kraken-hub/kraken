package config

import (
	"strings"
	"testing"
)

func TestRunJS_MergesLocalOverService(t *testing.T) {
	code := `({ local, service }) => ({ ...service, ...local })`
	input := `{ "local": {"a": 1}, "service": {"a": 0, "b": 2} }`

	got, err := RunJS(code, input)
	if err != nil {
		t.Fatalf("RunJS failed: %v", err)
	}

	want := `{"a":1,"b":2}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRunJS_ReturnsServiceUnmodified(t *testing.T) {
	code := `({ local, service }) => service`
	input := `{ "local": {}, "service": {"image": "nginx"} }`

	got, err := RunJS(code, input)
	if err != nil {
		t.Fatalf("RunJS failed: %v", err)
	}

	want := `{"image":"nginx"}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRunJS_InvalidSyntaxReturnsError(t *testing.T) {
	_, err := RunJS(`this is not valid js (((`, `{}`)
	if err == nil {
		t.Fatalf("expected an error for invalid JS syntax")
	}
}

func TestRunJS_ThrowingFunctionReturnsError(t *testing.T) {
	code := `() => { throw new Error("boom") }`
	_, err := RunJS(code, `{}`)
	if err == nil {
		t.Fatalf("expected an error when the JS function throws")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error to mention 'boom', got %v", err)
	}
}

func TestRunJS_ArraysPreserved(t *testing.T) {
	code := `({ local, service }) => ({ list: [...service.list, ...local.list] })`
	input := `{ "local": {"list": [3]}, "service": {"list": [1,2]} }`

	got, err := RunJS(code, input)
	if err != nil {
		t.Fatalf("RunJS failed: %v", err)
	}

	want := `{"list":[1,2,3]}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
