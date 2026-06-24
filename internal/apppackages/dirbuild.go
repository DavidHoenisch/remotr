package apppackages

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirSummary holds validation output for a package source directory.
type DirSummary struct {
	Dir      string
	Manifest Manifest
}

// ValidateDir checks remotr-package.yaml and referenced files under dir.
func ValidateDir(dir string) (DirSummary, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return DirSummary{}, fmt.Errorf("path required")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return DirSummary{}, err
	}
	if !info.IsDir() {
		return DirSummary{}, fmt.Errorf("path is not a directory: %s", dir)
	}

	manifestPath := filepath.Join(dir, ManifestName)
	raw, err := os.ReadFile(manifestPath) // #nosec G703
	if err != nil {
		return DirSummary{}, fmt.Errorf("read %s: %w", ManifestName, err)
	}
	manifest, err := ParseManifest(raw)
	if err != nil {
		return DirSummary{}, err
	}

	for _, src := range manifestSources(manifest) {
		member := filepath.Join(dir, filepath.FromSlash(src))
		if _, err := os.Stat(member); err != nil {
			return DirSummary{}, fmt.Errorf("manifest references missing file %q", src)
		}
	}

	return DirSummary{Dir: dir, Manifest: manifest}, nil
}

// DefaultZipFilename returns a conventional archive name for a manifest.
func DefaultZipFilename(m Manifest) string {
	safeName := strings.ReplaceAll(m.Name, "/", "_")
	return fmt.Sprintf("%s-%s.zip", safeName, m.Version)
}

// BuildZipFromDir validates dir and returns a zip archive plus summary.
func BuildZipFromDir(dir string) ([]byte, ZipSummary, error) {
	dirSum, err := ValidateDir(dir)
	if err != nil {
		return nil, ZipSummary{}, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "../") || rel == ".." {
			return fmt.Errorf("invalid file path: %s", rel)
		}

		data, err := os.ReadFile(path) // #nosec G703 -- under validated package dir
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, ZipSummary{}, err
	}
	if err := zw.Close(); err != nil {
		return nil, ZipSummary{}, err
	}

	data := buf.Bytes()
	sum := sha256.Sum256(data)
	// Re-validate the produced zip matches manifest expectations.
	zipSum, err := ValidateZip(data)
	if err != nil {
		return nil, ZipSummary{}, err
	}
	if zipSum.Manifest.Name != dirSum.Manifest.Name || zipSum.Manifest.Version != dirSum.Manifest.Version {
		return nil, ZipSummary{}, fmt.Errorf("built zip manifest mismatch")
	}
	zipSum.SHA256 = hex.EncodeToString(sum[:])
	zipSum.Size = int64(len(data))
	return data, zipSum, nil
}

// WriteZipFile builds dir into outPath.
func WriteZipFile(dir, outPath string) (ZipSummary, error) {
	data, sum, err := BuildZipFromDir(dir)
	if err != nil {
		return ZipSummary{}, err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil { // #nosec G703
		return ZipSummary{}, err
	}
	return sum, nil
}
