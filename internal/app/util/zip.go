package util

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExtractZip extracts zip bytes to destDir, stripping a single shared top-level
// directory prefix if present, and preserving Unix file permissions.
func ExtractZip(data []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	prefix := zipTopLevelPrefix(r.File)
	destPrefix := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range r.File {
		name := strings.TrimPrefix(f.Name, prefix)
		if name == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		if filepath.Clean(target) != filepath.Clean(destDir) && !strings.HasPrefix(filepath.Clean(target), destPrefix) {
			return fmt.Errorf("zip entry %q would escape destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

// zipTopLevelPrefix returns the single top-level directory prefix shared by all
// zip entries (e.g. "my-plugin/"), or "" if entries are at the root.
func zipTopLevelPrefix(files []*zip.File) string {
	var prefix string
	for _, f := range files {
		name := filepath.ToSlash(f.Name)
		slash := strings.Index(name, "/")
		if slash < 0 {
			return ""
		}
		p := name[:slash+1]
		if prefix == "" {
			prefix = p
		} else if !strings.HasPrefix(name, prefix) {
			return ""
		}
	}
	return prefix
}

func writeZipEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
