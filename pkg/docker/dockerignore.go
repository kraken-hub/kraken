package docker

import (
	"os"
	"path/filepath"

	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

// ReadDockerignore reads the .dockerignore file in the context directory and
// returns the list of paths to exclude.
func ReadDockerignore(contextDir string) ([]string, error) {
	f, err := os.Open(filepath.Join(contextDir, ".dockerignore"))
	switch {
	case os.IsNotExist(err):
		return nil, nil
	case err != nil:
		return nil, err
	}
	defer f.Close()
	return ignorefile.ReadAll(f)
}

// TrimBuildFilesFromExcludes ensures the Dockerfile and .dockerignore are
// never excluded from the build context.
func TrimBuildFilesFromExcludes(excludes []string, dockerfile string, dockerfileFromStdin bool) []string {
	if ok, _ := patternmatcher.Matches(".dockerignore", excludes); ok {
		excludes = append(excludes, "!.dockerignore")
	}
	if !dockerfileFromStdin {
		if ok, _ := patternmatcher.Matches(dockerfile, excludes); ok {
			excludes = append(excludes, "!"+dockerfile)
		}
	}
	return excludes
}
