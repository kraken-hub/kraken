package util

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

type zipEntry struct {
	name    string
	content string
	mode    os.FileMode
	isDir   bool
}

func buildZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name}
		if e.isDir {
			hdr.Name += "/"
			hdr.SetMode(os.ModeDir | 0o755)
		} else {
			hdr.SetMode(e.mode)
		}
		hdr.Method = zip.Deflate
		fw, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if !e.isDir {
			if _, err := fw.Write([]byte(e.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractZip(t *testing.T) {
	t.Run("extracts flat files", func(t *testing.T) {
		data := buildZip(t, []zipEntry{
			{name: "a.txt", content: "hello", mode: 0o644},
			{name: "sub/b.txt", content: "world", mode: 0o644},
		})
		dest := t.TempDir()
		if err := ExtractZip(data, dest); err != nil {
			t.Fatalf("ExtractZip failed: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "a.txt"))
		if err != nil || string(got) != "hello" {
			t.Errorf("a.txt = %q, %v", got, err)
		}
		got, err = os.ReadFile(filepath.Join(dest, "sub", "b.txt"))
		if err != nil || string(got) != "world" {
			t.Errorf("sub/b.txt = %q, %v", got, err)
		}
	})

	t.Run("strips single shared top-level directory", func(t *testing.T) {
		data := buildZip(t, []zipEntry{
			{name: "my-plugin/", isDir: true},
			{name: "my-plugin/kraken.json", content: `{"name":"x"}`, mode: 0o644},
			{name: "my-plugin/main.js", content: "console.log(1)", mode: 0o644},
		})
		dest := t.TempDir()
		if err := ExtractZip(data, dest); err != nil {
			t.Fatalf("ExtractZip failed: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "my-plugin")); !os.IsNotExist(err) {
			t.Errorf("expected top-level prefix stripped, my-plugin dir exists")
		}
		got, err := os.ReadFile(filepath.Join(dest, "kraken.json"))
		if err != nil || string(got) != `{"name":"x"}` {
			t.Errorf("kraken.json = %q, %v", got, err)
		}
	})

	t.Run("preserves file permissions", func(t *testing.T) {
		data := buildZip(t, []zipEntry{
			{name: "run.sh", content: "#!/bin/sh\necho hi\n", mode: 0o755},
		})
		dest := t.TempDir()
		if err := ExtractZip(data, dest); err != nil {
			t.Fatalf("ExtractZip failed: %v", err)
		}
		info, err := os.Stat(filepath.Join(dest, "run.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("mode = %v, want 0755", info.Mode().Perm())
		}
	})

	t.Run("rejects path traversal via dot-dot segments", func(t *testing.T) {
		data := buildZip(t, []zipEntry{
			{name: "../../etc/passwd", content: "pwned", mode: 0o644},
		})
		dest := t.TempDir()
		err := ExtractZip(data, dest)
		if err == nil {
			t.Fatal("expected zip-slip to be rejected")
		}
	})

	t.Run("dot-dot traversal mixed with safe entries is rejected before escaping", func(t *testing.T) {
		data := buildZip(t, []zipEntry{
			{name: "safe.txt", content: "ok", mode: 0o644},
			{name: "../evil.txt", content: "pwned", mode: 0o644},
		})
		dest := t.TempDir()
		err := ExtractZip(data, dest)
		if err == nil {
			t.Fatal("expected zip-slip to be rejected")
		}
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); !os.IsNotExist(statErr) {
			t.Errorf("expected evil.txt not to be written outside destDir")
		}
	})

	t.Run("absolute path entries are contained within destDir", func(t *testing.T) {
		data := buildZip(t, []zipEntry{
			{name: "regular.txt", content: "ok", mode: 0o644},
			{name: "/etc/passwd", content: "contained", mode: 0o644},
		})
		dest := t.TempDir()
		if err := ExtractZip(data, dest); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(dest, "etc", "passwd"))
		if err != nil || string(got) != "contained" {
			t.Errorf("expected absolute-path entry contained under destDir, got %q, %v", got, err)
		}
		if _, statErr := os.Stat("/etc/passwd-should-not-exist"); !os.IsNotExist(statErr) {
			t.Errorf("unexpected file created outside destDir")
		}
	})

	t.Run("invalid zip data errors", func(t *testing.T) {
		if err := ExtractZip([]byte("not a zip"), t.TempDir()); err == nil {
			t.Fatal("expected error for invalid zip data")
		}
	})

	t.Run("empty zip produces no files", func(t *testing.T) {
		data := buildZip(t, nil)
		dest := t.TempDir()
		if err := ExtractZip(data, dest); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("expected empty dest, got %v", entries)
		}
	})
}

func TestZipTopLevelPrefix(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{"shared prefix", []string{"pkg/a.txt", "pkg/b/c.txt"}, "pkg/"},
		{"no shared prefix - root file", []string{"a.txt", "pkg/b.txt"}, ""},
		{"different top dirs", []string{"pkg1/a.txt", "pkg2/b.txt"}, ""},
		{"empty file list", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var files []*zip.File
			for _, name := range tt.files {
				files = append(files, &zip.File{FileHeader: zip.FileHeader{Name: name}})
			}
			got := zipTopLevelPrefix(files)
			if got != tt.want {
				t.Errorf("zipTopLevelPrefix(%v) = %q, want %q", tt.files, got, tt.want)
			}
		})
	}
}
