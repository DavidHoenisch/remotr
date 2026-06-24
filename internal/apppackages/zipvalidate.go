package apppackages

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// ZipSummary holds validation output for a package zip.
type ZipSummary struct {
	Manifest Manifest
	SHA256   string
	Size     int64
}

// ValidateZip checks zip layout, manifest, and referenced member files.
func ValidateZip(data []byte) (ZipSummary, error) {
	sum := sha256.Sum256(data)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ZipSummary{}, fmt.Errorf("open zip: %w", err)
	}

	var manifestRaw []byte
	members := map[string]struct{}{}
	for _, f := range zr.File {
		name := normalizeZipName(f.Name)
		if name == "" {
			continue
		}
		members[name] = struct{}{}
		if name == ManifestName {
			rc, err := f.Open()
			if err != nil {
				return ZipSummary{}, fmt.Errorf("open manifest: %w", err)
			}
			manifestRaw, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return ZipSummary{}, fmt.Errorf("read manifest: %w", err)
			}
		}
	}
	if len(manifestRaw) == 0 {
		return ZipSummary{}, fmt.Errorf("zip missing %s at archive root", ManifestName)
	}

	manifest, err := ParseManifest(manifestRaw)
	if err != nil {
		return ZipSummary{}, err
	}

	for _, src := range manifestSources(manifest) {
		if _, ok := members[normalizeZipName(src)]; !ok {
			return ZipSummary{}, fmt.Errorf("manifest references missing zip member %q", src)
		}
	}

	return ZipSummary{
		Manifest: manifest,
		SHA256:   hex.EncodeToString(sum[:]),
		Size:     int64(len(data)),
	}, nil
}

func manifestSources(m Manifest) []string {
	var out []string
	switch m.Install.Mode {
	case "binary":
		for _, f := range m.Install.Files {
			out = append(out, f.Src)
		}
	case "script", "build":
		if len(m.Install.Script) > 0 {
			out = append(out, relPathFromScript(m.Install.Script[0]))
		}
		if m.Install.Mode == "build" && len(m.Install.Script) == 0 {
			out = append(out, "install.sh")
		}
	}
	if m.Uninstall != nil && len(m.Uninstall.Script) > 0 {
		out = append(out, relPathFromScript(m.Uninstall.Script[0]))
	}
	return out
}

func relPathFromScript(argv0 string) string {
	s := strings.TrimSpace(argv0)
	if strings.HasPrefix(s, "./") {
		return s[2:]
	}
	return s
}

func normalizeZipName(name string) string {
	return strings.TrimPrefix(filepathClean(name), "/")
}

func filepathClean(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimSpace(path)
	for strings.HasPrefix(path, "./") {
		path = path[2:]
	}
	return path
}
