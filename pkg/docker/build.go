package docker

import (
	"archive/tar"
	"bytes"
	"io"
	"path/filepath"
	"time"

	"github.com/moby/go-archive"
)

func CreateBuildContextFromPath(src, dockerfile string) (io.ReadCloser, error) {
	excludes, err := ReadDockerignore(src)
	if err != nil {
		return nil, err
	}

	relDockerfile := filepath.Clean(dockerfile)
	excludes = TrimBuildFilesFromExcludes(excludes, relDockerfile, false)

	return archive.TarWithOptions(src, &archive.TarOptions{
		ExcludePatterns: excludes,
	})
}

type BuildContextFile struct {
	Name, Body string
	Mode       int64
}

func CreateBuildContext() io.ReadCloser {
	var buf bytes.Buffer
	return io.NopCloser(&buf)
}

func AppendFileToBuildContext(file BuildContextFile, buildCtx io.ReadCloser) io.ReadCloser {
	now := time.Now()
	hdr := &tar.Header{
		Uid:        0,
		Gid:        0,
		Mode:       file.Mode,
		ModTime:    now,
		AccessTime: now,
		ChangeTime: now,
		Typeflag:   tar.TypeReg,
	}
	name := filepath.Join(file.Name, ".")
	body := []byte(file.Body)

	return archive.ReplaceFileTarWrapper(buildCtx, map[string]archive.TarModifierFunc{
		name: func(_ string, h *tar.Header, content io.Reader) (*tar.Header, []byte, error) {
			return hdr, body, nil
		},
	})
}

func AppendFilesToBuildContext(files []BuildContextFile, buildCtx io.ReadCloser) io.ReadCloser {
	for _, file := range files {
		buildCtx = AppendFileToBuildContext(file, buildCtx)
	}
	return buildCtx
}

func MergeBuildContexts(buildCtx1, buildCtx2 io.ReadCloser) (io.ReadCloser, error) {
	buildCtx := buildCtx1
	tr := tar.NewReader(buildCtx2)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		var body bytes.Buffer
		if _, err := io.Copy(&body, tr); err != nil {
			return nil, err
		}
		hdrValue := *hdr
		hdrCopy := &hdrValue
		bodyCopy := body.Bytes()
		buildCtx = archive.ReplaceFileTarWrapper(buildCtx, map[string]archive.TarModifierFunc{
			hdr.Name: func(_ string, h *tar.Header, content io.Reader) (*tar.Header, []byte, error) {
				return hdrCopy, bodyCopy, nil
			},
		})
	}
	return buildCtx, nil
}
