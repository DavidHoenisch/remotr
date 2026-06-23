package cliupgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
)

func extractBinary(data []byte, goos, wantName string) ([]byte, error) {
	if goos == "windows" {
		return extractZipBinary(data, wantName)
	}
	return extractTarGzBinary(data, wantName)
}

func extractTarGzBinary(data []byte, wantName string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if filepathBase(name) != wantName {
			continue
		}
		out, err := io.ReadAll(io.LimitReader(tr, 256<<20))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", wantName, err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("archive missing %s", wantName)
}

func extractZipBinary(data []byte, wantName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}
	for _, f := range zr.File {
		if filepathBase(f.Name) != wantName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		out, err := io.ReadAll(io.LimitReader(rc, 256<<20))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", wantName, err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("archive missing %s", wantName)
}

func filepathBase(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
