package util

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchURL(t *testing.T) {
	t.Run("successful fetch returns body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("hello world"))
		}))
		defer srv.Close()

		data, err := FetchURL(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != "hello world" {
			t.Errorf("data = %q, want %q", data, "hello world")
		}
	})

	t.Run("non-200 status returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
		}))
		defer srv.Close()

		_, err := FetchURL(srv.URL)
		if err == nil {
			t.Fatal("expected error for non-200 status")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error = %v, want it to mention 404", err)
		}
	})

	t.Run("unreachable url returns error", func(t *testing.T) {
		_, err := FetchURL("http://127.0.0.1:1/does-not-exist")
		if err == nil {
			t.Fatal("expected error for unreachable url")
		}
	})

	t.Run("malformed url returns error", func(t *testing.T) {
		_, err := FetchURL("://not-a-url")
		if err == nil {
			t.Fatal("expected error for malformed url")
		}
	})
}
