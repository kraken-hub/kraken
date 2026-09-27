package docker

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type tarEntry struct {
	name string
	body string
	mode int64
}

func makeTar(t *testing.T, entries []tarEntry) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0644
		}
		hdr := &tar.Header{
			Name: e.name,
			Mode: mode,
			Size: int64(len(e.body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%s): %v", e.name, err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatalf("Write(%s): %v", e.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}
	return io.NopCloser(&buf)
}

func readTar(t *testing.T, r io.Reader) []tarEntry {
	t.Helper()
	var out []tarEntry
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		var body bytes.Buffer
		if _, err := io.Copy(&body, tr); err != nil {
			t.Fatalf("tar body read: %v", err)
		}
		out = append(out, tarEntry{name: hdr.Name, body: body.String(), mode: hdr.Mode})
	}
	return out
}

func tarNames(entries []tarEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

func TestCreateBuildContext(t *testing.T) {
	rc := CreateBuildContext()
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty build context, got %d bytes", len(data))
	}
}

func TestAppendFileToBuildContext(t *testing.T) {
	base := CreateBuildContext()
	file := BuildContextFile{Name: "hello.txt", Body: "hello world", Mode: 0644}

	rc := AppendFileToBuildContext(file, base)
	defer rc.Close()

	entries := readTar(t, rc)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d (%v)", len(entries), tarNames(entries))
	}
	if entries[0].name != "hello.txt" {
		t.Errorf("name = %q, want %q", entries[0].name, "hello.txt")
	}
	if entries[0].body != "hello world" {
		t.Errorf("body = %q, want %q", entries[0].body, "hello world")
	}
	if entries[0].mode != 0644 {
		t.Errorf("mode = %o, want %o", entries[0].mode, 0644)
	}
}

func TestAppendFileToBuildContext_NestedPath(t *testing.T) {
	base := CreateBuildContext()
	file := BuildContextFile{Name: "sub/dir/config.json", Body: "{}", Mode: 0600}

	rc := AppendFileToBuildContext(file, base)
	defer rc.Close()

	entries := readTar(t, rc)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].name != "sub/dir/config.json" {
		t.Errorf("name = %q, want %q", entries[0].name, "sub/dir/config.json")
	}
}

func TestAppendFilesToBuildContext_PreservesOrder(t *testing.T) {
	base := CreateBuildContext()
	files := []BuildContextFile{
		{Name: "a.txt", Body: "A", Mode: 0644},
		{Name: "b.txt", Body: "B", Mode: 0644},
		{Name: "c.txt", Body: "C", Mode: 0644},
	}

	rc := AppendFilesToBuildContext(files, base)
	defer rc.Close()

	entries := readTar(t, rc)
	wantNames := []string{"a.txt", "b.txt", "c.txt"}
	gotNames := tarNames(entries)
	if len(gotNames) != len(wantNames) {
		t.Fatalf("got %d entries, want %d (%v)", len(gotNames), len(wantNames), gotNames)
	}
	for i, name := range wantNames {
		if gotNames[i] != name {
			t.Errorf("entry[%d] = %q, want %q", i, gotNames[i], name)
		}
	}
}

// When two BuildContextFile entries share the same Name, the later one
// overwrites the earlier entry in place (at its original position) rather
// than appending a duplicate.
func TestAppendFilesToBuildContext_DuplicateNameOverwritesInPlace(t *testing.T) {
	base := CreateBuildContext()
	files := []BuildContextFile{
		{Name: "a.txt", Body: "first", Mode: 0644},
		{Name: "b.txt", Body: "unrelated", Mode: 0644},
		{Name: "a.txt", Body: "second", Mode: 0755},
	}

	rc := AppendFilesToBuildContext(files, base)
	defer rc.Close()

	entries := readTar(t, rc)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d (%v)", len(entries), tarNames(entries))
	}
	if entries[0].name != "a.txt" || entries[0].body != "second" {
		t.Errorf("entries[0] = %+v, want name=a.txt body=second", entries[0])
	}
	if entries[0].mode != 0755 {
		t.Errorf("entries[0].mode = %o, want %o", entries[0].mode, 0755)
	}
	if entries[1].name != "b.txt" {
		t.Errorf("entries[1] = %+v, want name=b.txt", entries[1])
	}
}

func TestAppendFilesToBuildContext_Empty(t *testing.T) {
	base := CreateBuildContext()
	rc := AppendFilesToBuildContext(nil, base)
	defer rc.Close()

	entries := readTar(t, rc)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestMergeBuildContexts(t *testing.T) {
	ctx1 := makeTar(t, []tarEntry{
		{name: "a.txt", body: "from1"},
		{name: "b.txt", body: "shared-from1"},
	})
	ctx2 := makeTar(t, []tarEntry{
		{name: "b.txt", body: "shared-from2"},
		{name: "c.txt", body: "from2"},
	})

	merged, err := MergeBuildContexts(ctx1, ctx2)
	if err != nil {
		t.Fatalf("MergeBuildContexts: %v", err)
	}
	defer merged.Close()

	entries := readTar(t, merged)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d (%v)", len(entries), tarNames(entries))
	}

	byName := map[string]tarEntry{}
	for _, e := range entries {
		byName[e.name] = e
	}

	if e, ok := byName["a.txt"]; !ok || e.body != "from1" {
		t.Errorf("a.txt = %+v, want body=from1", e)
	}
	if e, ok := byName["b.txt"]; !ok || e.body != "shared-from2" {
		t.Errorf("b.txt = %+v, want body=shared-from2 (buildCtx2 should win)", e)
	}
	if e, ok := byName["c.txt"]; !ok || e.body != "from2" {
		t.Errorf("c.txt = %+v, want body=from2", e)
	}

	// b.txt was matched in buildCtx1 so it keeps its original position;
	// c.txt did not exist in buildCtx1 so it is appended at the end.
	names := tarNames(entries)
	if names[0] != "a.txt" || names[1] != "b.txt" || names[2] != "c.txt" {
		t.Errorf("unexpected entry order: %v", names)
	}
}

func TestMergeBuildContexts_FirstEmpty(t *testing.T) {
	ctx1 := CreateBuildContext()
	ctx2 := makeTar(t, []tarEntry{
		{name: "only.txt", body: "content"},
	})

	merged, err := MergeBuildContexts(ctx1, ctx2)
	if err != nil {
		t.Fatalf("MergeBuildContexts: %v", err)
	}
	defer merged.Close()

	entries := readTar(t, merged)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].name != "only.txt" || entries[0].body != "content" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
}

func TestMergeBuildContexts_SecondEmpty(t *testing.T) {
	ctx1 := makeTar(t, []tarEntry{
		{name: "only.txt", body: "content"},
	})
	ctx2 := CreateBuildContext()

	merged, err := MergeBuildContexts(ctx1, ctx2)
	if err != nil {
		t.Fatalf("MergeBuildContexts: %v", err)
	}
	defer merged.Close()

	entries := readTar(t, merged)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].name != "only.txt" || entries[0].body != "content" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
}

func TestCreateBuildContextFromPath(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n")
	writeFile(t, filepath.Join(dir, "app.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "debug.log"), "log content\n")
	writeFile(t, filepath.Join(dir, ".dockerignore"), "*.log\n")
	if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "node_modules", "pkg.json"), "{}")

	rc, err := CreateBuildContextFromPath(dir, "Dockerfile")
	if err != nil {
		t.Fatalf("CreateBuildContextFromPath: %v", err)
	}
	defer rc.Close()

	entries := readTar(t, rc)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.name] = true
	}

	for _, want := range []string{"Dockerfile", "app.go", ".dockerignore"} {
		if !names[want] {
			t.Errorf("expected %q in build context, entries: %v", want, tarNames(entries))
		}
	}
	if names["debug.log"] {
		t.Errorf("debug.log should have been excluded by .dockerignore, entries: %v", tarNames(entries))
	}
}

// Even when .dockerignore excludes everything, the Dockerfile and the
// .dockerignore file itself must always remain part of the build context.
func TestCreateBuildContextFromPath_ExcludeEverythingKeepsBuildFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n")
	writeFile(t, filepath.Join(dir, "other.txt"), "irrelevant\n")
	writeFile(t, filepath.Join(dir, ".dockerignore"), "*\n")

	rc, err := CreateBuildContextFromPath(dir, "Dockerfile")
	if err != nil {
		t.Fatalf("CreateBuildContextFromPath: %v", err)
	}
	defer rc.Close()

	entries := readTar(t, rc)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.name] = true
	}

	if !names["Dockerfile"] {
		t.Errorf("Dockerfile must be kept in build context even with catch-all .dockerignore, entries: %v", tarNames(entries))
	}
	if !names[".dockerignore"] {
		t.Errorf(".dockerignore must be kept in build context, entries: %v", tarNames(entries))
	}
	if names["other.txt"] {
		t.Errorf("other.txt should have been excluded, entries: %v", tarNames(entries))
	}
}

func TestCreateBuildContextFromPath_NoDockerignore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")

	rc, err := CreateBuildContextFromPath(dir, "Dockerfile")
	if err != nil {
		t.Fatalf("CreateBuildContextFromPath: %v", err)
	}
	defer rc.Close()

	entries := readTar(t, rc)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.name] = true
	}
	if !names["Dockerfile"] || !names["main.go"] {
		t.Errorf("expected all files present without a .dockerignore, entries: %v", tarNames(entries))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
