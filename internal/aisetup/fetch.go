package aisetup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/cliupgrade"
)

const defaultGitHubRepo = "DavidHoenisch/remotr"

// FetchOptions configures downloading a bundle from GitHub.
type FetchOptions struct {
	Repo       string
	Version    string
	HTTPClient *http.Client
}

// FetchedBundle is a extracted bundle directory on disk.
type FetchedBundle struct {
	Dir     string
	Tag     string
	cleanup func() error
}

func (b FetchedBundle) Close() error {
	if b.cleanup != nil {
		return b.cleanup()
	}
	return nil
}

// FetchFromGitHub downloads ai/remotr-agent from a release tag (or latest stable).
func FetchFromGitHub(ctx context.Context, opt FetchOptions) (FetchedBundle, error) {
	repo := strings.TrimSpace(opt.Repo)
	if repo == "" {
		repo = defaultGitHubRepo
	}
	client := opt.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}

	tag := strings.TrimSpace(opt.Version)
	if tag == "" {
		latest, err := cliupgrade.LatestStableRelease(ctx, client, repo)
		if err != nil {
			return FetchedBundle{}, err
		}
		tag = latest
	} else if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}

	data, err := downloadSourceArchive(ctx, client, repo, tag)
	if err != nil {
		return FetchedBundle{}, err
	}

	dir, err := extractBundleToTemp(data, tag)
	if err != nil {
		return FetchedBundle{}, err
	}
	return FetchedBundle{
		Dir: dir,
		Tag: tag,
		cleanup: func() error {
			return os.RemoveAll(dir)
		},
	}, nil
}

func (b FetchedBundle) FS() fs.FS {
	return os.DirFS(b.Dir)
}

func downloadSourceArchive(ctx context.Context, client *http.Client, repo, tag string) ([]byte, error) {
	url := fmt.Sprintf("https://github.com/%s/archive/refs/tags/%s.tar.gz", repo, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "remotr-cli")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download source archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download source archive: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func extractBundleToTemp(data []byte, tag string) (string, error) {
	tmp, err := os.MkdirTemp("", "remotr-ai-bundle-*")
	if err != nil {
		return "", err
	}
	clean := func() { _ = os.RemoveAll(tmp) }

	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		clean()
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var topPrefix string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			clean()
			return "", fmt.Errorf("tar: %w", err)
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if topPrefix == "" && hdr.Typeflag == tar.TypeDir && strings.Count(strings.Trim(name, "/"), "/") == 0 {
			topPrefix = strings.TrimSuffix(name, "/")
		}
		if !strings.Contains(name, "/ai/remotr-agent/") && !strings.HasSuffix(name, "/ai/remotr-agent") {
			continue
		}
		idx := strings.Index(name, "/ai/remotr-agent/")
		if idx < 0 {
			if strings.HasSuffix(name, "/ai/remotr-agent") {
				continue
			}
			clean()
			return "", fmt.Errorf("unexpected bundle path %q", name)
		}
		rel := name[idx+len("/ai/remotr-agent/"):]
		if rel == "" {
			if hdr.Typeflag == tar.TypeDir {
				continue
			}
		}
		dest := filepath.Join(tmp, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				clean()
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				clean()
				return "", err
			}
			mode := fs.FileMode(hdr.Mode)
			if mode == 0 {
				mode = 0o644
			}
			if strings.HasSuffix(rel, ".sh") {
				mode = mode | 0o755
			}
			out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode) // #nosec G304 G703
			if err != nil {
				clean()
				return "", err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, 8<<20)); err != nil {
				out.Close()
				clean()
				return "", err
			}
			out.Close()
		}
	}

	if _, err := os.Stat(filepath.Join(tmp, "SKILL.md")); err != nil {
		clean()
		return "", fmt.Errorf("archive for %s missing ai/remotr-agent/SKILL.md (prefix %q)", tag, topPrefix)
	}
	return tmp, nil
}
