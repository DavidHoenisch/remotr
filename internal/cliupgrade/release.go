package cliupgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
)

const defaultGitHubRepo = "DavidHoenisch/remotr"

type releaseInfo struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// LatestStableRelease returns the newest non-draft, non-prerelease tag from GitHub.
func LatestStableRelease(ctx context.Context, client *http.Client, repo string) (string, error) {
	if strings.TrimSpace(repo) == "" {
		repo = defaultGitHubRepo
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "remotr-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query GitHub releases: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub releases API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var info releaseInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("parse GitHub release: %w", err)
	}
	tag := strings.TrimSpace(info.TagName)
	if tag == "" {
		return "", fmt.Errorf("GitHub release missing tag_name")
	}
	if info.Draft || info.Prerelease {
		return "", fmt.Errorf("latest release %q is not stable", tag)
	}
	if _, err := agentversion.Normalize(tag); err != nil {
		return "", fmt.Errorf("release tag: %w", err)
	}
	return tag, nil
}
