package cliupgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
)

// Options configures a CLI self-upgrade.
type Options struct {
	CurrentVersion string
	TargetVersion  string
	GitHubRepo     string
	InstallPath    string
	Force          bool
	CheckOnly      bool
	HTTPClient     *http.Client
}

// Result summarizes an upgrade run.
type Result struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Target     string `json:"target"`
	UpToDate   bool   `json:"up_to_date"`
	Installed  bool   `json:"installed"`
	InstallPath string `json:"install_path,omitempty"`
}

// Run resolves the target release and optionally installs it over the current binary.
func Run(ctx context.Context, opt Options) (Result, error) {
	current := strings.TrimSpace(opt.CurrentVersion)
	if current == "" {
		current = "dev"
	}

	repo := strings.TrimSpace(opt.GitHubRepo)
	if repo == "" {
		repo = defaultGitHubRepo
	}
	client := opt.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}

	target := strings.TrimSpace(opt.TargetVersion)
	if target == "" {
		tag, err := LatestStableRelease(ctx, client, repo)
		if err != nil {
			return Result{Current: current}, err
		}
		target = tag
	} else {
		normalized, err := agentversion.Normalize(target)
		if err != nil {
			return Result{Current: current}, err
		}
		target = normalized
	}

	res := Result{
		Current: current,
		Latest:  target,
		Target:  target,
	}

	if !opt.Force && !upgradeNeeded(current, target) {
		res.UpToDate = true
		return res, nil
	}
	if opt.CheckOnly {
		res.UpToDate = false
		return res, nil
	}

	goos, goarch, err := currentPlatform()
	if err != nil {
		return res, err
	}

	dest, err := resolveInstallPath(opt.InstallPath)
	if err != nil {
		return res, err
	}
	res.InstallPath = dest

	url := downloadURL(repo, target, goos, goarch)
	data, err := downloadReleaseAsset(ctx, client, url)
	if err != nil {
		return res, err
	}
	bin, err := extractBinary(data, goos, binaryName(goos))
	if err != nil {
		return res, err
	}
	if err := installBinary(bin, dest); err != nil {
		return res, fmt.Errorf("install %s: %w (try running with appropriate permissions)", dest, err)
	}
	res.Installed = true
	res.UpToDate = false
	return res, nil
}

func upgradeNeeded(current, target string) bool {
	if strings.EqualFold(strings.TrimSpace(current), "dev") {
		return true
	}
	if agentversion.Match(current, target) {
		return false
	}
	cmp, err := agentversion.Compare(current, target)
	if err != nil {
		return true
	}
	return cmp < 0
}

func downloadReleaseAsset(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "remotr-cli")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download release: HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("download release: empty archive from %s", url)
	}
	return data, nil
}
