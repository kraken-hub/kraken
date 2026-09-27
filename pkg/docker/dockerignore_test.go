package docker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadDockerignore_NoFile(t *testing.T) {
	dir := t.TempDir()

	excludes, err := ReadDockerignore(dir)
	if err != nil {
		t.Fatalf("ReadDockerignore: %v", err)
	}
	if excludes != nil {
		t.Errorf("excludes = %v, want nil", excludes)
	}
}

func TestReadDockerignore_ParsesPatterns(t *testing.T) {
	dir := t.TempDir()
	content := "# a comment\n" +
		"node_modules\n" +
		"*.log\n" +
		"\n" +
		"  /leading-slash \n" +
		"!keep.log\n"
	if err := os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	excludes, err := ReadDockerignore(dir)
	if err != nil {
		t.Fatalf("ReadDockerignore: %v", err)
	}

	want := []string{"node_modules", "*.log", "leading-slash", "!keep.log"}
	if !reflect.DeepEqual(excludes, want) {
		t.Errorf("excludes = %v, want %v", excludes, want)
	}
}

func TestReadDockerignore_Empty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	excludes, err := ReadDockerignore(dir)
	if err != nil {
		t.Fatalf("ReadDockerignore: %v", err)
	}
	if len(excludes) != 0 {
		t.Errorf("excludes = %v, want empty", excludes)
	}
}

func TestReadDockerignore_ContextIsFileNotDir(t *testing.T) {
	dir := t.TempDir()
	// Passing a path where contextDir itself does not exist as a directory
	// should still just report "no .dockerignore" behavior via os.IsNotExist,
	// since filepath.Join(contextDir, ".dockerignore") won't exist either.
	nonexistent := filepath.Join(dir, "does-not-exist")

	excludes, err := ReadDockerignore(nonexistent)
	if err != nil {
		t.Fatalf("ReadDockerignore: %v", err)
	}
	if excludes != nil {
		t.Errorf("excludes = %v, want nil", excludes)
	}
}

func TestReadDockerignore_UnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission checks don't apply")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".dockerignore")
	if err := os.WriteFile(path, []byte("node_modules\n"), 0000); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(func() { os.Chmod(path, 0644) })

	_, err := ReadDockerignore(dir)
	if err == nil {
		t.Errorf("expected an error opening an unreadable .dockerignore file")
	}
}

func TestTrimBuildFilesFromExcludes(t *testing.T) {
	tests := []struct {
		name                string
		excludes            []string
		dockerfile          string
		dockerfileFromStdin bool
		want                []string
	}{
		{
			name:       "no matching excludes leaves list untouched",
			excludes:   []string{"node_modules", "*.log"},
			dockerfile: "Dockerfile",
			want:       []string{"node_modules", "*.log"},
		},
		{
			name:       "catch-all exclude adds negations for dockerignore and dockerfile",
			excludes:   []string{"*"},
			dockerfile: "Dockerfile",
			want:       []string{"*", "!.dockerignore", "!Dockerfile"},
		},
		{
			name:                "dockerfile from stdin skips dockerfile negation",
			excludes:            []string{"*"},
			dockerfile:          "Dockerfile",
			dockerfileFromStdin: true,
			want:                []string{"*", "!.dockerignore"},
		},
		{
			name:       "explicit dockerfile exclude is negated",
			excludes:   []string{"Dockerfile"},
			dockerfile: "Dockerfile",
			want:       []string{"Dockerfile", "!Dockerfile"},
		},
		{
			name:       "explicit dockerignore exclude is negated",
			excludes:   []string{".dockerignore"},
			dockerfile: "Dockerfile",
			want:       []string{".dockerignore", "!.dockerignore"},
		},
		{
			name:       "nested dockerfile path",
			excludes:   []string{"docker/*"},
			dockerfile: "docker/Dockerfile",
			want:       []string{"docker/*", "!docker/Dockerfile"},
		},
		{
			name:       "empty excludes stay empty",
			excludes:   nil,
			dockerfile: "Dockerfile",
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrimBuildFilesFromExcludes(tt.excludes, tt.dockerfile, tt.dockerfileFromStdin)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
